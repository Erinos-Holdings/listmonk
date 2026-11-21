package main

// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC T10 (the handler-permission and
// body-shape unit, no database): the three routes are registered with their permissions, and the
// feed body's validation (validatePromoPurchases) and the preview's parameters (promoPreviewParams).
// The stored behaviour is promo_db_test.go.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPromoRoutes(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, re := range []string{
		`g\.PUT\("/api/promo/purchases", pm\(a\.PutPromoPurchases, "subscribers:manage"\)\)`,
		`g\.GET\("/api/promo/purchases", pm\(a\.GetPromoPurchases, "subscribers:get_all"\)\)`,
		`g\.GET\("/api/campaigns/:id/promo-audience", pm\(hasID\(a\.GetCampaignPromoAudience\), "campaigns:manage_all", "campaigns:manage"\)\)`,
	} {
		if !regexp.MustCompile(re).Match(b) {
			t.Fatalf("handlers.go does not register %s", re)
		}
	}
}

func TestValidatePromoPurchases(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	hash := strings.Repeat("0f", 32)
	ord := func(id, at string, promos, skus []string) promoOrderReq {
		return promoOrderReq{OrderID: id, EmailHash: hash, OrderedAt: at, Promotions: promos, SKUs: skus}
	}
	req := func(o ...promoOrderReq) promoPurchasesReq { return promoPurchasesReq{Orders: &o} }

	rows, err := validatePromoPurchases(req(
		ord("A", "2026-11-20T10:00:00-07:00", []string{" BFCM-Deal-2 ", "BFCM-Deal-2", "komigobox"}, []string{"Exltd-1", "Exltd-1"}),
		ord("B", "2026-11-22T11:59:00Z", nil, nil), // no values: writes nothing; within 1 day ahead
	), now)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, fmt.Sprintf("%s/%s/%s@%s", r.OrderID, r.Kind, r.Value, r.OrderedAt.Format(time.RFC3339)))
	}
	if strings.Join(got, " ") != "A/promotion/BFCM-Deal-2@2026-11-20T17:00:00Z A/promotion/komigobox@2026-11-20T17:00:00Z A/sku/Exltd-1@2026-11-20T17:00:00Z" {
		t.Fatalf("rows %v", got)
	}
	if rows, err := validatePromoPurchases(req(), now); err != nil || len(rows) != 0 {
		t.Fatalf("an empty orders list: %v %v", rows, err)
	}
	if _, err := validatePromoPurchases(promoPurchasesReq{}, now); err == nil {
		t.Fatal("absent orders must refuse")
	}
	many := make([]string, 201)
	for i := range many {
		many[i] = fmt.Sprintf("v%d", i)
	}
	big := make([]promoOrderReq, 1001)
	for i := range big {
		big[i] = ord("A", "2026-11-20T10:00:00Z", []string{"x"}, nil)
	}
	for name, r := range map[string]promoPurchasesReq{
		"1001 orders":      {Orders: &big},
		"201 values":       req(ord("A", "2026-11-20T10:00:00Z", many[:150], many[150:])),
		"empty order id":   req(ord("", "2026-11-20T10:00:00Z", []string{"x"}, nil)),
		"129-char id":      req(ord(strings.Repeat("é", 129), "2026-11-20T10:00:00Z", []string{"x"}, nil)),
		"NUL in id":        req(ord("A\x00B", "2026-11-20T10:00:00Z", []string{"x"}, nil)),
		"control in id":    req(ord("A\tB", "2026-11-20T10:00:00Z", []string{"x"}, nil)),
		"value too long":   req(ord("A", "2026-11-20T10:00:00Z", []string{strings.Repeat("v", 129)}, nil)),
		"blank value":      req(ord("A", "2026-11-20T10:00:00Z", nil, []string{" "})),
		"control value":    req(ord("A", "2026-11-20T10:00:00Z", []string{"a\x00b"}, nil)),
		"bad time":         req(ord("A", "2026-11-20", []string{"x"}, nil)),
		"over a day ahead": req(ord("A", "2026-11-22T12:00:01Z", []string{"x"}, nil)),
		"bad hash":         {Orders: &[]promoOrderReq{{OrderID: "A", EmailHash: strings.Repeat("0F", 32), OrderedAt: "2026-11-20T10:00:00Z"}}},
	} {
		if _, err := validatePromoPurchases(r, now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// 200 values and a 128-character id / value are inside the limits.
	if _, err := validatePromoPurchases(req(ord(strings.Repeat("é", 128), "2026-11-20T10:00:00Z", many[:100], append(many[100:200:200], strings.Repeat("v", 128)[:128]))), now); err == nil {
		t.Error("201 values (100 + 101) must refuse")
	}
	if _, err := validatePromoPurchases(req(ord(strings.Repeat("é", 128), "2026-11-20T10:00:00Z", many[:100], many[100:200])), now); err != nil {
		t.Errorf("200 values refused: %v", err)
	}
}

func TestPromoPreviewParams(t *testing.T) {
	code, step, tags, err := promoPreviewParams(map[string][]string{"code": {" C "}, "step": {"2"}, "tags": {"BFCM-Deal-2, bfcm-deal-2 , BFCM*"}})
	if err != nil || *code != `"C"` || *step != "2" || *tags != `["BFCM-Deal-2","BFCM*"]` {
		t.Fatalf("%v %v %v %v", code, step, tags, err)
	}
	code, step, tags, err = promoPreviewParams(map[string][]string{})
	if err != nil || code != nil || step != nil || tags != nil {
		t.Fatalf("absent = stored: %v %v %v %v", code, step, tags, err)
	}
	code, step, tags, err = promoPreviewParams(map[string][]string{"code": {""}, "step": {""}, "tags": {""}})
	if err != nil || *code != `""` || *step != "null" || *tags != `[]` {
		t.Fatalf("blank = cleared: %v %v %v %v", *code, *step, *tags, err)
	}
	for name, q := range map[string]map[string][]string{
		"step 4":    {"step": {"4"}},
		"step x":    {"step": {"x"}},
		"code long": {"code": {strings.Repeat("x", 31)}},
		"tag *":     {"tags": {"a*b"}},
		"tag short": {"tags": {"ab*"}},
	} {
		if _, _, _, err := promoPreviewParams(q); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
