package main

// Fork (global language filter) -- integrations GLOBAL-LANG-SPEC I5 against a real database: the
// by-query bulk endpoints (blocklist, delete, list add / remove / unsubscribe) and the export apply
// a `lang` carried WITHOUT a segment (the global toggle's value reaches the Subscribers page as the
// route's lang; the bodies below still carry list_ids, as the page sends them, and one read-only
// export case carries no list at all -- the All-brands form) on the server -- lang=es touches only
// the send-language-ES subscribers,
// and lang=en is the send language (en plus no-language). Each verb is asserted. The expected ids
// come from internal/testfixtures/listgrid (an independent Go statement of the language rule), so
// no assertion compares the SQL with itself. Shares the list-grid fixture (LISTMONK_TEST_PG opt-in;
// a plain go test skips).

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func TestByQueryLangFilter(t *testing.T) {
	everyone := func(f *gridFixture) []int {
		out := make([]int, 0, len(f.rows))
		for _, r := range f.rows {
			out = append(out, r.ID)
		}
		sort.Ints(out)
		return out
	}
	union := func(a, b []int) []int {
		seen := map[int]bool{}
		var out []int
		for _, x := range append(append([]int{}, a...), b...) {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
		sort.Ints(out)
		return out
	}
	ids := func(f *gridFixture, q string, args ...any) []int {
		var out []int
		if err := f.db.Select(&out, q, args...); err != nil {
			f.t.Fatal(err)
		}
		sort.Ints(out)
		return out
	}
	blocklisted := func(f *gridFixture) []int {
		return ids(f, `SELECT id FROM subscribers WHERE email LIKE '%@grid.test' AND status = 'blocklisted'`)
	}
	onList := func(f *gridFixture, list int) []int {
		return ids(f, `SELECT subscriber_id FROM subscriber_lists WHERE list_id = $1`, list)
	}
	unsubscribedOn := func(f *gridFixture, list int) []int {
		return ids(f, `SELECT subscriber_id FROM subscriber_lists WHERE list_id = $1 AND status = 'unsubscribed'`, list)
	}

	t.Run("blocklist lang=es", func(t *testing.T) {
		f := newGridFixture(t)
		before, es := blocklisted(f), f.want(false, "", "es")
		// lang alone satisfies the "Blocklist by query needs a filter" guard -- the confirmation
		// naming the language is what makes it deliberate (D4).
		body := fmt.Sprintf(`{"list_ids": [%d], "lang": "es"}`, f.single)
		if code, msg := f.call(f.app.BlocklistSubscribersByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("blocklist: %d %s", code, msg)
		}
		if got, want := blocklisted(f), union(before, es); len(es) == 0 || !sameInts(got, want) {
			t.Fatalf("blocklist lang=es: %d blocklisted, want %d (+%d es)", len(got), len(want), len(es))
		}
	})

	t.Run("delete all=true + lang=es", func(t *testing.T) {
		f := newGridFixture(t)
		es := f.want(false, "", "es")
		body := fmt.Sprintf(`{"all": true, "list_ids": [%d], "lang": "es"}`, f.single)
		if code, msg := f.call(f.app.DeleteSubscribersByQuery, http.MethodPost, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("delete: %d %s", code, msg)
		}
		if got := f.survivors(); len(es) == 0 || !sameInts(got, minus(everyone(f), es)) {
			t.Fatalf("delete lang=es: %d survivors, want %d", len(got), len(f.rows)-len(es))
		}
	})

	t.Run("manage add lang=es", func(t *testing.T) {
		f := newGridFixture(t)
		var target int
		f.db.Get(&target, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'lang target', 'private', 'single') RETURNING id`)
		es := f.want(false, "", "es")
		body := fmt.Sprintf(`{"action": "add", "list_ids": [%d], "target_list_ids": [%d], "lang": "es", "status": "confirmed", "backfill": true}`, f.single, target)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("add: %d %s", code, msg)
		}
		if got := onList(f, target); len(es) == 0 || !sameInts(got, es) {
			t.Fatalf("add lang=es: %d on the target, want %d", len(got), len(es))
		}
	})

	t.Run("manage remove lang=es", func(t *testing.T) {
		f := newGridFixture(t)
		es := f.want(false, "", "es")
		before := onList(f, f.single)
		body := fmt.Sprintf(`{"action": "remove", "list_ids": [%d], "target_list_ids": [%d], "lang": "es"}`, f.single, f.single)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("remove: %d %s", code, msg)
		}
		if got := onList(f, f.single); len(es) == 0 || !sameInts(got, minus(before, es)) {
			t.Fatalf("remove lang=es: %d left, want %d", len(got), len(before)-len(es))
		}
	})

	t.Run("manage unsubscribe lang=es", func(t *testing.T) {
		f := newGridFixture(t)
		es := f.want(true, "", "es")
		before := unsubscribedOn(f, f.double)
		body := fmt.Sprintf(`{"action": "unsubscribe", "list_ids": [%d], "target_list_ids": [%d], "lang": "es"}`, f.double, f.double)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("unsubscribe: %d %s", code, msg)
		}
		if got, want := unsubscribedOn(f, f.double), union(before, es); len(es) == 0 || !sameInts(got, want) {
			t.Fatalf("unsubscribe lang=es: %d unsubscribed, want %d", len(got), len(want))
		}
	})

	t.Run("export lang=es and lang=en", func(t *testing.T) {
		f := newGridFixture(t)
		for _, lang := range []string{"es", "en"} {
			want := f.want(false, "", lang)
			code, body := f.call(f.app.ExportSubscribers, http.MethodGet,
				fmt.Sprintf("/api/subscribers/export?list_id=%d&lang=%s", f.single, lang), "", plainUser)
			if code != http.StatusOK {
				t.Fatalf("export lang=%s: %d %s", lang, code, body)
			}
			if got := strings.Count(strings.TrimSpace(body), "\n"); len(want) == 0 || got != len(want) {
				t.Fatalf("export lang=%s: %d rows, want %d", lang, got, len(want))
			}
		}
		// The All-brands form: no list_id at all -- lang alone narrows every subscriber (read-only).
		for _, lang := range []string{"es", "en"} {
			want := f.want(false, "", lang)
			code, body := f.call(f.app.ExportSubscribers, http.MethodGet, "/api/subscribers/export?lang="+lang, "", plainUser)
			if code != http.StatusOK {
				t.Fatalf("export no list lang=%s: %d %s", lang, code, body)
			}
			if got := strings.Count(strings.TrimSpace(body), "\n"); len(want) == 0 || got != len(want) {
				t.Fatalf("export no list lang=%s: %d rows, want %d", lang, got, len(want))
			}
		}
		// EN is the send language: it includes the no-language subscribers.
		if en, none := f.want(false, "", "en"), f.want(false, "", "none"); len(none) == 0 || len(minus(none, en)) != 0 {
			t.Fatalf("fixture: lang=en must include every no-language subscriber (%d none)", len(none))
		}
	})
}
