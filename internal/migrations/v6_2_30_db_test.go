package migrations

// Fork (usage decisions, 2026-10-31 polish) -- v6.2.30 widens usage_decisions' decision CHECK to
// accept/reject/current/recommended: from a v6.2.29 database (the two-value CHECK) it reaches
// exactly schema.sql's shape, is idempotent, keeps stored rows, and still refuses an unknown
// decision. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"strings"
	"testing"
)

func TestUsageDecisionsCheck_V6_2_30(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.decisionsShape()

	// A v6.2.29 database: the two-value CHECK, one stored row.
	h.db.MustExec(`ALTER TABLE usage_decisions DROP CONSTRAINT usage_decisions_decision_check`)
	h.db.MustExec(`ALTER TABLE usage_decisions ADD CONSTRAINT usage_decisions_decision_check CHECK (decision IN ('accept', 'reject'))`)
	ins := func(decision string) error {
		_, err := h.db.Exec(`INSERT INTO usage_decisions (item_key, text_hash, decision, note, user_id, user_name)
			VALUES ('both-per-brand', $1, $2, NULL, 4, 'Kaylinn')`, strings.Repeat("a", 64), decision)
		return err
	}
	if err := ins("accept"); err != nil {
		t.Fatal(err)
	}
	if err := ins("recommended"); err == nil {
		t.Fatal("the v6.2.29 CHECK accepted 'recommended'")
	}

	for i := 0; i < 2; i++ {
		if err := V6_2_30(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_30 run %d: %v", i+1, err)
		}
	}
	if got := h.decisionsShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	for _, d := range []string{"current", "recommended", "reject"} {
		if err := ins(d); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
	}
	if err := ins("defer"); err == nil {
		t.Fatal("an unknown decision was accepted")
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM usage_decisions`)
	if n != 4 {
		t.Fatalf("rows = %d, want 4 (the stored accept kept)", n)
	}
}
