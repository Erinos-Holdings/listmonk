package auth

// Fork (brand roles) -- integrations listmonk/BRAND-ROLES-SPEC I2 (the User half): a list reached
// only at the analytics level is in AnalyticsListIDs and nowhere else, so HasListPerm (get and
// manage), GetPermittedLists (every type combination), FilterListsByPerm (every type combination)
// and GetPermittedListIDs never return it -- while AnalyticsScope and FilterAnalyticsLists do. The
// setupUserFields half (that a resolved list:analytics entry lands only in AnalyticsListIDs) is
// internal/core/users_test.go. Pure tests.

import (
	"reflect"
	"sort"
	"testing"
)

const (
	mgList  = 11 // manage-level list (list:get + list:manage)
	anList  = 22 // analytics-level list
	anList2 = 23 // a second analytics-level list
)

// brandRoleUser is a loaded user as setupUserFields leaves it for a brand role resolving to one
// manage list and two analytics lists.
func brandRoleUser(perms ...string) User {
	u := User{
		PermissionsMap:     map[string]struct{}{},
		ListPermissionsMap: map[int]map[string]struct{}{mgList: {PermListGet: {}, PermListManage: {}}},
		GetListIDs:         []int{mgList},
		ManageListIDs:      []int{mgList},
		AnalyticsListIDs:   []int{anList, anList2},
	}
	for _, p := range perms {
		u.PermissionsMap[p] = struct{}{}
	}
	return u
}

func sorted(in []int) []int {
	out := append([]int{}, in...)
	sort.Ints(out)
	return out
}

func containsAny(ids []int, want ...int) bool {
	for _, id := range ids {
		for _, w := range want {
			if id == w {
				return true
			}
		}
	}
	return false
}

func TestAnalyticsListsAreNeverListAccess(t *testing.T) {
	u := brandRoleUser("campaigns:get", "campaigns:manage", "subscribers:get", "subscribers:manage")
	types := []PermType{PermTypeGet, PermTypeManage, PermTypeGet | PermTypeManage}

	for _, ty := range types {
		if err := u.HasListPerm(ty, anList); err == nil {
			t.Errorf("HasListPerm(%d, analytics list) allowed", ty)
		}
		if err := u.HasListPerm(ty, mgList, anList); err == nil {
			t.Errorf("HasListPerm(%d, manage + analytics list) allowed", ty)
		}
		if err := u.HasListPerm(ty, mgList); err != nil {
			t.Errorf("HasListPerm(%d, manage list) refused: %v", ty, err)
		}

		all, ids := u.GetPermittedLists(ty)
		if all || containsAny(ids, anList, anList2) {
			t.Errorf("GetPermittedLists(%d) = %v %v, holds an analytics list", ty, all, ids)
		}
		if !containsAny(ids, mgList) {
			t.Errorf("GetPermittedLists(%d) = %v, lacks the manage list", ty, ids)
		}

		if got := u.FilterListsByPerm(ty, []int{mgList, anList, anList2}); containsAny(got, anList, anList2) {
			t.Errorf("FilterListsByPerm(%d) = %v, holds an analytics list", ty, got)
		}
	}

	if got := u.GetPermittedListIDs([]int{anList}); containsAny(got, anList, anList2) {
		t.Errorf("GetPermittedListIDs([analytics]) = %v", got)
	}
	if got := u.GetPermittedListIDs(nil); containsAny(got, anList, anList2) {
		t.Errorf("GetPermittedListIDs(nil) = %v (the fallback must be the get lists only)", got)
	}
	if got := u.GetPermittedListIDs([]int{mgList, anList}); !reflect.DeepEqual(got, []int{mgList}) {
		t.Errorf("GetPermittedListIDs([manage, analytics]) = %v, want [%d]", got, mgList)
	}

	// An analytics-only user (no get or manage list at all) has no list access anywhere.
	ao := User{PermissionsMap: map[string]struct{}{"campaigns:get_analytics": {}}, ListPermissionsMap: map[int]map[string]struct{}{},
		AnalyticsListIDs: []int{anList}}
	for _, ty := range types {
		if ao.HasListPerm(ty, anList) == nil {
			t.Errorf("analytics-only: HasListPerm(%d) allowed", ty)
		}
		if all, ids := ao.GetPermittedLists(ty); all || len(ids) != 0 {
			t.Errorf("analytics-only: GetPermittedLists(%d) = %v %v", ty, all, ids)
		}
	}
	if got := ao.GetPermittedListIDs([]int{anList}); !reflect.DeepEqual(got, []int{-1}) {
		t.Errorf("analytics-only: GetPermittedListIDs = %v, want [-1]", got)
	}
}

func TestAnalyticsScope(t *testing.T) {
	u := brandRoleUser("campaigns:get")
	all, ids := u.AnalyticsScope()
	if all || !reflect.DeepEqual(sorted(ids), []int{mgList, anList, anList2}) {
		t.Fatalf("AnalyticsScope = %v %v, want the manage list and both analytics lists", all, ids)
	}
	if got := u.FilterAnalyticsLists([]int{anList, 999, mgList}); !reflect.DeepEqual(got, []int{anList, mgList}) {
		t.Fatalf("FilterAnalyticsLists = %v", got)
	}
	if got := u.FilterAnalyticsLists([]int{999}); got == nil || len(got) != 0 {
		t.Fatalf("FilterAnalyticsLists(out of scope) = %#v, want an empty non-nil slice", got)
	}

	// Blanket list access short-circuits; ids pass unchanged.
	for _, p := range []string{PermListGetAll, PermListManageAll} {
		b := brandRoleUser(p)
		if all, ids := b.AnalyticsScope(); !all || ids != nil {
			t.Fatalf("%s: AnalyticsScope = %v %v", p, all, ids)
		}
		if got := b.FilterAnalyticsLists([]int{5, 6}); !reflect.DeepEqual(got, []int{5, 6}) {
			t.Fatalf("%s: FilterAnalyticsLists = %v", p, got)
		}
	}
	sa := brandRoleUser()
	sa.UserRoleID = SuperAdminRoleID
	if all, _ := sa.AnalyticsScope(); !all {
		t.Fatal("Super Admin: AnalyticsScope not all")
	}

	// No duplicates when a list is somehow in both halves.
	d := brandRoleUser()
	d.AnalyticsListIDs = append(d.AnalyticsListIDs, mgList)
	if _, ids := d.AnalyticsScope(); len(ids) != 3 {
		t.Fatalf("AnalyticsScope duplicates: %v", ids)
	}
}
