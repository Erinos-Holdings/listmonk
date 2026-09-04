package migrations

// Fork (import presets) -- DB harness for the upsert-subscriber-fill statement and the
// preset import path end to end (Transform -> PrepareImport -> RunPreset), through the
// REAL goyesql-parsed statements. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG); every fixture is synthetic.

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

const presetTestJSON = `[{
  "key": "rewards",
  "name": "Import Rewards",
  "columns": { "email": "earner_email", "name": "earner_name", "locale": "earner_locale" },
  "attrib_columns": { "earned_from_site_url": "rewards_site_url" },
  "list_name_pattern": "^(?P<date>[0-9]{6})_rewards_(?P<kind>[a-z]+)_list( \\([0-9]+\\))?\\.csv$",
  "list_name_template": "{date} Rewards-{Kind}",
  "list_type": "private", "list_optin": "single",
  "subscription_status": "confirmed", "backfill": true, "merge": "fill",
  "skip_email_pattern": "@canceled\\.local$",
  "dedupe": { "name": "longest-first", "locale": "first-mapped", "attribs": "last" }
}]`

type presetHarness struct {
	*evergreenHarness
	im   *subimporter.Importer
	p    *subimporter.Preset
	fill *sql.Stmt
}

func newPresetHarness(t *testing.T) *presetHarness {
	h := newEvergreenHarness(t)
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","subscribers.invalidEmail":"invalid email","subscribers.domainBlocklisted":"domain blocklisted"}`))
	if err != nil {
		t.Fatal(err)
	}
	prep := func(name string) *sql.Stmt {
		q, ok := h.qs[name]
		if !ok {
			t.Fatalf("query %s not found", name)
		}
		st, err := h.db.DB.Prepare(q.Query)
		if err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		return st
	}
	im := subimporter.New(subimporter.Options{
		UpsertStmt:         prep("upsert-subscriber"),
		UpsertFillStmt:     prep("upsert-subscriber-fill"),
		BlocklistStmt:      prep("upsert-blocklist-subscriber"),
		UpdateListDateStmt: prep("update-lists-date"),
		PostCB:             func(string, any) error { return nil },
	}, h.db.DB, i)

	ps, err := subimporter.ParsePresets([]byte(presetTestJSON), models.CampaignLangs)
	if err != nil {
		t.Fatal(err)
	}
	return &presetHarness{evergreenHarness: h, im: im, p: &ps[0], fill: prep("upsert-subscriber-fill")}
}

// fillOne runs the fill statement for one row under the backfill flag, as a session does.
func (h *presetHarness) fillOne(email, name, attribs string, list int, status string) {
	h.t.Helper()
	tx, err := h.db.Begin()
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := tx.Exec(`SET LOCAL listmonk.backfill = 'true'`); err != nil {
		h.t.Fatal(err)
	}
	if _, err := tx.Stmt(h.fill).Exec("00000000-0000-4000-8000-"+strings.Repeat("0", 12), email, name, attribs, pq.Array([]int{list}), status, subimporter.PlaceholderName(email)); err != nil {
		tx.Rollback()
		h.t.Fatalf("upsert-subscriber-fill %s: %v", email, err)
	}
	if err := tx.Commit(); err != nil {
		h.t.Fatal(err)
	}
}

func (h *presetHarness) sub(email string) (name string, attribs string, status string) {
	h.t.Helper()
	if err := h.db.QueryRow(`SELECT name, attribs::text, status FROM subscribers WHERE email = $1`, email).Scan(&name, &attribs, &status); err != nil {
		h.t.Fatalf("subscriber %s: %v", email, err)
	}
	return
}

func (h *presetHarness) membership(email string, list int) (status string, confirmed bool) {
	h.t.Helper()
	var ts sql.NullTime
	if err := h.db.QueryRow(`SELECT sl.status, sl.confirmed_at FROM subscriber_lists sl JOIN subscribers s ON s.id = sl.subscriber_id WHERE s.email = $1 AND sl.list_id = $2`, email, list).Scan(&status, &ts); err != nil {
		h.t.Fatalf("membership %s/%d: %v", email, list, err)
	}
	return status, ts.Valid
}

