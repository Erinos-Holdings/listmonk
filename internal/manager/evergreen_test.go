package manager

import (
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

// Fork (evergreen) -- the scan gate: flag off never pipes; an idle mark holds the
// campaign out of the scan for evergreenIdle and then releases it.
func TestShouldPipeEvergreen(t *testing.T) {
	c := &models.Campaign{Base: models.Base{ID: 7}, Evergreen: true}

	m := &Manager{cfg: Config{EvergreenEnabled: false}, evergreenIdleUntil: map[int]time.Time{}}
	if m.shouldPipeEvergreen(c) {
		t.Fatal("flag off must never pipe an evergreen")
	}

	m.cfg.EvergreenEnabled = true
	if !m.shouldPipeEvergreen(c) {
		t.Fatal("flag on, no idle mark: must pipe")
	}

	m.markEvergreenIdle(c.ID)
	if m.shouldPipeEvergreen(c) {
		t.Fatal("freshly idle-marked: must not pipe")
	}

	m.evergreenIdleUntil[c.ID] = time.Now().Add(-time.Second)
	if !m.shouldPipeEvergreen(c) {
		t.Fatal("expired idle mark: must pipe again")
	}

	// A regular campaign's pipe never reaches evergreenCleanup's branch.
	p := &pipe{camp: &models.Campaign{Base: models.Base{ID: 8}}, m: m}
	if p.evergreenCleanup() {
		t.Fatal("regular campaign must not take the evergreen cleanup path")
	}
	p.camp.Evergreen = true
	if !p.evergreenCleanup() {
		t.Fatal("evergreen campaign must take the evergreen cleanup path")
	}
	if _, ok := m.evergreenIdleUntil[8]; !ok {
		t.Fatal("evergreen cleanup must mark the campaign idle")
	}
}
