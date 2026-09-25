package review

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/knadh/listmonk/models"
)

// Verdicts.
const (
	VerdictPass    = "pass"
	VerdictBlocked = "blocked"
	VerdictNone    = "none"
)

// Pseudo-keys (integrations CAMPAIGN-INSPECT-SPEC D7): KeyAI accepts an AI-unavailable/partial
// report (the user override of parent §6); KeyR records the declined rendering-matrix offer
// (advisory, never blocks, logged like everything else).
const (
	KeyAI = "A"
	KeyR  = "R"
)

// Blocker is one item that keeps the gate closed.
type Blocker struct {
	Key    string `json:"key"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// VerdictResult is what the gate and the checklist window both read -- the window NEVER recomputes it.
type VerdictResult struct {
	Verdict  string    `json:"verdict"`
	Blockers []Blocker `json:"blockers"`
}

// reportView is the slice of the Lambda's report the rule reads. Everything else in the stored
// document is free to evolve.
type reportView struct {
	Status string `json:"status"`
	Items  []struct {
		ID         string `json:"id"`
		Tier       string `json:"tier"`
		Verdict    string `json:"verdict"`
		Acceptable *bool  `json:"acceptable"`
		Findings   []struct {
			Key      string `json:"key"`
			Severity string `json:"severity"`
		} `json:"findings"`
	} `json:"items"`
	AI struct {
		Status string `json:"status"`
	} `json:"ai"`
}

// LatestDispositions folds the append-only log to the newest action per item key.
func LatestDispositions(ds []models.ReviewDisposition) map[string]models.ReviewDisposition {
	sorted := append([]models.ReviewDisposition(nil), ds...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	out := map[string]models.ReviewDisposition{}
	for _, d := range sorted {
		if _, ok := out[d.ItemKey]; !ok {
			out[d.ItemKey] = d
		}
	}
	return out
}

// Verdict is the ONE implementation of the review verdict (D7), pure:
//
//   - none unless report.status == complete;
//   - a D item with verdict fail blocks unless the latest disposition for each of its finding keys
//     is accept -- and blocks regardless when the item is acceptable:false;
//   - an A finding with severity critical or high blocks unless accepted ("acknowledged", D9);
//   - ai.status other than ok (unavailable, partial) blocks unless the pseudo-key A is accepted;
//   - fixme never satisfies; warn, medium, low, n/a, R offers and context lines never block;
//   - keys are <id>#<fingerprint>, so a disposition on D2.3#fp1 never satisfies D2.3#fp2.
func Verdict(report json.RawMessage, dispositions []models.ReviewDisposition) VerdictResult {
	var r reportView
	if len(report) == 0 || json.Unmarshal(report, &r) != nil || r.Status != models.ReviewStatusComplete {
		return VerdictResult{Verdict: VerdictNone, Blockers: []Blocker{}}
	}

	latest := LatestDispositions(dispositions)
	accepted := func(key string) bool {
		d, ok := latest[key]
		return ok && d.Action == models.DispositionAccept
	}

	blockers := []Blocker{}
	for _, it := range r.Items {
		switch it.Tier {
		case "D":
			if it.Verdict != "fail" {
				continue
			}
			acceptable := it.Acceptable == nil || *it.Acceptable
			keys := []string{}
			for _, f := range it.Findings {
				keys = append(keys, f.Key)
			}
			if len(keys) == 0 {
				keys = append(keys, it.ID+"#")
			}
			for _, k := range keys {
				switch {
				case !acceptable:
					blockers = append(blockers, Blocker{Key: k, ID: it.ID, Reason: fmt.Sprintf("%s cannot be accepted", it.ID)})
				case !accepted(k):
					blockers = append(blockers, Blocker{Key: k, ID: it.ID, Reason: fmt.Sprintf("%s failed", it.ID)})
				}
			}
		case "A":
			for _, f := range it.Findings {
				if f.Severity != "critical" && f.Severity != "high" {
					continue
				}
				if !accepted(f.Key) {
					blockers = append(blockers, Blocker{Key: f.Key, ID: it.ID, Reason: fmt.Sprintf("%s %s finding", it.ID, f.Severity)})
				}
			}
		}
	}

	if r.AI.Status != "ok" && !accepted(KeyAI) {
		status := r.AI.Status
		if status == "" {
			status = "unavailable"
		}
		blockers = append(blockers, Blocker{Key: KeyAI, ID: KeyAI, Reason: "AI review " + status})
	}

	if len(blockers) > 0 {
		return VerdictResult{Verdict: VerdictBlocked, Blockers: blockers}
	}
	return VerdictResult{Verdict: VerdictPass, Blockers: blockers}
}

// ItemKeys returns every disposition key the report carries (finding keys, an empty-findings
// failed D item's `<id>#`) plus the two pseudo-keys -- what a disposition may name (I4).
func ItemKeys(report json.RawMessage) (map[string]struct {
	ID         string
	Acceptable bool
}, error) {
	var r reportView
	if err := json.Unmarshal(report, &r); err != nil {
		return nil, err
	}
	out := map[string]struct {
		ID         string
		Acceptable bool
	}{}
	for _, it := range r.Items {
		acceptable := it.Acceptable == nil || *it.Acceptable
		for _, f := range it.Findings {
			out[f.Key] = struct {
				ID         string
				Acceptable bool
			}{it.ID, acceptable}
		}
		if len(it.Findings) == 0 && it.Verdict == "fail" {
			out[it.ID+"#"] = struct {
				ID         string
				Acceptable bool
			}{it.ID, acceptable}
		}
	}
	return out, nil
}
