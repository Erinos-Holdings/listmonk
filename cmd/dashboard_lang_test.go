package main

// Fork (global language filter) -- integrations GLOBAL-LANG-SPEC I7, pure and at the handler: the
// Dashboard counts, charts, clients and countries reads and the Brand Matrix take an optional
// `lang` that is '' (absent = All) or exactly one of models.CampaignLangs; anything else is a 400
// before any statement runs. "Bound, not interpolated" is read in the Stage 4 diff (a test cannot
// tell the two apart once validation passes).

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

func TestDashboardLangParam(t *testing.T) {
	// The pure rule.
	ok := append([]string{""}, models.CampaignLangs...)
	for _, l := range ok {
		got, err := dashboardLang(url.Values{"lang": {l}})
		if err != nil || got != l {
			t.Errorf("lang=%q: got %q, %v; want %q, nil", l, got, err, l)
		}
	}
	if got, err := dashboardLang(url.Values{}); err != nil || got != "" {
		t.Errorf("absent lang: got %q, %v; want '', nil", got, err)
	}
	bad := []string{"xx", "EN", "fr-CA", "none", "en_only", "other", "pt", " fr", "fr;"}
	for _, l := range bad {
		if _, err := dashboardLang(url.Values{"lang": {l}}); err == nil {
			t.Errorf("lang=%q: want an error", l)
		}
	}

	// Every handler refuses an invalid lang with a 400 before touching the database (no core here:
	// a handler that reached a statement would panic, not return 400).
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","globals.messages.invalidFields":"Invalid fields: {name}"}`))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{i18n: i}
	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll})
	scoped := baUser([]string{auth.PermCampaignsGetAnalytics}, 3)
	handlers := map[string]echo.HandlerFunc{
		"/api/dashboard/counts":       a.GetDashboardCounts,
		"/api/dashboard/charts":       a.GetDashboardCharts,
		"/api/dashboard/clients":      a.GetDashboardClients,
		"/api/dashboard/countries":    a.GetDashboardCountries,
		"/api/dashboard/brand-matrix": a.GetDashboardBrandMatrix,
	}
	for path, h := range handlers {
		for _, u := range []auth.User{admin, scoped} {
			for _, l := range []string{"xx", "EN", "none"} {
				code, body := baCall(t, u, path+"?list_id=3&lang="+url.QueryEscape(l), h)
				if code != http.StatusBadRequest {
					t.Errorf("%s lang=%s: %d %s, want 400", path, l, code, body)
				}
				code, body = baCall(t, u, path+"?lang="+url.QueryEscape(l), h)
				if code != http.StatusBadRequest {
					t.Errorf("%s lang=%s (no list_id): %d %s, want 400", path, l, code, body)
				}
			}
		}
	}
}
