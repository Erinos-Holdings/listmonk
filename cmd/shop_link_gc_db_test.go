package main

// Fork (shop-link frozen) -- integrations SHOP-LINK-FROZEN-SPEC I5 against a real database, through
// the GCCampaignAnalytics handler (DELETE /api/maintenance/analytics/:type?before_date=...): types
// clicks and all delete both shop-link snapshot tables' rows for campaigns whose summary taken_at is
// before the date and keep later ones; views touches neither. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestShopLinkSnapshotGC(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	newCamp := func(name string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldCamp, newCampID := newCamp("Old"), newCamp("New")

	// seed writes a two-recipient snapshot for a campaign taken at the given time.
	seed := func(camp int, takenAt string) {
		db.MustExec(`INSERT INTO campaign_shop_link_audience (campaign_id, brand, kind, audience, taken_at) VALUES ($1, 'ruze', 'self', 2, $2::TIMESTAMPTZ)`, camp, takenAt)
		db.MustExec(`INSERT INTO campaign_shop_link_recipients (campaign_id, subscriber_id, brand, kind) VALUES ($1, 1, 'ruze', 'self'), ($1, 2, 'ruze', 'self')`, camp)
	}
	counts := func(camp int) string {
		var a, r int
		db.Get(&a, `SELECT COUNT(*) FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp)
		db.Get(&r, `SELECT COUNT(*) FROM campaign_shop_link_recipients WHERE campaign_id = $1`, camp)
		return fmt.Sprintf("summary=%d recipients=%d", a, r)
	}
	gc := func(typ string) {
		t.Helper()
		q := url.Values{}
		q.Set("before_date", "2026-06-01T00:00:00Z")
		req := httptest.NewRequest(http.MethodDelete, "/api/maintenance/analytics/"+typ+"?"+q.Encode(), nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("type")
		c.SetParamValues(typ)
		if err := h.app.GCCampaignAnalytics(c); err != nil {
			t.Fatalf("GC %s: %v", typ, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("GC %s: %d %s", typ, rec.Code, rec.Body.String())
		}
	}

	seed(oldCamp, "2026-01-01T00:00:00Z")
	seed(newCampID, "2026-10-01T00:00:00Z")

	// views touches neither table.
	gc("views")
	if got := counts(oldCamp); got != "summary=1 recipients=2" {
		t.Fatalf("views GC touched the old snapshot: %s", got)
	}

	for _, typ := range []string{"clicks", "all"} {
		gc(typ)
		if got := counts(oldCamp); got != "summary=0 recipients=0" {
			t.Fatalf("%s GC left the old snapshot: %s", typ, got)
		}
		if got := counts(newCampID); got != "summary=1 recipients=2" {
			t.Fatalf("%s GC touched the later snapshot: %s", typ, got)
		}
		// Re-seed for the next type.
		seed(oldCamp, "2026-01-01T00:00:00Z")
	}
}
