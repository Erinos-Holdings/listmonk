package manager

// Fork (erinos evergreen campaigns).
//
// An evergreen campaign runs AS a pipe, so everything upstream's pipe already does —
// the per-second and sliding-window rate limits, the worker's stopped check on
// Cancel/Pause, OnError → MaxSendErrors auto-pause with notification, the send-rate
// stats — applies unchanged. What differs lives here and is reached through small
// hooks in pipe.go / manager.go:
//
//   - the batch comes from next-evergreen-subscribers (join-time eligibility, with
//     the campaign_sends CLAIM written in the same statement) instead of the
//     last_subscriber_id checkpoint query;
//   - the worker marks the claim SENT on the delivery attempt and RELEASES it when it
//     drops the message unattempted (pipe stopped by pause/cancel) — otherwise every
//     queued-but-unsent welcome at a pause would be lost forever;
//   - an empty batch ends the pipe WITHOUT finishing the campaign (it stays
//     'running', idle) and without a notification; scanCampaigns re-pipes it after
//     an idle interval, which is the tick;
//   - because the pipe is recreated every tick, the send-error streak is counted on
//     the manager (per campaign) so MaxSendErrors trips on a trickle of signups during
//     an SMTP outage, and the prepared (images resolved, template compiled, media
//     attached) campaign is cached by content hash so a re-pipe is cheap;
//   - the feature flag and the idle throttle decide whether scanCampaigns pipes it.

import (
	"fmt"
	"hash/fnv"
	"strconv"
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

// forgetEvergreen drops the campaign's manager-side state once its pipe stopped
// (pause, cancel, auto-pause). The error streak resets so a resume starts clean.
func (m *Manager) forgetEvergreen(id int) {
	m.evergreenMut.Lock()
	delete(m.evergreenIdleUntil, id)
	delete(m.evergreenErrors, id)
	delete(m.evergreenPrepared, id)
	m.evergreenMut.Unlock()
}

// evergreenErrorInc bumps the campaign's consecutive-error streak and returns it.
func (m *Manager) evergreenErrorInc(id int) uint64 {
	m.evergreenMut.Lock()
	defer m.evergreenMut.Unlock()
	m.evergreenErrors[id]++
	return uint64(m.evergreenErrors[id])
}

// evergreenErrorReset clears the streak after a successful send.
func (m *Manager) evergreenErrorReset(id int) {
	m.evergreenMut.Lock()
	delete(m.evergreenErrors, id)
	m.evergreenMut.Unlock()
}

// evergreenContentHash covers everything newPipe's prepare step depends on.
func evergreenContentHash(c *models.Campaign) string {
	h := fnv.New64a()
	for _, s := range []string{c.Subject, c.Body, c.BodySource.String, c.AltBody.String, c.TemplateBody,
		c.FrozenTemplateBody.String, c.ContentType, c.Messenger, c.FromEmail, strconv.Itoa(int(c.TemplateID.Int))} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	for _, id := range c.MediaIDs {
		h.Write([]byte(strconv.FormatInt(id, 10)))
		h.Write([]byte{0})
	}
	for _, hd := range c.Headers {
		for k, v := range hd {
			h.Write([]byte(k + "=" + v))
			h.Write([]byte{0})
		}
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// reuseEvergreen returns the already-prepared instance of this campaign when its
// content is unchanged since the last pipe, else the fresh row (to be prepared and
// cached by the caller's newPipe via Campaign.Prepared).
func (m *Manager) reuseEvergreen(c *models.Campaign) *models.Campaign {
	key := evergreenContentHash(c)

	m.evergreenMut.Lock()
	defer m.evergreenMut.Unlock()
	if prev, ok := m.evergreenPrepared[c.ID]; ok && prev.Prepared && evergreenContentHash(prev) == key {
		return prev
	}
	m.evergreenPrepared[c.ID] = c
	return c
}

// nextEvergreenSubscribers fetches (and claims) the next eligible batch.
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

// evergreenStopped runs from cleanup() when the pipe was stopped (manual or errors).
func (p *pipe) evergreenStopped() {
	if p.camp.Evergreen {
		p.m.forgetEvergreen(p.camp.ID)
	}
}

// evergreenDropped runs from the worker when a queued message is discarded because
// its pipe stopped: the claim is given back so the subscriber is welcomed on resume.
func (m *Manager) evergreenDropped(msg CampaignMessage) {
	if msg.Campaign == nil || !msg.Campaign.Evergreen {
		return
	}
	if err := m.store.ReleaseEvergreenClaim(msg.Campaign.ID, msg.Subscriber.ID); err != nil {
		m.log.Printf("error releasing evergreen claim (%s, subscriber %d): %v", msg.Campaign.Name, msg.Subscriber.ID, err)
	}
}

// evergreenAttempted runs from the worker after a delivery attempt, success or not.
func (m *Manager) evergreenAttempted(msg CampaignMessage, sendErr error) {
	if msg.Campaign == nil || !msg.Campaign.Evergreen {
		return
	}
	if err := m.store.MarkEvergreenSent(msg.Campaign.ID, msg.Subscriber.ID); err != nil {
		m.log.Printf("error marking evergreen send (%s, subscriber %d): %v", msg.Campaign.Name, msg.Subscriber.ID, err)
	}
	if sendErr == nil {
		m.evergreenErrorReset(msg.Campaign.ID)
	}
}
