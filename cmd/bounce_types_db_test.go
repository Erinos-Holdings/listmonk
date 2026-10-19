package main

// Fork (bounce types) -- integrations BOUNCE-TYPES-SPEC I1 and I2 against a real database, through
// the handler: type=bounce_types counts raw bounce rows per type as the Bounces line does (so the
// split sums to the line's total), orders the rows hard, soft, complaint and leaves a zero type
// out; it is refused exactly where type=bounces is refused. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go) and the
// brand-analytics users.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
)

// bounceTypeRows renders a bounce_types response body as "type=count ..." in response order.
func bounceTypeRows(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Data []models.CampaignAnalyticsBounceType `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	out := make([]string, 0, len(resp.Data))
	for _, r := range resp.Data {
		out = append(out, fmt.Sprintf("%s=%d", r.Type, r.Count))
	}
	return strings.Join(out, " ")
}

// TestBounceTypesAggregation is I1 (as adjudicated -- raw rows, the Bounces line's count), through
// the handler.
func TestBounceTypesAggregation(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	db := h.db

	list := h.baList("Bounce", "public", "single")
	a := h.baCamp("A", "finished", 10, list)
	b := h.baCamp("B", "finished", 10, list)
	other := h.baCamp("Other", "finished", 10, list)
	s1 := h.baSub("s1@x", "enabled", map[int]string{list: "confirmed"})
	s2 := h.baSub("s2@x", "enabled", map[int]string{list: "confirmed"})
	s3 := h.baSub("s3@x", "enabled", map[int]string{list: "confirmed"})
	s4 := h.baSub("s4@x", "enabled", map[int]string{list: "confirmed"})

	// ago = days before now; outside the request's 48-hour window when 30.
	bounce := func(camp, sub int, typ string, times, ago int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO bounces (subscriber_id, campaign_id, type, created_at) VALUES ($1, $2, $3::bounce_type, NOW() - ($4 || ' days')::INTERVAL)`,
				sub, camp, typ, fmt.Sprint(ago))
		}
	}

	// s1 bounces twice with the same type on A (two raw rows, counted twice) and once on B; s2 has
	// a hard and a soft on A (once in each type, twice in the total); s3 a complaint on B; s4 rows outside the campaign filter and the date window.
	bounce(a, s1, "hard", 2, 0)
	bounce(b, s1, "hard", 1, 0)
	bounce(a, s2, "hard", 1, 0)
	bounce(a, s2, "soft", 1, 0)
	bounce(b, s3, "complaint", 1, 0)
	bounce(other, s4, "soft", 1, 0)
	bounce(a, s4, "complaint", 1, 30)

	member := baUser([]string{auth.PermCampaignsGetAnalytics}, list)
	get := func(typ string, ids ...int) string {
		t.Helper()
		code, body := analyticsMultiReq(t, h, member, typ, ids...)
		if code != http.StatusOK {
			t.Fatalf("%s %v: %d %s", typ, ids, code, body)
		}
		return body
	}
	// lineTotal sums the bounces counts type's buckets (the Bounces line's total).
	lineTotal := func(ids ...int) int {
		t.Helper()
		var resp struct {
			Data []models.CampaignAnalyticsCount `json:"data"`
		}
		body := get("bounces", ids...)
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("bounces body %s: %v", body, err)
		}
		n := 0
		for _, r := range resp.Data {
			n += r.Count
		}
		return n
	}
	splitSum := func(body string) int {
		t.Helper()
		var resp struct {
			Data []models.CampaignAnalyticsBounceType `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("bounce_types body %s: %v", body, err)
		}
		n := 0
		for _, r := range resp.Data {
			n += r.Count
		}
		return n
	}

	// Raw rows (the harness's individual tracking ON does not change the count): s1's repeat hard
	// bounce counts twice on A, s2 once in hard and once in soft.
	for _, tc := range []struct {
		name string
		ids  []int
		want string
	}{
		{"A+B", []int{a, b}, "hard=4 soft=1 complaint=1"},
		// A zero type is absent: campaign A alone has no complaint in the window.
		{"A only", []int{a}, "hard=3 soft=1"},
	} {
		body := get("bounce_types", tc.ids...)
		if got := bounceTypeRows(t, body); got != tc.want {
			t.Fatalf("%s counts\n got  %s\n want %s", tc.name, got, tc.want)
		}
		// The split sums exactly to the Bounces line's total for the same selection and range.
		if sum, line := splitSum(body), lineTotal(tc.ids...); sum != line {
			t.Fatalf("%s: split sums to %d, Bounces line total %d", tc.name, sum, line)
		}
	}
}

// TestBounceTypesPermission is I2: type=bounce_types answers with the status type=bounces answers
// for the same user and ids, through the same a.auth.Perm wrapper handlers.go registers, and 400s on
// the same bad dates with the same message.
func TestBounceTypesPermission(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)

	var (
		memberA  = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
		memberB  = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listB)
		noPerm   = baUser(nil, f.listA, f.listB)
		bothList = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA, f.listB)
	)
	for _, tc := range []struct {
		name string
		u    auth.User
		ids  []int
		want int
	}{
		{"member A on A", memberA, []int{f.campA}, http.StatusOK},
		{"member A on A+AB", memberA, []int{f.campA, f.campAB}, http.StatusOK},
		{"member B on A", memberB, []int{f.campA}, http.StatusForbidden},
		{"member A on A+B (one unpermitted)", memberA, []int{f.campA, f.campB}, http.StatusForbidden},
		{"no analytics permission", noPerm, []int{f.campA}, http.StatusForbidden},
		{"both lists on A+B", bothList, []int{f.campA, f.campB}, http.StatusOK},
	} {
		bc, bb := analyticsMultiReq(t, h, tc.u, "bounces", tc.ids...)
		tc2, tb := analyticsMultiReq(t, h, tc.u, "bounce_types", tc.ids...)
		if bc != tc.want || tc2 != bc {
			t.Fatalf("%s: bounces %d (%s), bounce_types %d (%s), want both %d", tc.name, bc, bb, tc2, tb, tc.want)
		}
	}

	// Bad dates: the same 400 for both types (and, for a refused user, the same 403 first).
	for _, tc := range []struct {
		name     string
		u        auth.User
		from, to string
		want     int
	}{
		{"short from", memberA, "2026", time.Now().UTC().Format(time.RFC3339), http.StatusBadRequest},
		{"missing to", memberA, time.Now().UTC().Format(time.RFC3339), "", http.StatusBadRequest},
		{"overlong", memberA, strings.Repeat("2", 31), strings.Repeat("2", 31), http.StatusBadRequest},
		{"refused user, bad dates", memberB, "x", "y", http.StatusForbidden},
	} {
		req := func(typ string) (int, string) {
			q := url.Values{}
			q.Set("id", fmt.Sprint(f.campA))
			q.Set("from", tc.from)
			q.Set("to", tc.to)
			return baCall(t, tc.u, "/api/campaigns/analytics/"+typ+"?"+q.Encode(),
				h.app.auth.Perm(h.app.GetCampaignViewAnalytics, "campaigns:get_analytics"), "type", typ)
		}
		bc, bb := req("bounces")
		tc2, tb := req("bounce_types")
		if bc != tc.want || tc2 != bc || tb != bb {
			t.Fatalf("%s: bounces %d (%s), bounce_types %d (%s), want both %d with the same message", tc.name, bc, bb, tc2, tb, tc.want)
		}
	}
}
