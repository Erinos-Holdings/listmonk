package review

// Integrations CAMPAIGN-INSPECT-SPEC I5 -- the Go signer produces what the TS verifier accepts. The
// same vector is pinned in integrations tests/lambda/campaign-review/handler.test.ts.

import "testing"

const (
	signVectorSecret = "review-test-secret"
	signVectorTS     = int64(1790000000)
	signVectorBody   = `{"jobId":"00000000-0000-4000-8000-000000000001","campaignId":108,"bundleHash":"824337bdbfef82eb131f4150df40ec621465ae141f43648fbec7d12f764befbb","requestedBy":"robbie"}`
	signVectorWant   = "sha256=563aa4dc190c7e72fe370b4616dbf4a7fd2fff70cc02b2304dd9571b09883363"
)

func TestSignPinnedVector(t *testing.T) {
	if got := Sign(signVectorSecret, signVectorTS, []byte(signVectorBody)); got != signVectorWant {
		t.Fatalf("Sign = %s, want %s (the TS verifier's vector)", got, signVectorWant)
	}
}

func TestSignDependsOnEveryInput(t *testing.T) {
	base := Sign(signVectorSecret, signVectorTS, []byte(signVectorBody))
	for name, got := range map[string]string{
		"secret": Sign("other", signVectorTS, []byte(signVectorBody)),
		"ts":     Sign(signVectorSecret, signVectorTS+1, []byte(signVectorBody)),
		"body":   Sign(signVectorSecret, signVectorTS, []byte(signVectorBody+" ")),
	} {
		if got == base {
			t.Fatalf("changing the %s did not change the signature", name)
		}
	}
}
