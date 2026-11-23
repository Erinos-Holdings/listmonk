package core

// Fork (brand roles) -- integrations listmonk/BRAND-ROLES-SPEC I2 (the setupUserFields half): a
// resolved list_role_perms entry carrying list:analytics lands in AnalyticsListIDs ONLY -- never in
// ListPermissionsMap, GetListIDs or ManageListIDs -- while a manage entry fills all three; the
// profile keeps both shapes (ListRole.Lists with every entry and its brand, ListRole.Brands). The
// User methods are internal/auth/models_test.go. Pure: no database.

import (
	"encoding/json"
	"io"
	"log"
	"reflect"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
)

func TestSetupUserFieldsAnalyticsEntries(t *testing.T) {
	c := &Core{log: log.New(io.Discard, "", 0)}

	perms := json.RawMessage(`[
		{"id": 1, "name": "Shala news", "brand": "shala", "permissions": ["list:get", "list:manage"]},
		{"id": 2, "name": "Ruze news", "brand": "ruze", "permissions": ["list:analytics"]},
		{"id": 3, "name": "Odd", "brand": "ruze", "permissions": ["list:analytics", "list:get", "list:manage"]}
	]`)
	brands := json.RawMessage(`[{"brand": "shala", "level": "manage"}, {"brand": null, "level": "analytics"}]`)
	roleID := 9
	u := auth.User{Type: auth.UserTypeUser, ListRoleID: &roleID, ListsPermsRaw: &perms, ListBrandsRaw: &brands}

	got := c.setupUserFields([]auth.User{u})[0]

	if !reflect.DeepEqual(got.GetListIDs, []int{1}) || !reflect.DeepEqual(got.ManageListIDs, []int{1}) {
		t.Fatalf("GetListIDs %v ManageListIDs %v, want [1] [1]", got.GetListIDs, got.ManageListIDs)
	}
	if !reflect.DeepEqual(got.AnalyticsListIDs, []int{2, 3}) {
		t.Fatalf("AnalyticsListIDs = %v, want [2 3]", got.AnalyticsListIDs)
	}
	for _, id := range []int{2, 3} {
		if _, ok := got.ListPermissionsMap[id]; ok {
			t.Fatalf("analytics list %d is in ListPermissionsMap: %v", id, got.ListPermissionsMap)
		}
	}
	if _, ok := got.ListPermissionsMap[1][auth.PermListManage]; !ok {
		t.Fatalf("manage list missing from ListPermissionsMap: %v", got.ListPermissionsMap)
	}

	// The profile keeps every resolved entry (with its brand) and the grants.
	if got.ListRole == nil || len(got.ListRole.Lists) != 3 || got.ListRole.Lists[1].Brand != "ruze" {
		t.Fatalf("ListRole.Lists = %+v", got.ListRole)
	}
	if len(got.ListRole.Brands) != 2 || got.ListRole.Brands[1].Brand != nil || got.ListRole.Brands[1].Level != auth.BrandLevelAnalytics {
		t.Fatalf("ListRole.Brands = %+v", got.ListRole.Brands)
	}

	// A role with no grants: empty, never null, arrays in the profile JSON.
	empty := json.RawMessage(`[]`)
	u2 := auth.User{Type: auth.UserTypeUser, ListRoleID: &roleID, ListsPermsRaw: &empty, ListBrandsRaw: &empty}
	g2 := c.setupUserFields([]auth.User{u2})[0]
	b, _ := json.Marshal(g2.ListRole)
	if string(b) != `{"id":9,"name":"","lists":[],"brands":[]}` {
		t.Fatalf("empty role JSON = %s", b)
	}
}
