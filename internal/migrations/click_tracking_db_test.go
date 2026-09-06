package migrations

// Fork (click tracking) -- CLICK-TRACKING-SPEC T8 (I17: the v6.2.6 settings exist with their
// defaults after upgrade on a database that never had them) and T6-DB (I14: the attribute
// coverage count against a seeded list). Same opt-in harness (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"testing"
)

func TestV626SettingsDefaults(t *testing.T) {
	h := newEvergreenHarness(t)

	// A database that never had the keys: delete what schema.sql seeded, run the migration
	// twice (idempotent), and read the defaults back.
	h.db.MustExec(`DELETE FROM settings WHERE key IN ('app.link_fallback_url', 'app.utm_enable', 'app.utm_hosts', 'app.utm_params')`)
	lo := log.New(os.Stderr, "", 0)
	for i := 0; i < 2; i++ {
		if err := V6_2_6(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_6 run %d: %v", i+1, err)
		}
	}

	get := func(key string) string {
		var v string
		if err := h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = $1`, key); err != nil {
			t.Fatalf("setting %s: %v", key, err)
		}
		return v
	}
	if got := get("app.link_fallback_url"); got != `"https://curatedfor.you"` {
		t.Fatalf("app.link_fallback_url = %s", got)
	}
	if got := get("app.utm_enable"); got != "true" {
		t.Fatalf("app.utm_enable = %s (D9: ships enabled)", got)
	}
	if got := get("app.utm_hosts"); got != `["curatedfor.you", "curatedby.you"]` && got != `["curatedfor.you","curatedby.you"]` {
		t.Fatalf("app.utm_hosts = %s", got)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM settings WHERE key = 'app.utm_params' AND value ? 'utm_source' AND value ? 'utm_medium' AND value ? 'utm_campaign' AND value ? 'utm_content'`)
	if n != 1 {
		t.Fatalf("app.utm_params lacks the four D8 keys: %s", get("app.utm_params"))
	}

	// An existing value is never overwritten by the migration.
	h.db.MustExec(`UPDATE settings SET value = '"https://custom.test"' WHERE key = 'app.link_fallback_url'`)
	if err := V6_2_6(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	if got := get("app.link_fallback_url"); got != `"https://custom.test"` {
		t.Fatalf("migration overwrote an existing value: %s", got)
	}
}

// T6-DB -- get-campaign-attrib-coverage counts missing/empty keys against the campaign's
// actual targeting (opt-in status, blocklist, language) and reports zero when covered.
func TestAttribCoverageQuery(t *testing.T) {
	h := newEvergreenHarness(t)
	L := h.listA
	cov := h.prep("get-campaign-attrib-coverage")

	has := h.subscriber("has@x")
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"site":"https://shop.test/1"}' WHERE id = $1`, has)
	empty := h.subscriber("empty@x")
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"site":""}' WHERE id = $1`, empty)
	missing := h.subscriber("missing@x")
	unsub := h.subscriber("unsub@x")
	blocked := h.subscriber("blocked@x")
	h.db.MustExec(`UPDATE subscribers SET status = 'blocklisted' WHERE id = $1`, blocked)
	fr := h.subscriber("fr@x")
	h.setSubLang(fr, "fr")
	for _, s := range []int{has, empty, missing, blocked, fr} {
		h.join(s, L, "confirmed")
	}
	h.join(unsub, L, "unsubscribed")

	camp := h.campaign("cov", false, 0, L)

	type row struct {
		Missing int `db:"missing"`
		Total   int `db:"total"`
	}
	var r row
	if err := cov.Get(&r, camp, "site"); err != nil {
		t.Fatal(err)
	}
	// Targeted: has, empty, missing, fr (unsubscribed and blocklisted are not). Missing: empty, missing, fr.
	if r.Total != 4 || r.Missing != 3 {
		t.Fatalf("site coverage = %+v, want missing 3 of 4", r)
	}

	// Language scoping narrows the audience the same way the send does.
	h.setCampLang(camp, "fr")
	if err := cov.Get(&r, camp, "site"); err != nil {
		t.Fatal(err)
	}
	if r.Total != 1 || r.Missing != 1 {
		t.Fatalf("fr-scoped coverage = %+v, want 1 of 1 missing", r)
	}
	h.setCampLang(camp, "")

	// A fully covered key is silent (zero missing).
	h.db.MustExec(`UPDATE subscribers SET attribs = attribs || '{"site":"https://shop.test/x"}'`)
	if err := cov.Get(&r, camp, "site"); err != nil {
		t.Fatal(err)
	}
	if r.Missing != 0 || r.Total != 4 {
		t.Fatalf("covered key = %+v, want 0 missing of 4", r)
	}

	// An unknown key counts everyone as missing.
	if err := cov.Get(&r, camp, "nope"); err != nil {
		t.Fatal(err)
	}
	if r.Missing != 4 {
		t.Fatalf("unknown key = %+v", r)
	}
}
