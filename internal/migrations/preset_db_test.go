package migrations

// Fork (import presets) -- preview is read-only, list resolution, the zero-row guard, the
// hash guard and the feeder's stop handling. Shares newPresetHarness (LISTMONK_TEST_PG).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/subimporter"
)

func (h *presetHarness) counts() (lists, subs, memberships int) {
	h.db.Get(&lists, `SELECT COUNT(*) FROM lists`)
	h.db.Get(&subs, `SELECT COUNT(*) FROM subscribers`)
	h.db.Get(&memberships, `SELECT COUNT(*) FROM subscriber_lists`)
	return
}

// I8 -- preview writes nothing and opens no session. I12 -- exact-name resolve 0/1/>1 and
// the LOWER(email) lookup. I13 -- a hash mismatch is rejected before any write. I18 -- a
// zero-row file creates no list.
func TestPresetPreviewAndResolve(t *testing.T) {
	h := newPresetHarness(t)
	ctx := context.Background()
	h.db.MustExec(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'Mixed.Case@Example.TEST', 'Mixed Case', '{}')`)
	h.db.MustExec(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'has.lang@example.test', 'Real Name', '{"lang":"de"}')`)
	file := "090426_rewards_bundle_list.csv"
	data := []byte("earner_name,earner_email,earner_locale,earned_from_site_url\r\n" +
		"Mixed Real,mixed.case@example.test,en-US,https://example.test/m\r\n" +
		"Other Name,has.lang@example.test,fr-FR,\r\n" +
		"Newbie,new@example.test,it,\r\n" +
		"Gone,canceled-x@canceled.local,en-US,\r\n")

	l0, s0, m0 := h.counts()
	pv, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, file, data)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	l1, s1, m1 := h.counts()
	if l0 != l1 || s0 != s1 || m0 != m1 {
		t.Errorf("I8 preview wrote: lists %d->%d subs %d->%d memberships %d->%d", l0, l1, s0, s1, m0, m1)
	}
	if st := h.im.GetStats().Status; st != subimporter.StatusNone {
		t.Errorf("I8 preview opened a session: %s", st)
	}
	if pv.ContentHash != subimporter.ContentHash(data) || len(pv.ContentHash) != 64 {
		t.Errorf("hash %q", pv.ContentHash)
	}
	if pv.List.Exists || pv.List.Name != "090426 Rewards-Bundle" {
		t.Errorf("list should not exist yet: %+v", pv.List)
	}
	if pv.Rows != 4 || pv.Subscribers != 3 || pv.New != 1 || pv.Existing != 2 || len(pv.Skipped) != 1 {
		t.Errorf("counts rows=%d subs=%d new=%d existing=%d skipped=%d", pv.Rows, pv.Subscribers, pv.New, pv.Existing, len(pv.Skipped))
	}
	// I12 -- the mixed-case stored email is found by LOWER(email); its placeholder name is fillable.
	if len(pv.WillFillName) != 1 || pv.WillFillName[0].Email != "mixed.case@example.test" || pv.WillFillName[0].From != "Mixed Case" || pv.WillFillName[0].To != "Mixed Real" {
		t.Errorf("will_fill_name %+v", pv.WillFillName)
	}
	if len(pv.WillFillLang) != 1 || pv.WillFillLang[0].Email != "mixed.case@example.test" || pv.WillFillLang[0].Lang != "en" {
		t.Errorf("will_fill_lang %+v (a stored de must not be listed)", pv.WillFillLang)
	}
	if pv.Encoding != subimporter.EncodingUTF8 {
		t.Errorf("encoding %s", pv.Encoding)
	}

	// I13 -- wrong hash: rejected, nothing created.
	if _, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file, data, "0000"); !errors.Is(err, subimporter.ErrHashMismatch) {
		t.Errorf("want ErrHashMismatch, got %v", err)
	}
	if l2, _, _ := h.counts(); l2 != l0 {
		t.Errorf("I13 hash mismatch created a list")
	}

	// I18 -- zero valid rows: rejected before the list is created.
	empty := []byte("earner_name,earner_email,earner_locale,earned_from_site_url\r\nGone,canceled-y@canceled.local,en,\r\nBad,nope,en,\r\n")
	if _, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file, empty, subimporter.ContentHash(empty)); !errors.Is(err, subimporter.ErrNoValidRows) {
		t.Errorf("want ErrNoValidRows, got %v", err)
	}
	if l2, _, _ := h.counts(); l2 != l0 {
		t.Errorf("I18 zero-row file created a list")
	}
	// A bad filename is rejected before anything is read.
	if _, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, "rewards.csv", data); err == nil {
		t.Errorf("bad filename must be rejected")
	}

	// I12 -- 1 match resolves to the list; "(1).csv" resolves to the same list; >1 errors.
	h.db.MustExec(`INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), '090426 Rewards-Bundle', 'private', 'single')`)
	pv, err = subimporter.Preview(ctx, h.db.DB, h.im, h.p, "090426_rewards_bundle_list (1).csv", data)
	if err != nil {
		t.Fatal(err)
	}
	if !pv.List.Exists || pv.List.ID == 0 || pv.List.SubscriberCount != 0 {
		t.Errorf("existing list: %+v", pv.List)
	}
	h.db.MustExec(`INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), '090426 Rewards-Bundle', 'private', 'single')`)
	if _, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, file, data); !errors.Is(err, subimporter.ErrListAmbiguous) {
		t.Errorf("want ErrListAmbiguous, got %v", err)
	}
	if _, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file, data, subimporter.ContentHash(data)); !errors.Is(err, subimporter.ErrListAmbiguous) {
		t.Errorf("confirm: want ErrListAmbiguous, got %v", err)
	}
}

