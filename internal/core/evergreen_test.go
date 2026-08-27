package core

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/listmonk/models"
	null "gopkg.in/volatiletech/null.v6"
)

// Fork (evergreen) -- review C2/M1: evergreen flag and lists are frozen once started.
func TestEvergreenLockedChange(t *testing.T) {
	started := null.TimeFrom(time.Now())
	mk := func(evergreen, isStarted bool) models.Campaign {
		cm := models.Campaign{Evergreen: evergreen}
		if isStarted {
			cm.StartedAt = started
		}
		cm.Lists = types.JSONText(`[{"id":1,"name":"A"}]`)
		return cm
	}

	cases := []struct {
		name      string
		cm        models.Campaign
		evergreen bool
		listIDs   []int
		locked    bool
	}{
		{"draft evergreen may flip", mk(true, false), false, []int{1}, false},
		{"draft regular may become evergreen", mk(false, false), true, []int{1}, false},
		{"started evergreen -> regular", mk(true, true), false, []int{1}, true},
		{"started regular -> evergreen", mk(false, true), true, []int{1}, true},
		{"started evergreen same list", mk(true, true), true, []int{1}, false},
		{"started evergreen other list", mk(true, true), true, []int{2}, true},
		{"started evergreen extra list", mk(true, true), true, []int{1, 2}, true},
		{"started regular may change lists", mk(false, true), false, []int{2}, false},
	}
	for _, c := range cases {
		if got := EvergreenLockedChange(c.cm, c.evergreen, c.listIDs); got != c.locked {
			t.Errorf("%s: locked=%v want %v", c.name, got, c.locked)
		}
	}
}
