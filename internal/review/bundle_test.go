package review

// Integrations CAMPAIGN-INSPECT-SPEC I2 -- the Go and TS bundle hashes are equal for the same
// campaign. testdata/bundle-hash.json is a COPY of integrations
// tests/fixtures/campaign-review/bundle-hash.json; its TS test pins the same hex as pinnedHash.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/listmonk/models"
)

const pinnedHash = "824337bdbfef82eb131f4150df40ec621465ae141f43648fbec7d12f764befbb"

type hashFixture struct {
	Campaign map[string]json.RawMessage `json:"campaign"`
	Expected string                     `json:"expected"`
	Pairs    []struct {
		Name string                     `json:"name"`
		A    map[string]json.RawMessage `json:"a"`
		B    map[string]json.RawMessage `json:"b"`
		Same bool                       `json:"same"`
	} `json:"pairs"`
}

func loadHashFixture(t *testing.T) hashFixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "bundle-hash.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx hashFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// hashOf overlays patch onto the fixture campaign and hashes it the way the gate does: the
// campaign row from JSON (the API shape) and the list ids from its `lists`.
func hashOf(t *testing.T, base, patch map[string]json.RawMessage) string {
	t.Helper()
	m := map[string]json.RawMessage{}
	for k, v := range base {
		m[k] = v
	}
	for k, v := range patch {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	var camp models.Campaign
	if err := json.Unmarshal(b, &camp); err != nil {
		t.Fatalf("unmarshal campaign: %v", err)
	}
	var lists []struct {
		ID int `json:"id"`
	}
	json.Unmarshal(m["lists"], &lists)
	ids := make([]int, 0, len(lists))
	for _, l := range lists {
		ids = append(ids, l.ID)
	}
	return BundleHash(camp, ids)
}

func TestBundleHashPinned(t *testing.T) {
	fx := loadHashFixture(t)
	if fx.Expected != pinnedHash {
		t.Fatalf("fixture expected %s, pinned %s -- the fixture was regenerated; re-pin both repos", fx.Expected, pinnedHash)
	}
	if got := hashOf(t, fx.Campaign, nil); got != pinnedHash {
		t.Fatalf("BundleHash = %s, want %s (the TS twin's hash)", got, pinnedHash)
	}
}

func TestBundleHashPairs(t *testing.T) {
	fx := loadHashFixture(t)
	for _, p := range fx.Pairs {
		t.Run(p.Name, func(t *testing.T) {
			a := hashOf(t, fx.Campaign, p.A)
			b := hashOf(t, fx.Campaign, p.B)
			if p.Same && a != b {
				t.Fatalf("expected equal hashes, got %s vs %s", a, b)
			}
			if !p.Same && a == b {
				t.Fatalf("expected a different hash, both %s", a)
			}
		})
	}
}

func TestCanonicalJSON(t *testing.T) {
	got := string(CanonicalJSON(map[string]any{"b": 1, "a": "<&>", "é": 2, "Z": []any{true, nil}}))
	if want := `{"Z":[true,null],"a":"<&>","b":1,"é":2}`; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	got = string(CanonicalJSON("a\u0001\n\"\\ "))
	if want := "\"a\\u0001\\u000a\\\"\\\\ \""; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if got := string(CanonicalJSON(1e21)); got != "1e+21" {
		t.Fatalf("1e21 -> %s", got)
	}
}