// I17 -- the feeder honours Stop(): after a stopped preset import, a following stock
// import proceeds normally (no stale stop request).
func TestPresetFeederHonoursStop(t *testing.T) {
	h := newPresetHarness(t)
	L := h.listA
	subs := make([]subimporter.SubReq, 0, 3)
	for _, e := range []string{"s1@example.test", "s2@example.test", "s3@example.test"} {
		s := subimporter.SubReq{}
		s.Email, s.Name = e, "S"
		subs = append(subs, s)
	}

	sess, err := h.im.NewSession(h.p.SessionOpt("090426_rewards_bundle_list.csv", L))
	if err != nil {
		t.Fatal(err)
	}
	// Stop before the feeder runs: the request sits in the buffered channel and the feeder
	// must drain it at its first row.
	h.im.Stop()
	if st := h.im.GetStats().Status; st != subimporter.StatusStopping {
		t.Fatalf("want stopping, got %s", st)
	}
	go sess.Start()
	if err := sess.LoadRows(subs); err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if st := h.waitImport(); st != subimporter.StatusFinished {
		t.Fatalf("stopped import ended %s", st)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM subscribers WHERE email LIKE 's%@example.test'`)
	if n != 0 {
		t.Errorf("stopped feed imported %d rows", n)
	}
	h.im.Stop() // clear the finished state

	// A stock CSV import now runs to completion with both rows.
	dir := t.TempDir()
	path := filepath.Join(dir, "stock.csv")
	if err := os.WriteFile(path, []byte("email,name\nt1@example.test,T1\nt2@example.test,T2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stock, err := h.im.NewSession(subimporter.SessionOpt{
		Filename: "stock.csv", Mode: subimporter.ModeSubscribe, SubStatus: "confirmed", Delim: ",", ListIDs: []int{L},
	})
	if err != nil {
		t.Fatal(err)
	}
	go stock.Start()
	if err := stock.LoadCSV(path, ','); err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}
	if st := h.waitImport(); st != subimporter.StatusFinished {
		t.Fatalf("stock import ended %s: %s", st, h.im.GetLogs())
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM subscribers WHERE email LIKE 't%@example.test'`)
	if n != 2 {
		t.Errorf("I17 stock import after a stopped preset import imported %d of 2 rows", n)
	}
	if st := h.im.GetStats(); st.Imported != 2 {
		t.Errorf("imported count %d", st.Imported)
	}
}

