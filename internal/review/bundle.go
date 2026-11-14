// Package review is the fork's pure half of the campaign review (integrations
// CAMPAIGN-INSPECT-SPEC D4/D7/D2): the bundle hash the gate keys reviews on, the verdict rule the
// gate and the checklist window both read, and the HMAC signer for the job the fork POSTs to the
// review Lambda. No I/O, no database, no HTTP -- go test only.
package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/knadh/listmonk/models"
)

// BundleHash is the review's "state" (parent CAMPAIGN-REVIEW-SPEC §8, INSPECT-SPEC D4): sha256
// over the canonical JSON of everything the review reads that a save can change. The gate
// compares it with the hash a review was recorded against; any save that changes it invalidates
// the inspection.
//
// THE TS TWIN is integrations lib/campaign-review/bundle.ts (campaignHash). Both are pinned to one
// fixture (testdata/bundle-hash.json, copied from integrations tests/fixtures/campaign-review/)
// with the same expected hex in both tests -- change them together.
//
// Canonical form (byte for byte in both languages):
//   - exactly the keys below, sorted: NOT send_at (a reschedule is not a content change), status,
//     updated_at;
//   - object keys sorted by UTF-8 bytes, recursively (attribs, archive_meta, each header);
//   - strings: `"` and `\` escaped, U+0000-U+001F as \u00xx (lower-case hex), everything else raw
//     UTF-8 (no HTML escaping, no U+2028/2029 escaping);
//   - numbers as ECMAScript Number#toString (encoding/json's float64 format matches it);
//   - null/"" -> "" for altbody, body_source, archive_slug; null/{} -> {} for attribs and
//     archive_meta; null headers -> [] (the Vue save round-trip must reproduce the hash of an
//     unedited campaign);
//   - attribs without its smart_sending key (fork, SMART-SENDING-SPEC D4 -- the TS twin must drop it
//     too, or a switched-off campaign's Inspect reads stale);
//   - lists = the target list ids, sorted;
//   - NOT the campaign name (fork, integrations NAMING-STANDARD-SPEC D12 -- a rename must not void a
//     passed Inspect; the review no longer reads the name). LegacyBundleHash is the pre-release
//     formula, which still hashed "name", kept so a review written before the release stays
//     current (core.CurrentReview).
func BundleHash(camp models.Campaign, listIDs []int) string {
	return bundleHash(camp, listIDs, false, "")
}

// LegacyBundleHash is the pre-NAMING-STANDARD formula: BundleHash plus "name" = name (the name the
// campaign carried when the review was written -- core.CurrentReview passes the campaign's
// former_names row when one exists, else its current name). The shared fixture pins it as
// legacyExpected in both repos. Permanent (NAMING-STANDARD-SPEC §4.7).
func LegacyBundleHash(camp models.Campaign, listIDs []int, name string) string {
	return bundleHash(camp, listIDs, true, name)
}

func bundleHash(camp models.Campaign, listIDs []int, withName bool, name string) string {
	ids := append([]int(nil), listIDs...)
	sort.Ints(ids)
	idsAny := make([]any, len(ids))
	for i, id := range ids {
		idsAny[i] = id
	}

	// Fork (smart sending, integrations SMART-SENDING-SPEC D4) -- attribs.smart_sending is dropped
	// before hashing: it changes who receives, not what was inspected, and reviewGuardOnEdit refuses
	// a scheduled campaign's save whose hash changes -- so without this the switch could not be
	// flipped on the scheduled launch it exists for. A copy: the campaign's map is never mutated.
	attribs := map[string]any{}
	for k, v := range camp.Attribs {
		if k == models.SmartSendingAttrib {
			continue
		}
		attribs[k] = v
	}

	var archiveMeta any = map[string]any{}
	if len(bytes.TrimSpace(camp.ArchiveMeta)) > 0 {
		var v any
		if err := json.Unmarshal(camp.ArchiveMeta, &v); err == nil && v != nil {
			archiveMeta = v
		}
	}

	headers := []any{}
	for _, h := range camp.Headers {
		m := map[string]any{}
		for k, v := range h {
			m[k] = v
		}
		headers = append(headers, m)
	}

	obj := map[string]any{
		"altbody":             camp.AltBody.String,
		"archive":             camp.Archive,
		"archive_meta":        archiveMeta,
		"archive_slug":        camp.ArchiveSlug.String,
		"archive_template_id": nullInt(camp.ArchiveTemplateID.Valid, camp.ArchiveTemplateID.Int),
		"attribs":             attribs,
		"body":                camp.Body,
		"body_source":         camp.BodySource.String,
		"content_type":        camp.ContentType,
		"evergreen":           camp.Evergreen,
		"headers":             headers,
		"lists":               idsAny,
		"send_delay_secs":     camp.SendDelaySecs,
		"subject":             camp.Subject,
		"template_id":         nullInt(camp.TemplateID.Valid, camp.TemplateID.Int),
	}

	if withName {
		obj["name"] = name
	}

	var buf bytes.Buffer
	canon(&buf, obj)
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

func nullInt(valid bool, v int) any {
	if !valid {
		return nil
	}
	return v
}

// CanonicalJSON is the canonical encoding (exported for tests and diagnostics).
func CanonicalJSON(v any) []byte {
	var buf bytes.Buffer
	canon(&buf, v)
	return buf.Bytes()
}

func canon(w *bytes.Buffer, v any) {
	switch t := v.(type) {
	case nil:
		w.WriteString("null")
	case bool:
		if t {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case string:
		canonString(w, t)
	case int:
		w.WriteString(strconv.Itoa(t))
	case int64:
		w.WriteString(strconv.FormatInt(t, 10))
	case float64:
		b, err := json.Marshal(t)
		if err != nil {
			// NaN/Inf cannot come out of a JSON document.
			w.WriteString("null")
			return
		}
		w.Write(b)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			w.WriteString(t.String())
			return
		}
		canon(w, f)
	case []any:
		w.WriteByte('[')
		for i, x := range t {
			if i > 0 {
				w.WriteByte(',')
			}
			canon(w, x)
		}
		w.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // Go strings compare bytewise = UTF-8 byte order.
		w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.WriteByte(',')
			}
			canonString(w, k)
			w.WriteByte(':')
			canon(w, t[k])
		}
		w.WriteByte('}')
	case models.JSON:
		canon(w, map[string]any(t))
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, s := range t {
			m[k] = s
		}
		canon(w, m)
	default:
		// Anything else: round-trip through encoding/json into the generic shapes above.
		b, err := json.Marshal(t)
		if err != nil {
			w.WriteString("null")
			return
		}
		var g any
		if err := json.Unmarshal(b, &g); err != nil {
			w.WriteString("null")
			return
		}
		canon(w, g)
	}
}

func canonString(w *bytes.Buffer, s string) {
	w.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '"':
			w.WriteString(`\"`)
		case r == '\\':
			w.WriteString(`\\`)
		case r < 0x20:
			fmt.Fprintf(w, `\u%04x`, r)
		default:
			w.WriteString(s[i : i+size])
		}
		i += size
	}
	w.WriteByte('"')
}
