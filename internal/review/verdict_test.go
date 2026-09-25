package review

// Integrations CAMPAIGN-INSPECT-SPEC I3 -- the verdict rule (D7).

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

type tItem struct {
	ID         string     `json:"id"`
	Tier       string     `json:"tier"`
	Verdict    string     `json:"verdict"`
	Acceptable bool       `json:"acceptable"`
	Findings   []tFinding `json:"findings"`
}

type tFinding struct {
	Key      string `json:"key"`
	Severity string `json:"severity,omitempty"`
}

func rep(status, ai string, items ...tItem) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"v": 1, "status": status, "items": items, "ai": map[string]any{"status": ai}})
	return b
}

func d(id, verdict string, acceptable bool, keys ...string) tItem {
	it := tItem{ID: id, Tier: "D", Verdict: verdict, Acceptable: acceptable, Findings: []tFinding{}}
	for _, k := range keys {
		it.Findings = append(it.Findings, tFinding{Key: k})
	}
	return it
}

func a(id string, findings ...tFinding) tItem {
	v := "pass"
	if len(findings) > 0 {
		v = "finding"
	}
	return tItem{ID: id, Tier: "A", Verdict: v, Acceptable: true, Findings: findings}
}

var t0 = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func disp(key, action string, at int) models.ReviewDisposition {
	return models.ReviewDisposition{ID: int64(at), ItemKey: key, Action: action, CreatedAt: t0.Add(time.Duration(at) * time.Minute)}
}

func mustVerdict(t *testing.T, got VerdictResult, want string, blockers int) {
	t.Helper()
	if got.Verdict != want || len(got.Blockers) != blockers {
		t.Fatalf("verdict = %s with %d blockers %+v, want %s with %d", got.Verdict, len(got.Blockers), got.Blockers, want, blockers)
	}
}

func TestVerdictNoneUnlessComplete(t *testing.T) {
	for _, st := range []string{"running", "failed", "stale", ""} {
		mustVerdict(t, Verdict(rep(st, "ok"), nil), VerdictNone, 0)
	}
	mustVerdict(t, Verdict(nil, nil), VerdictNone, 0)
	mustVerdict(t, Verdict(json.RawMessage(`not json`), nil), VerdictNone, 0)
	mustVerdict(t, Verdict(rep("complete", "ok"), nil), VerdictPass, 0)
}

func TestVerdictDFail(t *testing.T) {
	r := rep("complete", "ok", d("D2.3", "fail", true, "D2.3#fp1"))
	mustVerdict(t, Verdict(r, nil), VerdictBlocked, 1)
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("D2.3#fp1", "accept", 1)}), VerdictPass, 0)
	// fixme never satisfies.
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("D2.3#fp1", "fixme", 1)}), VerdictBlocked, 1)
	// The latest disposition per key wins -- in both directions.
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("D2.3#fp1", "accept", 1), disp("D2.3#fp1", "fixme", 2)}), VerdictBlocked, 1)
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("D2.3#fp1", "fixme", 1), disp("D2.3#fp1", "accept", 2)}), VerdictPass, 0)
	// A disposition on one fingerprint never satisfies another of the same id.
	r2 := rep("complete", "ok", d("D2.3", "fail", true, "D2.3#fp2"))
	mustVerdict(t, Verdict(r2, []models.ReviewDisposition{disp("D2.3#fp1", "accept", 1)}), VerdictBlocked, 1)
	// Two findings: each must be accepted.
	r3 := rep("complete", "ok", d("D2.3", "fail", true, "D2.3#fp1", "D2.3#fp2"))
	mustVerdict(t, Verdict(r3, []models.ReviewDisposition{disp("D2.3#fp1", "accept", 1)}), VerdictBlocked, 1)
}

func TestVerdictNotAcceptable(t *testing.T) {
	r := rep("complete", "ok", d("D2.7", "fail", false, "D2.7#fp"))
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("D2.7#fp", "accept", 1)}), VerdictBlocked, 1)
}

func TestVerdictNonBlocking(t *testing.T) {
	r := rep("complete", "ok",
		d("D1.4b", "warn", true, "D1.4b#x"),
		d("D2.14", "n/a", true),
		d("D1.1", "pass", false),
		a("A2.1", tFinding{Key: "A2.1#m", Severity: "medium"}),
		a("A6.2", tFinding{Key: "A6.2#l", Severity: "low"}),
	)
	mustVerdict(t, Verdict(r, nil), VerdictPass, 0)
	// R is advisory: a declined offer never blocks, an absent one neither.
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp(KeyR, "accept", 1)}), VerdictPass, 0)
}

func TestVerdictACriticalHigh(t *testing.T) {
	r := rep("complete", "ok", a("A1.1", tFinding{Key: "A1.1#c", Severity: "critical"}), a("A2.3", tFinding{Key: "A2.3#h", Severity: "high"}))
	mustVerdict(t, Verdict(r, nil), VerdictBlocked, 2)
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("A1.1#c", "accept", 1)}), VerdictBlocked, 1)
	mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp("A1.1#c", "accept", 1), disp("A2.3#h", "accept", 2)}), VerdictPass, 0)
}

func TestVerdictAIUnavailable(t *testing.T) {
	for _, st := range []string{"unavailable", "partial", ""} {
		r := rep("complete", st)
		mustVerdict(t, Verdict(r, nil), VerdictBlocked, 1)
		mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp(KeyAI, "accept", 1)}), VerdictPass, 0)
		mustVerdict(t, Verdict(r, []models.ReviewDisposition{disp(KeyAI, "fixme", 1)}), VerdictBlocked, 1)
	}
}

func TestItemKeys(t *testing.T) {
	r := rep("complete", "ok", d("D2.6", "fail", false, "D2.6#a"), d("D1.1", "fail", false), a("A1.1", tFinding{Key: "A1.1#c", Severity: "critical"}))
	keys, err := ItemKeys(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || keys["D2.6#a"].Acceptable || keys["D1.1#"].ID != "D1.1" || !keys["A1.1#c"].Acceptable {
		t.Fatalf("keys = %+v", keys)
	}
}