// waitImport blocks until the importer leaves the importing/stopping states.
func (h *presetHarness) waitImport() string {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := h.im.GetStats().Status
		if st != subimporter.StatusImporting && st != subimporter.StatusStopping {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("import did not finish: %s", h.im.GetStats().Status)
	return ""
}

// runPreset drives the confirm path exactly as the handler does and waits for it.
func (h *presetHarness) runPreset(filename string, data []byte) *subimporter.Prepared {
	h.t.Helper()
	prep, err := subimporter.PrepareImport(context.Background(), h.db.DB, h.im, h.p, filename, data, subimporter.ContentHash(data))
	if err != nil {
		h.t.Fatalf("PrepareImport: %v", err)
	}
	if err := h.im.RunPreset(h.p, filename, prep.List.ID, prep.Parsed.Subs); err != nil {
		h.t.Fatalf("RunPreset: %v", err)
	}
	if st := h.waitImport(); st != subimporter.StatusFinished {
		h.t.Fatalf("import ended %s: %s", st, h.im.GetLogs())
	}
	h.im.Stop() // clears a finished session, as the UI's Done button does
	return prep
}

// I1, I2, I3, I4, I7 -- the fill statement's semantics, row by row.
func TestPresetFillStatement(t *testing.T) {
	h := newPresetHarness(t)
	L := h.listA
	ins := func(email, name, attribs string) {
		h.db.MustExec(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, $2, $3::jsonb)`, email, name, attribs)
	}

	// I1 -- a real name is never changed; empty and placeholder names are filled.
	ins("real@example.test", "Real Person", `{"lang":"fr","shop_links":["a"]}`)
	ins("empty@example.test", "", `{}`)
	ins("ann42@example.test", "Ann42", `{}`)
	ins("jane.doe@example.test", "Jane Doe", `{}`)
	ins("bine-j@example.test", "Bine-J", `{}`)
	h.fillOne("real@example.test", "File Name", `{"lang":"en","rewards_site_url":"u"}`, L, "confirmed")
	h.fillOne("empty@example.test", "Filled Empty", `{"lang":"en"}`, L, "confirmed")
	h.fillOne("ann42@example.test", "Ann Real", `{}`, L, "confirmed")
	h.fillOne("jane.doe@example.test", "Jane Real", `{}`, L, "confirmed")
	h.fillOne("bine-j@example.test", "Bine Real", `{}`, L, "confirmed")
	for email, want := range map[string]string{
		"real@example.test": "Real Person", "empty@example.test": "Filled Empty", "ann42@example.test": "Ann Real",
		"jane.doe@example.test": "Jane Real", "bine-j@example.test": "Bine Real",
	} {
		if name, _, _ := h.sub(email); name != want {
			t.Errorf("I1 %s: name %q want %q", email, name, want)
		}
	}

	// I2 -- lang only when absent or blank; a present value is never changed.
	_, attribs, _ := h.sub("real@example.test")
	if !strings.Contains(attribs, `"lang": "fr"`) {
		t.Errorf("I2 fr must stay under an en row: %s", attribs)
	}
	if !strings.Contains(attribs, `"shop_links": ["a"]`) || !strings.Contains(attribs, `"rewards_site_url": "u"`) {
		t.Errorf("I4 attribs must merge, not replace: %s", attribs)
	}
	ins("blank@example.test", "B", `{"lang":""}`)
	ins("absent@example.test", "A", `{"x":1}`)
	h.fillOne("blank@example.test", "B", `{"lang":"de"}`, L, "confirmed")
	h.fillOne("absent@example.test", "A", `{"lang":"it"}`, L, "confirmed")
	if _, a, _ := h.sub("blank@example.test"); !strings.Contains(a, `"lang": "de"`) {
		t.Errorf("I2 blank -> filled: %s", a)
	}
	if _, a, _ := h.sub("absent@example.test"); !strings.Contains(a, `"lang": "it"`) || !strings.Contains(a, `"x": 1`) {
		t.Errorf("I2 absent -> filled, other keys kept: %s", a)
	}

	// I3 -- an empty file name arrives as the placeholder and changes nothing, never blanks.
	ins("keep.me@example.test", "Keeper", `{}`)
	h.fillOne("keep.me@example.test", subimporter.PlaceholderName("keep.me@example.test"), `{}`, L, "confirmed")
	if name, _, _ := h.sub("keep.me@example.test"); name != "Keeper" {
		t.Errorf("I3 placeholder-in must not change a real name: %q", name)
	}
	ins("holder@example.test", "Holder", `{}`) // stored placeholder for holder@
	h.fillOne("holder@example.test", "Holder", `{}`, L, "confirmed")
	if name, _, _ := h.sub("holder@example.test"); name != "Holder" {
		t.Errorf("I3 placeholder-in on placeholder-stored stays the placeholder: %q", name)
	}

	// I4 -- attrib_columns overwritten by the file; an empty column (key omitted) leaves it.
	ins("url@example.test", "U", `{"rewards_site_url":"old","signups":{"a":1}}`)
	h.fillOne("url@example.test", "U", `{"rewards_site_url":"new"}`, L, "confirmed")
	if _, a, _ := h.sub("url@example.test"); !strings.Contains(a, `"rewards_site_url": "new"`) || !strings.Contains(a, `"signups"`) {
		t.Errorf("I4 overwrite/keep: %s", a)
	}
	h.fillOne("url@example.test", "U", `{}`, L, "confirmed")
	if _, a, _ := h.sub("url@example.test"); !strings.Contains(a, `"rewards_site_url": "new"`) {
		t.Errorf("I4 omitted key must leave the stored value: %s", a)
	}
	// I4 -- a stored attribs of JSON null (and a non-object) is treated as {} and imports.
	ins("null@example.test", "N", `null`)
	ins("scalar@example.test", "S", `"text"`)
	h.fillOne("null@example.test", "N", `{"lang":"en","rewards_site_url":"u"}`, L, "confirmed")
	h.fillOne("scalar@example.test", "S", `{"lang":"en"}`, L, "confirmed")
	if _, a, _ := h.sub("null@example.test"); !strings.Contains(a, `"lang": "en"`) || !strings.Contains(a, `"rewards_site_url": "u"`) {
		t.Errorf("I4 null attribs: %s", a)
	}
	if _, a, _ := h.sub("scalar@example.test"); !strings.Contains(a, `"lang": "en"`) {
		t.Errorf("I4 scalar attribs: %s", a)
	}
	// And a null $4 side (no attribs at all) is guarded too.
	h.fillOne("null@example.test", "N", `null`, L, "confirmed")

	// I7 -- blocklisted subscribers land unsubscribed and stay blocklisted.
	ins("blocked@example.test", "Blocked", `{}`)
	h.db.MustExec(`UPDATE subscribers SET status = 'blocklisted' WHERE email = 'blocked@example.test'`)
	h.fillOne("blocked@example.test", "Blocked", `{}`, L, "confirmed")
	if _, _, st := h.sub("blocked@example.test"); st != "blocklisted" {
		t.Errorf("I7 status %s", st)
	}
	if st, _ := h.membership("blocked@example.test", L); st != "unsubscribed" {
		t.Errorf("I7 membership %s", st)
	}

	// Membership status of an existing row is kept.
	h.join(h.subscriber("unsub@example.test"), L, "unsubscribed")
	h.fillOne("unsub@example.test", "X", `{}`, L, "confirmed")
	if st, _ := h.membership("unsub@example.test", L); st != "unsubscribed" {
		t.Errorf("existing membership must be kept: %s", st)
	}

	// A mixed-case STORED email is the same subscriber (the conflict target is LOWER(email),
	// matching the preview's lookup) -- it is filled, not duplicated, and never raises.
	ins("Mixed.Case@Example.TEST", "Mixed Case", `{}`)
	h.fillOne("mixed.case@example.test", "Mixed Real", `{"lang":"en"}`, L, "confirmed")
	var mixed int
	h.db.Get(&mixed, `SELECT COUNT(*) FROM subscribers WHERE LOWER(email) = 'mixed.case@example.test'`)
	if mixed != 1 {
		t.Errorf("mixed-case stored email duplicated: %d rows", mixed)
	}
	if name, a, _ := h.sub("Mixed.Case@Example.TEST"); name != "Mixed Real" || !strings.Contains(a, `"lang": "en"`) {
		t.Errorf("mixed-case stored email not filled: %q %s", name, a)
	}
	if st, _ := h.membership("Mixed.Case@Example.TEST", L); st != "confirmed" {
		t.Errorf("mixed-case membership %s", st)
	}

	// A brand-new subscriber is inserted with the file's values.
	h.fillOne("new@example.test", "New Person", `{"lang":"es"}`, L, "confirmed")
	if name, a, st := h.sub("new@example.test"); name != "New Person" || !strings.Contains(a, `"lang": "es"`) || st != "enabled" {
		t.Errorf("new subscriber: %q %s %s", name, a, st)
	}
	if st, stamped := h.membership("new@example.test", L); st != "confirmed" || stamped {
		t.Errorf("new membership under backfill: %s stamped=%v", st, stamped)
	}
}

// I5 -- re-running the same file changes nothing but updated_at. Also I16 end to end.
func TestPresetImportIdempotent(t *testing.T) {
	h := newPresetHarness(t)
	h.db.MustExec(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'lyss@example.test', 'Lyss', '{"shop_links":["x"],"lang":"fr"}')`)
	file := "090426_rewards_bundle_list.csv"
	data := []byte("earner_name,earner_email,earner_locale,earned_from_site_url\r\n" +
		"Alyssa Freed,lyss@example.test,en-US,https://example.test/al\r\n" +
		"Susanne,susanne@example.test,de-DE,https://example.test/su\r\n" +
		"Gone,canceled-x@canceled.local,en-US,https://example.test/g\r\n" +
		",placeholder@example.test,,\r\n")

	snapshot := func() (string, string) {
		var subs, lists string
		h.db.Get(&subs, `SELECT COALESCE(string_agg(email || '|' || name || '|' || attribs::text || '|' || status, ';' ORDER BY email), '') FROM subscribers`)
		h.db.Get(&lists, `SELECT COALESCE(string_agg(s.email || '|' || sl.list_id || '|' || sl.status || '|' || COALESCE(sl.confirmed_at::text, ''), ';' ORDER BY s.email, sl.list_id), '') FROM subscriber_lists sl JOIN subscribers s ON s.id = sl.subscriber_id`)
		return subs, lists
	}

	prep := h.runPreset(file, data)
	if prep.List.Exists || prep.List.Name != "090426 Rewards-Bundle" {
		t.Errorf("first run must create the list: %+v", prep.List)
	}
	s1, l1 := snapshot()
	if name, a, _ := h.sub("lyss@example.test"); name != "Alyssa Freed" || !strings.Contains(a, `"lang": "fr"`) || !strings.Contains(a, `"shop_links"`) {
		t.Errorf("fill on existing: %q %s", name, a)
	}
	if name, _, _ := h.sub("placeholder@example.test"); name != "Placeholder" {
		t.Errorf("empty name -> placeholder: %q", name)
	}
	// I16 -- the skipped row was never created or linked.
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM subscribers WHERE email LIKE '%@canceled.local'`)
	if n != 0 {
		t.Errorf("I16 skipped row created")
	}
	var members int
	h.db.Get(&members, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id = $1`, prep.List.ID)
	if members != 3 {
		t.Errorf("members %d want 3", members)
	}
	// I6 at the SESSION level -- the session set the backfill flag, so no membership was
	// stamped confirmed_at and no evergreen welcome can claim these rows.
	var stamped int
	h.db.Get(&stamped, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id = $1 AND confirmed_at IS NOT NULL`, prep.List.ID)
	if stamped != 0 {
		t.Errorf("I6 %d memberships stamped confirmed_at under a preset import", stamped)
	}

	prep2 := h.runPreset(file, data)
	if !prep2.List.Exists || prep2.List.ID != prep.List.ID {
		t.Errorf("second run must reuse the list: %+v", prep2.List)
	}
	s2, l2 := snapshot()
	if s1 != s2 {
		t.Errorf("I5 subscribers changed on re-run:\n%s\n%s", s1, s2)
	}
	if l1 != l2 {
		t.Errorf("I5 memberships changed on re-run:\n%s\n%s", l1, l2)
	}
	if st := h.im.GetStats(); st.Imported != 0 || st.Total != 0 {
		// Stop() cleared the finished session, as the UI does.
		t.Errorf("stats not cleared: %+v", st)
	}
}
