package main

// Fork (campaign status filter) -- integrations listmonk/CAMPAIGN-STATUS-FILTER-SPEC I5 (and I6's
// filter coverage) against a real database, through the handlers: for an all-permission user and
// a list-restricted user, and under no filter, evergreen, list_id, lang, tag and search filters,
// every count GET /api/campaigns/statuses returns equals GET /api/campaigns' total for that status
// under the same params, and a status with no campaign is absent. The status param is ignored by
// the counts; an invalid list_id or lang is a 400 on both endpoints. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go) and baUser / baCall
// (brand_analytics_db_test.go). Fixture names are example.test only.

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
)

var allCampaignStatuses = []string{"draft", "running", "scheduled", "paused", "cancelled", "finished"}

func TestCampaignStatusCounts(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	listA := h.baList("Status A", "private", "single")
	listB := h.baList("Status B", "private", "single")

	mk := func(name, status string, evergreen bool, lang string, tags string, lists ...int) int {
		t.Helper()
		attribs := `{}`
		if lang != "" {
			attribs = `{"lang":"` + lang + `"}`
		}
		var id int
		if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, status, evergreen, attribs, tags)
			VALUES (gen_random_uuid(), $1, 's', 'Acme <hello@example.test>', '<p>b</p>', 'email', $2, $3, $4::JSONB, $5::VARCHAR(100)[]) RETURNING id`,
			name, status, evergreen, attribs, tags); err != nil {
			t.Fatal(err)
		}
		for _, l := range lists {
			h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, l)
		}
		return id
	}
	// No cancelled campaign at all: absent under every scope and filter.
	idDraft := mk("Spring draft", "draft", false, "en", "{}", listA)
	mk("Spring send", "running", false, "fr", "{}", listA)
	idPausedB := mk("Paused B", "paused", false, "en", "{}", listB)
	mk("Paused welcome", "paused", true, "", "{}", listA, listB)
	mk("Finished B", "finished", false, "fr", "{}", listB)
	mk("Scheduled welcome", "scheduled", true, "de", "{}", listA)
	mk("Orphan finished", "finished", false, "en", "{}")
	mk("Summer promo", "draft", false, "en", "{promo}", listB)
	mk("Paused A", "paused", false, "fr", "{promo}", listA)

	users := map[string]auth.User{
		"all":        baUser([]string{auth.PermCampaignsGetAll}),
		"restricted": baUser([]string{auth.PermCampaignsGet}, listA),
	}
	a, b := strconv.Itoa(listA), strconv.Itoa(listB)
	filters := []string{
		"",
		"evergreen=true",
		"evergreen=false",
		"list_id=" + a,
		"list_id=" + b,
		"list_id=" + a + "&list_id=" + b,
		"lang=fr",
		"lang=en&evergreen=false",
		"tag=promo",
		"query=paused",
		"query=spring&list_id=" + a,
		// The route id scope (the page sends it to the counts too) and a search the cleaner rewrites.
		"id=" + strconv.Itoa(idDraft) + "&id=" + strconv.Itoa(idPausedB),
		"query=spring-send",
	}

	counts := func(u auth.User, q string) map[string]int {
		t.Helper()
		code, body := baCall(t, u, "/api/campaigns/statuses?"+q, h.app.GetCampaignStatuses)
		if code != http.StatusOK {
			t.Fatalf("statuses %q: %d %s", q, code, body)
		}
		var resp struct {
			Data []struct {
				Status string `json:"status"`
				Count  int    `json:"count"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode %q: %v: %s", q, err, body)
		}
		out := map[string]int{}
		prev := -1
		for _, r := range resp.Data {
			if r.Count <= 0 {
				t.Fatalf("statuses %q: zero count returned for %s", q, r.Status)
			}
			if _, dup := out[r.Status]; dup {
				t.Fatalf("statuses %q: %s twice", q, r.Status)
			}
			idx := -1
			for i, s := range allCampaignStatuses {
				if s == r.Status {
					idx = i
				}
			}
			if idx <= prev {
				t.Fatalf("statuses %q: not in enum order: %s", q, body)
			}
			prev = idx
			out[r.Status] = r.Count
		}
		return out
	}
	listTotal := func(u auth.User, q, status string) int {
		t.Helper()
		path := "/api/campaigns?per_page=all&no_body=true&status=" + status
		if q != "" {
			path += "&" + q
		}
		code, body := baCall(t, u, path, h.app.GetCampaigns)
		if code != http.StatusOK {
			t.Fatalf("list %q: %d %s", path, code, body)
		}
		var resp struct {
			Data struct {
				Total   int               `json:"total"`
				Results []json.RawMessage `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode %q: %v: %s", path, err, body)
		}
		if len(resp.Data.Results) != resp.Data.Total {
			t.Fatalf("list %q: %d rows, total %d", path, len(resp.Data.Results), resp.Data.Total)
		}
		return resp.Data.Total
	}

	nonzero := 0
	for un, u := range users {
		for _, q := range filters {
			got := counts(u, q)
			for _, s := range allCampaignStatuses {
				want := listTotal(u, q, s)
				if got[s] != want {
					t.Errorf("%s %q: %s count %d, list total %d", un, q, s, got[s], want)
				}
				if want > 0 {
					nonzero++
				}
			}
			if _, ok := got["cancelled"]; ok {
				t.Errorf("%s %q: cancelled present with no cancelled campaign", un, q)
			}
			// The status param is ignored by the counts.
			if withStatus := counts(u, q+"&status=paused"); !reflect.DeepEqual(withStatus, got) {
				t.Errorf("%s %q: status param changed the counts: %v vs %v", un, q, withStatus, got)
			}
		}
	}
	if nonzero == 0 {
		t.Fatal("I5 vacuous: every count zero")
	}

	// The scopes differ on the fixture (the restricted user never sees list B only or orphan rows).
	if all, res := counts(users["all"], ""), counts(users["restricted"], ""); all["finished"] != 2 || res["finished"] != 0 || res["paused"] != 2 {
		t.Fatalf("scope fixture: all %v restricted %v", all, res)
	}

	// A 400 on one is a 400 on the other.
	for _, q := range []string{"list_id=x", "id=x", "lang=xx", "evergreen=maybe"} {
		c1, _ := baCall(t, users["all"], "/api/campaigns/statuses?"+q, h.app.GetCampaignStatuses)
		c2, _ := baCall(t, users["all"], "/api/campaigns?"+q, h.app.GetCampaigns)
		if c1 != http.StatusBadRequest || c2 != http.StatusBadRequest {
			t.Errorf("%q: statuses %d, list %d, want 400 on both", q, c1, c2)
		}
	}
}
