package migrations

// Fork (brand unsubscribe) -- integrations listmonk/BRAND-UNSUBSCRIBE-SPEC D1/D2 (the migration
// half): v6.2.33 brings a v6.2.32 database to exactly schema.sql's shape (the
// brand_marketing_list_ids and campaign_brand function bodies) and is idempotent. Same opt-in
// harness as evergreen_db_test.go (LISTMONK_TEST_PG). The behaviour is
// cmd/subscribers_brand_unsub_db_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func (h *evergreenHarness) brandUnsubShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		WHERE p.proname IN ('brand_marketing_list_ids', 'campaign_brand') ORDER BY p.proname`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(fn, "; ")
}

func TestBrandUnsubscribeMigration_V6_2_33(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.brandUnsubShape()
	for _, want := range []string{
		"brand_marketing_list_ids(p_brand text)", "campaign_brand(camp_uuid uuid)",
		"transactional = FALSE", "COUNT(DISTINCT list_brand_tag",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.32 database: neither function exists.
	h.db.MustExec(`DROP FUNCTION brand_marketing_list_ids(TEXT)`)
	h.db.MustExec(`DROP FUNCTION campaign_brand(UUID)`)
	if got := h.brandUnsubShape(); got != "" {
		t.Fatalf("functions survived the drop: %s", got)
	}

	for i := 0; i < 2; i++ {
		if err := V6_2_33(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_33 run %d: %v", i+1, err)
		}
	}
	if got := h.brandUnsubShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	// The definitions work on real rows: a transactional list of the brand is out, an untagged
	// list belongs to no brand, and a campaign spanning two brands resolves to NULL.
	news := h.list("news", "public", []string{"brand:acme"}, "")
	step := h.list("step", "private", []string{"brand:acme"}, "")
	txn := h.list("txn", "private", []string{"brand:acme"}, "")
	other := h.list("other", "public", []string{"brand:zeta"}, "")
	untagged := h.list("untagged", "public", []string{}, "")
	h.db.MustExec(`UPDATE lists SET transactional = TRUE WHERE id = $1`, txn)

	var ids pq.Int64Array
	if err := h.db.Get(&ids, `SELECT brand_marketing_list_ids('acme')`); err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 2 || !got[int64(news)] || !got[int64(step)] {
		t.Fatalf("brand_marketing_list_ids('acme') = %v, want {%d,%d}", ids, news, step)
	}
	if err := h.db.Get(&ids, `SELECT brand_marketing_list_ids('nope')`); err != nil || len(ids) != 0 {
		t.Fatalf("unknown brand: %v %v", ids, err)
	}
	if err := h.db.Get(&ids, `SELECT brand_marketing_list_ids(NULL)`); err != nil || len(ids) != 0 {
		t.Fatalf("NULL brand: %v %v", ids, err)
	}

	for name, c := range map[string][]int{
		"one brand":         {news, step},
		"two brands":        {news, other},
		"tagged + untagged": {news, untagged},
		"untagged only":     {untagged},
	} {
		camp := h.campaign("c-"+name, false, 0, c...)
		var uu string
		h.db.Get(&uu, `SELECT uuid::text FROM campaigns WHERE id = $1`, camp)
		var brand *string
		if err := h.db.Get(&brand, `SELECT campaign_brand($1::UUID)`, uu); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		switch name {
		case "one brand", "tagged + untagged":
			if brand == nil || *brand != "acme" {
				t.Fatalf("%s: campaign_brand = %v, want acme", name, brand)
			}
		default:
			if brand != nil {
				t.Fatalf("%s: campaign_brand = %v, want NULL", name, *brand)
			}
		}
	}
}
