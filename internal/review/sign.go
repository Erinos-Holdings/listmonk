package review

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Sign is the job signature the fork sends the review Lambda (integrations CAMPAIGN-INSPECT-SPEC
// D2): `sha256=<hex HMAC-SHA256(secret, ts + "." + body)>`, in the X-Review-Signature header next
// to X-Review-Timestamp: ts (unix seconds). The Lambda verifies it in constant time and refuses a
// skew over 300 s. Pinned by sign_test.go and the Lambda's handler test on the same vector.
func Sign(secret string, ts int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(ts, 10)))
	m.Write([]byte("."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}
