package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D5) -- an analytics bucket is
// written as its wall-clock digits with NO offset, whatever location the driver handed back, so the
// page shows the Mountain bucket verbatim in every browser.
func TestCampaignAnalyticsCountJSONHasNoOffset(t *testing.T) {
	for _, loc := range []*time.Location{time.UTC, time.FixedZone("", 0), time.FixedZone("x", -6*3600)} {
		c := CampaignAnalyticsCount{CampaignID: 217, Count: 8107, Timestamp: time.Date(2026, 10, 21, 18, 0, 0, 0, loc)}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != `{"campaign_id":217,"count":8107,"timestamp":"2026-10-21T18:00:00"}` {
			t.Errorf("%v: %s", loc, b)
		}
		// Inside a slice too (the API's shape), and it round-trips as a plain JSON object.
		b, _ = json.Marshal([]CampaignAnalyticsCount{c})
		var back []map[string]any
		if err := json.Unmarshal(b, &back); err != nil || len(back) != 1 || strings.ContainsAny(back[0]["timestamp"].(string), "Z+") {
			t.Errorf("%v: slice form %s (%v)", loc, b, err)
		}
		// The model round-trips: the same wall-clock digits come back.
		var rt []CampaignAnalyticsCount
		if err := json.Unmarshal(b, &rt); err != nil || len(rt) != 1 || rt[0].Count != 8107 || rt[0].CampaignID != 217 ||
			rt[0].Timestamp.Format(AnalyticsBucketLayout) != "2026-10-21T18:00:00" {
			t.Errorf("%v: round trip %v (%v)", loc, rt, err)
		}
	}
}
