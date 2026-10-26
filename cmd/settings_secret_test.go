package main

// Fork (on-demand brand-health refresh) -- integrations BRANDHEALTH-REFRESH-SPEC I7, the no-DB
// half (the router half is health_refresh_db_test.go): the HMAC secrets are listed in
// secretSettingKeys (the per-key PUT route) and resolved by resolveSettingsSecrets (the full PUT) --
// a mask keeps the stored secret, a new value replaces it, a value merely containing the mask is
// refused.

import (
	"testing"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/models"
)

func TestHMACSecretSettingsAreSecret(t *testing.T) {
	for _, k := range []string{"app.health_refresh_hmac_secret", "app.review_hmac_secret"} {
		if !secretSettingKeys[k] {
			t.Fatalf("%s is not in secretSettingKeys", k)
		}
	}

	cur := models.Settings{HealthRefreshHMACSecret: "stored-refresh", ReviewHMACSecret: "stored-review"}

	set := models.Settings{HealthRefreshHMACSecret: core.MaskSecret("stored-refresh"), ReviewHMACSecret: ""}
	if err := resolveSettingsSecrets(&set, cur); err != nil {
		t.Fatal(err)
	}
	if set.HealthRefreshHMACSecret != "stored-refresh" || set.ReviewHMACSecret != "stored-review" {
		t.Fatalf("mask/empty did not keep the stored secrets: %q %q", set.HealthRefreshHMACSecret, set.ReviewHMACSecret)
	}

	set = models.Settings{HealthRefreshHMACSecret: "new-refresh"}
	if err := resolveSettingsSecrets(&set, cur); err != nil || set.HealthRefreshHMACSecret != "new-refresh" {
		t.Fatalf("new value: %q %v", set.HealthRefreshHMACSecret, err)
	}

	set = models.Settings{HealthRefreshHMACSecret: "x" + core.SecretMask}
	if err := resolveSettingsSecrets(&set, cur); err == nil {
		t.Fatal("a value containing the mask was accepted")
	}
}