// BRAND-PICKER-SPEC I11 -- the preset's list_brand (the harness preset names "rewards"): a NEW
// list is created with the brand row's projection; an existing UNTAGGED list of the name is
// tagged with it on confirm (free tags kept) and the preview says "will tag"; an existing list of
// the SAME brand is reused untouched; one of ANOTHER brand fails the preview and the confirm,
// naming both brands, and nothing is written.
func TestPresetListBrandCreateTagReuseConflict(t *testing.T) {
	h := newPresetHarness(t)
	ctx := context.Background()
	data := []byte("earner_name,earner_email,earner_locale,earned_from_site_url\r\nAnn,ann@example.test,en,\r\n")
	hash := subimporter.ContentHash(data)
	const proj = "brand:rewards|from:Rewards <hello@rewards.test>"
	tags := func(id int) string {
		var out []string
		h.db.Select(&out, `SELECT unnest(tags) FROM lists WHERE id = $1 ORDER BY 1`, id)
		return strings.Join(out, "|")
	}
	file := func(kind string) string { return "090426_rewards_" + kind + "_list.csv" }

	// New: born with the projection; the preview said so.
	pv, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, file("bundle"), data)
	if err != nil {
		t.Fatal(err)
	}
	if pv.List.Exists || pv.List.WillTag || strings.Join(pv.List.Tags, "|") != proj {
		t.Fatalf("preview of a list to create: %+v", pv.List)
	}
	prep, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file("bundle"), data, hash)
	if err != nil {
		t.Fatal(err)
	}
	if got := tags(prep.List.ID); got != proj {
		t.Fatalf("created list tags = %q", got)
	}

	// Existing untagged: the preview reports "will tag" and writes nothing; the confirm tags it.
	var untagged int
	h.db.Get(&untagged, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), '090426 Rewards-Product', 'private', 'single', '{keep}') RETURNING id`)
	pv, err = subimporter.Preview(ctx, h.db.DB, h.im, h.p, file("product"), data)
	if err != nil {
		t.Fatal(err)
	}
	if !pv.List.Exists || pv.List.ID != untagged || !pv.List.WillTag || pv.List.Brand != "rewards" || pv.List.From != "Rewards <hello@rewards.test>" {
		t.Fatalf("preview of an existing untagged list: %+v", pv.List)
	}
	if got := tags(untagged); got != "keep" {
		t.Fatalf("the preview wrote tags: %q", got)
	}
	prep, err = subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file("product"), data, hash)
	if err != nil {
		t.Fatal(err)
	}
	if prep.List.ID != untagged {
		t.Fatalf("existing list not reused: %+v", prep.List)
	}
	if got := tags(untagged); got != proj+"|keep" {
		t.Fatalf("existing untagged list after confirm: %q", got)
	}

	// Existing same brand: reused untouched, no "will tag".
	pv, err = subimporter.Preview(ctx, h.db.DB, h.im, h.p, file("product"), data)
	if err != nil {
		t.Fatal(err)
	}
	if !pv.List.Exists || pv.List.WillTag || len(pv.List.Tags) != 0 {
		t.Fatalf("preview of an existing same-brand list: %+v", pv.List)
	}
	before := tags(untagged)
	if _, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file("product"), data, hash); err != nil {
		t.Fatal(err)
	}
	if got := tags(untagged); got != before {
		t.Fatalf("same-brand list changed: %q -> %q", before, got)
	}

	// Existing other brand: refused at preview and at confirm, naming both, nothing written.
	var other int
	h.db.Get(&other, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), '090426 Rewards-Other', 'private', 'single', '{"brand:other","from:Other <hello@other.test>"}') RETURNING id`)
	l0, s0, m0 := h.counts()
	_, err = subimporter.Preview(ctx, h.db.DB, h.im, h.p, file("other"), data)
	var bc *subimporter.ListBrandConflictError
	if !errors.As(err, &bc) || bc.Have != "other" || bc.Want != "rewards" || bc.List != "090426 Rewards-Other" {
		t.Fatalf("preview of an other-brand list: %v", err)
	}
	if _, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file("other"), data, hash); !errors.As(err, &bc) {
		t.Fatalf("confirm of an other-brand list: %v", err)
	}
	if l1, s1, m1 := h.counts(); l1 != l0 || s1 != s0 || m1 != m0 {
		t.Fatalf("a refused import wrote: lists %d->%d subs %d->%d memberships %d->%d", l0, l1, s0, s1, m0, m1)
	}
	if got := tags(other); got != "brand:other|from:Other <hello@other.test>" {
		t.Fatalf("other-brand list changed: %q", got)
	}
}

// BRAND-PICKER-SPEC I20 -- the brand row is read at preview AND at confirm, never from a copy made
// at boot (or at preview): a row changed between preview and confirm is what the created list
// carries.
func TestPresetBrandReadLive(t *testing.T) {
	h := newPresetHarness(t)
	ctx := context.Background()
	data := []byte("earner_name,earner_email,earner_locale,earned_from_site_url\r\nAnn,ann@example.test,en,\r\n")
	file := "090426_rewards_live_list.csv"

	pv, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, file, data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pv.List.Tags, "|") != "brand:rewards|from:Rewards <hello@rewards.test>" {
		t.Fatalf("preview tags %v", pv.List.Tags)
	}

	h.db.MustExec(`UPDATE brands SET from_email = 'Rewards Club <hello@rewards.test>', site = 'https://shop.rewards.test' WHERE slug = 'rewards'`)

	prep, err := subimporter.PrepareImport(ctx, h.db.DB, h.im, h.p, file, data, subimporter.ContentHash(data))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	h.db.Select(&got, `SELECT unnest(tags) FROM lists WHERE id = $1 ORDER BY 1`, prep.List.ID)
	if strings.Join(got, "|") != "brand:rewards|from:Rewards Club <hello@rewards.test>|site:https://shop.rewards.test" {
		t.Fatalf("created list carries %v, want the row as changed after the preview", got)
	}

	// A row removed after load (SQL only -- the API has no DELETE) fails the preview by name.
	h.db.MustExec(`DELETE FROM brands WHERE slug = 'rewards'`)
	if _, err := subimporter.Preview(ctx, h.db.DB, h.im, h.p, file, data); err == nil || !strings.Contains(err.Error(), `"rewards"`) {
		t.Fatalf("preview with the row gone: %v", err)
	}
}
