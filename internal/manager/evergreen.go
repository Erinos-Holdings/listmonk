package manager

// Fork (erinos evergreen campaigns).
//
// An evergreen campaign runs AS a pipe, so everything upstream's pipe already does —
// the per-second and sliding-window rate limits, the worker's stopped check on
// Cancel/Pause, OnError → MaxSendErrors auto-pause with notification, the send-rate
// stats — applies unchanged. Only three things differ, and they are the three hooks
// pipe.go calls into this file:
//
//   - the batch comes from next-evergreen-subscribers (join-time eligibility, with
//     the campaign_sends marker written in the same statement) instead of the
//     last_subscriber_id checkpoint query;
//   - an empty batch ends the pipe WITHOUT finishing the campaign (it stays
//     'running', idle) and without a notification; scanCampaigns re-pipes it after
//     an idle interval, which is the tick;
//   - the feature flag and the idle throttle decide whether scanCampaigns pipes it.

import (
	"fmt"
	"time"

	"github.com/knadh/listmonk/models"
)

// evergreenIdle is how long an evergreen campaign sits out of the scan after a batch
// came back empty. Welcome latency is bounded by this plus one ScanInterval.
const evergreenIdle = 30 * time.Second

// shouldPipeEvergreen reports whether scanCampaigns should create a pipe for this
// evergreen campaign now. Off-flag evergreens stay 'running' in the DB but idle.
func (m *Manager) shouldPipeEvergreen(c *models.Campaign) bool {
	if !m.cfg.EvergreenEnabled {
		return false
	}

	m.evergreenMut.Lock()
	defer m.evergreenMut.Unlock()
	if until, ok := m.evergreenIdleUntil[c.ID]; ok && time.Now().Before(until) {
		return false
	}
	return true
}

// markEvergreenIdle records that this campaign's last batch was empty.
func (m *Manager) markEvergreenIdle(id int) {
	m.evergreenMut.Lock()
	m.evergreenIdleUntil[id] = time.Now().Add(evergreenIdle)
	m.evergreenMut.Unlock()
}

// nextEvergreenSubscribers fetches the next eligible batch for an evergreen pipe.
// The store records the batch in campaign_sends atomically with the fetch.
func (p *pipe) nextEvergreenSubscribers() ([]models.Subscriber, error) {
	subs, err := p.m.store.NextEvergreenSubscribers(p.camp.ID, p.m.cfg.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("error fetching evergreen campaign subscribers (%s): %v", p.camp.Name, err)
	}
	return subs, nil
}

// evergreenCleanup is the tail of pipe.cleanup() for an evergreen campaign whose
// batch came back empty and which was not stopped. It returns true when it handled
// the pipe — the campaign is left 'running' and idle rather than 'finished'.
func (p *pipe) evergreenCleanup() bool {
	if !p.camp.Evergreen {
		return false
	}
	p.m.markEvergreenIdle(p.camp.ID)
	return true
}
