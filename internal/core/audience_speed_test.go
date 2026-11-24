package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

// Integrations LIST-AUDIENCE-SPEED-SPEC I1 + I4 -- the no_audience path (prepareAudience with
// noCount) fills every non-audience FillAudience field exactly as the default path does, marks
// audience_pending exactly where wantsAudience is true, and the default path never marks it, runs
// the count wherever wantsAudience is true, and serves no audience_pending key.
func TestPrepareAudienceNoCount(t *testing.T) {
	statuses := []string{
		models.CampaignStatusDraft, models.CampaignStatusScheduled, models.CampaignStatusRunning,
		models.CampaignStatusPaused, models.CampaignStatusFinished, models.CampaignStatusCancelled,
	}
	mk := func(status string, evergreen bool) models.Campaign {
		cm := models.Campaign{Evergreen: evergreen}
		cm.ID = 7
		cm.Status = status
		cm.Attribs = models.JSON{
			models.SmartSendingAttrib: false,
			models.SilentSkipAttrib:   true,
			models.PromoCodeAttrib:    "BFCM",
			models.PromoStepAttrib:    float64(2),
			models.PromoTagsAttrib:    []any{"sku-a"},
		}
		return cm
	}
	for _, status := range statuses {
		for _, evergreen := range []bool{false, true} {
			want := wantsAudience(mk(status, evergreen))

			off := mk(status, evergreen)
			runOff := prepareAudience(&off, false)
			on := mk(status, evergreen)
			runOn := prepareAudience(&on, true)

			if runOff != want {
				t.Errorf("%s/%v default: count runs %v, want %v", status, evergreen, runOff, want)
			}
			if runOn {
				t.Errorf("%s/%v no_audience: the count ran", status, evergreen)
			}
			if off.AudiencePending {
				t.Errorf("%s/%v default: audience_pending set", status, evergreen)
			}
			if on.AudiencePending != want {
				t.Errorf("%s/%v no_audience: audience_pending %v, want %v", status, evergreen, on.AudiencePending, want)
			}

			// Every non-audience served field is set, and equal on both paths.
			if on.SmartSendingOn || !on.SilentSkipOn || on.PromoCodeOut != "BFCM" ||
				on.PromoStepOut.Int != 2 || !on.PromoStepOut.Valid || !reflect.DeepEqual(on.PromoTagsOut, []string{"sku-a"}) {
				t.Errorf("%s/%v no_audience: served fields not filled: %+v", status, evergreen, on)
			}
			on.AudiencePending = off.AudiencePending
			if !reflect.DeepEqual(on, off) {
				t.Errorf("%s/%v: the two paths differ beyond audience_pending", status, evergreen)
			}

			// I4 -- the default response carries no audience_pending key.
			b, _ := json.Marshal(off)
			if strings.Contains(string(b), "audience_pending") {
				t.Errorf("%s/%v default JSON carries audience_pending", status, evergreen)
			}
			if want {
				on.AudiencePending = true
				b, _ = json.Marshal(on)
				if !strings.Contains(string(b), `"audience_pending":true`) {
					t.Errorf("%s/%v no_audience JSON lacks audience_pending", status, evergreen)
				}
			}
		}
	}
}

// LIST-AUDIENCE-SPEED-SPEC I2 -- runBounded calls fn once per index, never more than the bound at
// once, and returns only after every call has finished.
func TestRunBounded(t *testing.T) {
	for _, n := range []int{0, 1, 3, 4, 5, 13} {
		var (
			mu       sync.Mutex
			calls    = make([]int, n)
			cur, max int32
			done     int32
		)
		runBounded(n, audienceFillConcurrency, func(i int) {
			c := atomic.AddInt32(&cur, 1)
			mu.Lock()
			calls[i]++
			if c > max {
				max = c
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&cur, -1)
			atomic.AddInt32(&done, 1)
		})
		if int(atomic.LoadInt32(&done)) != n {
			t.Errorf("n=%d: returned with %d of %d calls finished", n, done, n)
		}
		for i, k := range calls {
			if k != 1 {
				t.Errorf("n=%d: index %d called %d times", n, i, k)
			}
		}
		if max > int32(audienceFillConcurrency) {
			t.Errorf("n=%d: %d at once, bound %d", n, max, audienceFillConcurrency)
		}
		if n >= audienceFillConcurrency && max < 2 {
			t.Errorf("n=%d: never ran concurrently (max %d)", n, max)
		}
	}
	if audienceFillConcurrency != 4 {
		t.Errorf("bound %d, spec D1 says 4", audienceFillConcurrency)
	}
}
