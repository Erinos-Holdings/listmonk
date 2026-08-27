package manager

import (
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

// fakeStore records the evergreen claim bookkeeping calls the worker makes.
type fakeStore struct {
	Store
	marked   []int
	released []int
}

func (f *fakeStore) MarkEvergreenSent(campID, subID int) error {
	f.marked = append(f.marked, subID)
	return nil
}
func (f *fakeStore) ReleaseEvergreenClaim(campID, subID int) error {
	f.released = append(f.released, subID)
	return nil
}

func newTestManager(fs *fakeStore) *Manager {
	return &Manager{
		cfg:                Config{EvergreenEnabled: true, MaxSendErrors: 3},
		store:              fs,
		log:                log.New(os.Stderr, "", 0),
		evergreenIdleUntil: map[int]time.Time{},
		evergreenErrors:    map[int]int{},
		evergreenPrepared:  map[int]*models.Campaign{},
	}
}

func evergreenCamp(id int) *models.Campaign {
	c := &models.Campaign{Evergreen: true, Name: "w"}
	c.ID = id
	return c
}

// Fork (evergreen) -- the scan gate: flag off never pipes; an idle mark holds the
// campaign out of the scan for evergreenIdle and then releases it.
func TestShouldPipeEvergreen(t *testing.T) {
	c := evergreenCamp(7)
	m := newTestManager(&fakeStore{})

	m.cfg.EvergreenEnabled = false
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

	p := &pipe{camp: &models.Campaign{}, m: m}
	p.camp.ID = 8
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
	p.evergreenStopped()
	if _, ok := m.evergreenIdleUntil[8]; ok {
		t.Fatal("a stopped evergreen must be forgotten (review L1)")
	}
}

// Review C1: a message dropped because its pipe stopped gives its claim back; an
// attempted delivery (success or failure) marks the claim sent; a regular campaign's
// messages touch neither.
func TestEvergreenClaimBookkeeping(t *testing.T) {
	fs := &fakeStore{}
	m := newTestManager(fs)
	c := evergreenCamp(1)

	sub := func(id int) models.Subscriber {
		s := models.Subscriber{}
		s.ID = id
		return s
	}
	m.evergreenDropped(CampaignMessage{Campaign: c, Subscriber: sub(11)})
	m.evergreenAttempted(CampaignMessage{Campaign: c, Subscriber: sub(12)}, nil)
	m.evergreenAttempted(CampaignMessage{Campaign: c, Subscriber: sub(13)}, errors.New("smtp"))

	reg := &models.Campaign{}
	reg.ID = 2
	m.evergreenDropped(CampaignMessage{Campaign: reg, Subscriber: sub(21)})
	m.evergreenAttempted(CampaignMessage{Campaign: reg, Subscriber: sub(22)}, nil)

	if len(fs.released) != 1 || fs.released[0] != 11 {
		t.Fatalf("released = %v, want [11]", fs.released)
	}
	if len(fs.marked) != 2 || fs.marked[0] != 12 || fs.marked[1] != 13 {
		t.Fatalf("marked = %v, want [12 13]", fs.marked)
	}
}

// Review H1: the error streak lives on the manager, survives a re-pipe, trips
// MaxSendErrors, resets on a successful send and on stop.
func TestEvergreenErrorStreakAcrossPipes(t *testing.T) {
	fs := &fakeStore{}
	m := newTestManager(fs)
	c := evergreenCamp(5)

	newPipeFor := func() *pipe {
		p := &pipe{camp: c, m: m}
		return p
	}

	p1 := newPipeFor()
	p1.OnError()
	p1.OnError()
	if p1.stopped.Load() {
		t.Fatal("2 errors < MaxSendErrors 3 must not stop")
	}
	// Fresh pipe next tick: the streak must carry over and trip on the third error.
	p2 := newPipeFor()
	p2.OnError()
	if !p2.stopped.Load() || !p2.withErrors.Load() {
		t.Fatal("third error across pipes must trip MaxSendErrors")
	}
	p2.evergreenStopped()
	if m.evergreenErrors[c.ID] != 0 {
		t.Fatal("stop must reset the streak")
	}

	// A success in between resets the streak.
	p3 := newPipeFor()
	p3.OnError()
	p3.OnError()
	m.evergreenAttempted(CampaignMessage{Campaign: c, Subscriber: models.Subscriber{}}, nil)
	p3.OnError()
	if p3.stopped.Load() {
		t.Fatal("a success must reset the streak")
	}

	// Regular campaigns still count on the pipe.
	reg := &models.Campaign{}
	reg.ID = 6
	pr := &pipe{camp: reg, m: m}
	pr.OnError()
	pr.OnError()
	pr.OnError()
	if !pr.stopped.Load() {
		t.Fatal("regular pipe error counting broken")
	}
}

// Review M4: an unchanged evergreen re-pipes with the already-prepared instance; any
// content change yields the fresh row.
func TestReuseEvergreen(t *testing.T) {
	m := newTestManager(&fakeStore{})
	a := evergreenCamp(9)
	a.Body = "v1"
	got := m.reuseEvergreen(a)
	if got != a {
		t.Fatal("first sight must return the fresh row")
	}
	a.Prepared = true // newPipe did its work

	b := evergreenCamp(9)
	b.Body = "v1"
	if m.reuseEvergreen(b) != a {
		t.Fatal("unchanged content must reuse the prepared instance")
	}

	c := evergreenCamp(9)
	c.Body = "v2"
	if m.reuseEvergreen(c) != c {
		t.Fatal("changed content must not reuse")
	}
	m.forgetEvergreen(9)
	if _, ok := m.evergreenPrepared[9]; ok {
		t.Fatal("forget must drop the cache")
	}
}
