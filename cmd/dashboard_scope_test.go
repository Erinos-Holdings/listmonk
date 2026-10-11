package main

// Fork (global brand) -- integrations GLOBAL-BRAND-SPEC I1 and I7, pure: dashboardListScope is the
// one decision the three Dashboard reads share. No list_id is today's branch per permission (the
// materialized views for blanket list access; the permitted lists, allCampaigns = get_all, for a
// list-scoped user); a list_id is the scoped queries over permitted ∩ list_id with allCampaigns
// FALSE, even for a campaigns:get_all user.

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
)

func TestDashboardListScope(t *testing.T) {
	superAdmin := baUser(nil)
	superAdmin.UserRoleID = auth.SuperAdminRoleID
	superAdmin.UserRole.ID = auth.SuperAdminRoleID

	hasAllGetAll := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll})
	manageAll := baUser([]string{auth.PermListManageAll})
	scoped := baUser([]string{auth.PermCampaignsGetAnalytics}, 3, 5)
	scopedGetAll := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, 3, 5)
	noLists := baUser([]string{auth.PermCampaignsGetAnalytics})

	ids := func(v ...string) url.Values { return url.Values{"list_id": v} }

	cases := []struct {
		name string
		u    auth.User
		q    url.Values
		want dashboardScope
	}{
		// I7: no list_id -- today's branch.
		{"hasAll, no param", hasAllGetAll, url.Values{}, dashboardScope{}},
		{"manage_all, no param", manageAll, url.Values{}, dashboardScope{}},
		{"super admin, no param", superAdmin, url.Values{}, dashboardScope{}},
		{"empty list_id slice is no param", hasAllGetAll, url.Values{"list_id": {}}, dashboardScope{}},
		{"list-scoped, no param", scoped, url.Values{}, dashboardScope{scoped: true, listIDs: []int{3, 5}, allCampaigns: false}},
		{"list-scoped get_all, no param", scopedGetAll, url.Values{}, dashboardScope{scoped: true, listIDs: []int{3, 5}, allCampaigns: true}},

		// I1: list_id -- permitted ∩ list_id, allCampaigns always false.
		{"hasAll get_all, list_id", hasAllGetAll, ids("23", "7"), dashboardScope{scoped: true, listIDs: []int{23, 7}, allCampaigns: false}},
		{"super admin, list_id", superAdmin, ids("23"), dashboardScope{scoped: true, listIDs: []int{23}, allCampaigns: false}},
		{"list-scoped, list_id inside", scoped, ids("5"), dashboardScope{scoped: true, listIDs: []int{5}, allCampaigns: false}},
		{"list-scoped get_all, list_id inside", scopedGetAll, ids("3"), dashboardScope{scoped: true, listIDs: []int{3}, allCampaigns: false}},
		{"list-scoped, list_id partly outside", scoped, ids("5", "23"), dashboardScope{scoped: true, listIDs: []int{5}, allCampaigns: false}},
		{"list-scoped, list_id outside", scoped, ids("23"), dashboardScope{scoped: true, listIDs: []int{}, allCampaigns: false}},
		{"no lists, list_id", noLists, ids("23"), dashboardScope{scoped: true, listIDs: []int{}, allCampaigns: false}},
	}
	for _, tc := range cases {
		got, err := dashboardListScope(tc.u, tc.q)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.scoped != tc.want.scoped || got.allCampaigns != tc.want.allCampaigns ||
			(tc.want.scoped && !reflect.DeepEqual(got.listIDs, tc.want.listIDs)) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
		// A scoped decision never carries a nil id set (the scoped queries bind an array).
		if got.scoped && got.listIDs == nil {
			t.Errorf("%s: scoped with nil listIDs", tc.name)
		}
	}

	if _, err := dashboardListScope(hasAllGetAll, ids("x")); err == nil {
		t.Fatal("list_id=x: want an error")
	}
}
