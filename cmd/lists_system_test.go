package main

// Fork (campaign lists) -- integrations listmonk/CAMPAIGN-LISTS-SPEC I1 and I2: the hidden
// system-list set (isHiddenSystemList, D2/D6) over every I1 case, and the one per-list setter that
// puts journey_locked and system_hidden on every list response together (D6). Pure; no database.
// The same case table is the fixture of the D7 agreement test (lists_system_db_test.go).

import (
	"testing"

	"github.com/knadh/listmonk/models"
)

// systemListCase is one I1 case: a list's stored trigger (raw JSON, "" = none) and tags, and
// whether it is hidden.
type systemListCase struct {
	name    string
	trigger string
	tags    []string
	hidden  bool
}

// systemListCases is every I1 case (also the D7 SQL agreement fixture).
var systemListCases = []systemListCase{
	{"first_order trigger", `{"kind":"first_order"}`, nil, true},
	{"no_purchase trigger", `{"kind":"no_purchase","days":2}`, nil, true},
	{"post_purchase trigger", `{"kind":"post_purchase","days":14}`, nil, true},
	{"queue 3", "", []string{"reengage:5:3"}, true},
	{"queue 4", "", []string{"brand:acme", "reengage:5:4"}, true},
	{"malformed reengage:x", "", []string{"reengage:x"}, true},
	{"malformed reengage:5:5", "", []string{"reengage:5:5"}, true},
	{"padded queue 3", "", []string{" reengage:5:3"}, true},
	{"standard trigger plus a win-back tag", `{"kind":"first_order"}`, []string{"reengage:5:1"}, true},
	{"win-back and final tags together", "", []string{"reengage:5:2", "reengage:5:4"}, true},
	{"queue 1", "", []string{"reengage:5:1"}, false},
	{"queue 2", "", []string{"brand:acme", "reengage:5:2"}, false},
	{"padded queue 1", "", []string{" reengage:5:1 "}, false},
	{"padded queue 2", "", []string{"reengage:12:2 "}, false},
	{"sku trigger", `{"kind":"skus","skus":["widget"]}`, nil, false},
	{"no trigger, no tags", "", nil, false},
	{"internal tag", "", []string{"internal"}, false},
	{"test tag", "", []string{"test"}, false},
}

func (c systemListCase) list() models.List {
	l := models.List{Name: c.name, Tags: c.tags}
	if c.trigger != "" {
		l.JoinTrigger = models.ListJSON(c.trigger)
	}
	return l
}

// I1 -- the hidden set.
func TestIsHiddenSystemList(t *testing.T) {
	for _, c := range systemListCases {
		if got := isHiddenSystemList(c.list()); got != c.hidden {
			t.Errorf("%s: isHiddenSystemList = %v, want %v", c.name, got, c.hidden)
		}
	}

	// A typed-but-unstored trigger is no trigger: only the stored join_trigger is read. The list
	// struct carries the stored value only (the request binds its own listReq), so a list whose
	// stored trigger is empty is not hidden whatever a request typed.
	var req listReq
	req.List = models.List{Name: "typed"}
	req.JoinTrigger = []byte(`{"kind":"first_order"}`)
	if isHiddenSystemList(req.List) {
		t.Errorf("a list with no stored trigger is hidden")
	}

	// The hidden set is the journey-locked set minus the two win-back queues: every hidden list is
	// locked, and a locked list that is not hidden is a win-back queue.
	for _, c := range systemListCases {
		l := c.list()
		if c.hidden && !isJourneyLockedList(l) {
			t.Errorf("%s: hidden but not journey-locked", c.name)
		}
	}
}

// I2 -- the one setter: markJourneyLocked (the listings) and decorateList (the single read, create
// and update responses) both set journey_locked and system_hidden, from the two definitions.
func TestListFlagsSetTogether(t *testing.T) {
	lists := make([]models.List, 0, len(systemListCases))
	for _, c := range systemListCases {
		lists = append(lists, c.list())
	}
	markJourneyLocked(lists)
	a := &App{}
	for i, c := range systemListCases {
		if lists[i].SystemHidden != c.hidden || lists[i].JourneyLocked != isJourneyLockedList(lists[i]) {
			t.Errorf("%s: markJourneyLocked set hidden=%v locked=%v", c.name, lists[i].SystemHidden, lists[i].JourneyLocked)
		}
		one := c.list()
		if err := a.decorateList(&one, false); err != nil {
			t.Fatal(err)
		}
		if one.SystemHidden != c.hidden || one.JourneyLocked != isJourneyLockedList(one) {
			t.Errorf("%s: decorateList set hidden=%v locked=%v", c.name, one.SystemHidden, one.JourneyLocked)
		}
	}
}
