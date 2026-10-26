package migrations

// Fork (on-demand brand-health refresh) -- integrations BRANDHEALTH-REFRESH-SPEC I4: v6.2.27
// inserts app.health_refresh_url and app.health_refresh_hmac_secret with the default '""', only
// when absent (a stored value survives a re-run), is idempotent, and schema.sql seeds the same two
// rows. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var healthRefreshKeys = []string{"app.health_refresh_url", "app.health_refresh_hmac_secret"}

func TestHealthRefreshSettings_V6_2_27(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// The fresh schema already carries both rows with the default (schema.sql parity).
	for _, k := range healthRefreshKeys {
		var v string
		if err := h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = $1`, k); err != nil || v != `""` {
			t.Fatalf("fresh schema: %s = %q (%v), want \"\"", k, v, err)
		}
	}

	// A v6.2.26 database: neither key.
	h.db.MustExec(`DELETE FROM settings WHERE key = ANY($1)`, "{"+strings.Join(healthRefreshKeys, ",")+"}")

	for i := 0; i < 2; i++ {
		if err := V6_2_27(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_27 run %d: %v", i+1, err)
		}
	}
	for _, k := range healthRefreshKeys {
		var v string
		if err := h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = $1`, k); err != nil || v != `""` {
			t.Fatalf("%s = %q (%v), want \"\"", k, v, err)
		}
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM settings WHERE key = $1`, k)
		if n != 1 {
			t.Fatalf("%s rows = %d after two runs, want 1", k, n)
		}
	}

	// A stored (pasted) value is left untouched by a re-run.
	h.db.MustExec(`UPDATE settings SET value = '"https://x.lambda-url.us-west-2.on.aws/"' WHERE key = 'app.health_refresh_url'`)
	h.db.MustExec(`UPDATE settings SET value = '"s3cret"' WHERE key = 'app.health_refresh_hmac_secret'`)
	if err := V6_2_27(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_27 over stored values: %v", err)
	}
	var url, secret string
	h.db.Get(&url, `SELECT value::TEXT FROM settings WHERE key = 'app.health_refresh_url'`)
	h.db.Get(&secret, `SELECT value::TEXT FROM settings WHERE key = 'app.health_refresh_hmac_secret'`)
	if url != `"https://x.lambda-url.us-west-2.on.aws/"` || secret != `"s3cret"` {
		t.Fatalf("re-run overwrote stored values: url %s secret %s", url, secret)
	}

	// schema.sql carries the same two rows, beside the review keys.
	b, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range healthRefreshKeys {
		if row := "('" + k + "', '\"\"')"; !strings.Contains(string(b), row) {
			t.Fatalf("schema.sql lacks %s", row)
		}
	}
}
