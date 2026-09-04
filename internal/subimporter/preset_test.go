package subimporter

// Fork (import presets) -- table tests for the pure half of preset.go. Every fixture is
// synthetic (example.test addresses); nothing here names a real subscriber or deployment.

import (
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/models"
	"golang.org/x/text/encoding/charmap"
)

const testPresetJSON = `[{
  "key": "rewards",
  "name": "Import Rewards",
  "columns": { "email": "earner_email", "name": "earner_name", "locale": "earner_locale" },
  "attrib_columns": { "earned_from_site_url": "rewards_site_url" },
  "list_name_pattern": "^(?P<date>[0-9]{6})_rewards_(?P<kind>[a-z]+)_list( \\([0-9]+\\))?\\.csv$",
  "list_name_template": "{date} Rewards-{Kind}",
  "list_type": "private", "list_optin": "single",
  "subscription_status": "confirmed", "backfill": true, "merge": "fill",
  "skip_email_pattern": "@canceled\\.local$",
  "dedupe": { "name": "longest-first", "locale": "first-mapped", "attribs": "last" }
}]`

func testI18n(t *testing.T) *i18n.I18n {
	t.Helper()
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","subscribers.invalidEmail":"invalid email","subscribers.domainBlocklisted":"domain blocklisted"}`))
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func testImporter(t *testing.T) *Importer {
	t.Helper()
	return New(Options{}, nil, testI18n(t))
}

func testPreset(t *testing.T) *Preset {
	t.Helper()
	ps, err := ParsePresets([]byte(testPresetJSON), models.CampaignLangs)
	if err != nil {
		t.Fatalf("ParsePresets: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("want 1 preset, got %d", len(ps))
	}
	return &ps[0]
}

// I3 -- PlaceholderName is exactly what ValidateFields produces for an empty name.
func TestPresetPlaceholderNameMatchesValidateFields(t *testing.T) {
	im := testImporter(t)
	for _, email := range []string{
		"ann42@example.test", "jane.doe@example.test", "bine-j@example.test",
		"first.middle.last@example.test", "user+tag@example.test", "UPPER.Case@Example.TEST",
		"x@example.test", "a.b-c_d@example.test", "12345@example.test", "o.brien@example.test",
	} {
		got, err := im.ValidateFields(SubReq{Subscriber: models.Subscriber{Email: email}})
		if err != nil {
			t.Fatalf("%s: %v", email, err)
		}
		if want := PlaceholderName(strings.ToLower(email)); got.Name != want {
			t.Errorf("%s: ValidateFields=%q PlaceholderName=%q", email, got.Name, want)
		}
		if got.Name == "" {
			t.Errorf("%s: placeholder must never be empty", email)
		}
	}
	// The documented shapes.
	for email, want := range map[string]string{
		"ann42@example.test": "Ann42", "jane.doe@example.test": "Jane Doe", "bine-j@example.test": "Bine-J",
	} {
		if got := PlaceholderName(email); got != want {
			t.Errorf("PlaceholderName(%s)=%q want %q", email, got, want)
		}
	}
}

// I9 -- encoding detection and round-trip.
func TestPresetDecodeText(t *testing.T) {
	names := []string{"Mariné", "Feeß", "O’Donnell"}
	src := strings.Join(names, ",")

	// Valid UTF-8 is used as-is, with and without a BOM.
	if got, enc := DecodeText([]byte(src)); got != src || enc != EncodingUTF8 {
		t.Errorf("utf-8: got %q/%s", got, enc)
	}
	if got, enc := DecodeText(append([]byte{0xEF, 0xBB, 0xBF}, []byte(src)...)); got != src || enc != EncodingUTF8 {
		t.Errorf("utf-8 BOM: got %q/%s", got, enc)
	}

	// Windows-1252 bytes decode to the same text.
	enc1252, err := charmap.Windows1252.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got, enc := DecodeText(enc1252); got != src || enc != EncodingWindows1252 {
		t.Errorf("1252: got %q/%s", got, enc)
	}
}

// I11 -- lang derivation against the campaign-language constant.
func TestPresetLangFor(t *testing.T) {
	p := testPreset(t)
	for locale, want := range map[string]string{
		"de-DE": "de", "en_US": "en", "de-US": "de", "fr": "fr", "pt-BR": "", "": "",
		"EN-GB": "en", " it-IT ": "it", "es-419": "es", "zz": "", "-": "",
	} {
		if got := p.LangFor(locale); got != want {
			t.Errorf("LangFor(%q)=%q want %q", locale, got, want)
		}
	}
	// The allowed set IS models.CampaignLangs, never a second list.
	for _, l := range models.CampaignLangs {
		if got := p.LangFor(l + "-XX"); got != l {
			t.Errorf("CampaignLangs %s not honoured: %q", l, got)
		}
	}
}

// I12 -- filename to list name.
func TestPresetListName(t *testing.T) {
	p := testPreset(t)
	for file, want := range map[string]string{
		"090426_rewards_bundle_list.csv":     "090426 Rewards-Bundle",
		"090426_rewards_bundle_list (1).csv": "090426 Rewards-Bundle",
		"090426_rewards_product_list.csv":    "090426 Rewards-Product",
	} {
		got, err := p.ListName(file)
		if err != nil || got != want {
			t.Errorf("ListName(%s)=%q,%v want %q", file, got, err, want)
		}
	}
	_, err := p.ListName("rewards.csv")
	if err == nil || !strings.Contains(err.Error(), p.ListNamePattern) {
		t.Errorf("non-matching filename must be rejected with the pattern in the message, got %v", err)
	}
	var fe *FilenameError
	if !errorsAs(err, &fe) {
		t.Errorf("want *FilenameError, got %T", err)
	}
}

func errorsAs(err error, target **FilenameError) bool {
	fe, ok := err.(*FilenameError)
	if ok {
		*target = fe
	}
	return ok
}

// I10 -- dedupe, plus I16 skip pattern and the transform's row handling.
func TestPresetTransformDedupe(t *testing.T) {
	p, im := testPreset(t), testImporter(t)
	csv := "\uFEFF Earner_Name ,earner_email,earner_locale,earned_from_site_url\r\n" +
		"Ann Lee,ann@example.test,de-DE,https://example.test/a1\r\n" +
		"Ann Marie Lee,ANN@example.test,en-US,https://example.test/a2\r\n" +
		"Bob Ray,bob@example.test,,https://example.test/b1\r\n" +
		"Bob Roy,bob@example.test,fr,\r\n" +
		"Cy,cy@example.test,pt-BR,https://example.test/c1\r\n" +
		"Cy,cy@example.test,en-US,https://example.test/c2\r\n" +
		"Di,di@example.test,es-MX,https://example.test/d1\r\n" +
		"Di,di@example.test,es-MX,https://example.test/d2\r\n" +
		",eve@example.test,it-IT,https://example.test/e1\r\n" +
		"  Eve   Adams ,eve@example.test,,\r\n" +
		"Zed,zed@example.test,zz-ZZ,\r\n" +
		"Gone,canceled-abc@canceled.local,en-US,https://example.test/g\r\n" +
		"Bad,not-an-email,en-US,\r\n" +
		"Nobody,,en-US,\r\n" +
		",,,\r\n" +
		"Fé,fe@example.test,fr-CA,\r\n"

	out, err := p.Transform(im, []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if out.Encoding != EncodingUTF8 {
		t.Errorf("encoding %s", out.Encoding)
	}
	if out.Rows != 15 {
		t.Errorf("rows %d want 15 (blank line not counted)", out.Rows)
	}
	if len(out.Subs) != 7 {
		t.Fatalf("subs %d want 7", len(out.Subs))
	}
	byEmail := map[string]SubReq{}
	for _, s := range out.Subs {
		byEmail[s.Email] = s
	}
	want := map[string]struct {
		name, lang, url string
	}{
		"ann@example.test": {"Ann Marie Lee", "de", "https://example.test/a2"}, // longest name, first mapped locale, last url
		"bob@example.test": {"Bob Ray", "fr", "https://example.test/b1"},       // equal-length tie keeps the first, empty url does not blank
		"cy@example.test":  {"Cy", "en", "https://example.test/c2"},            // pt-BR ahead of en-US must not lose en
		"di@example.test":  {"Di", "es", "https://example.test/d2"},
		"eve@example.test": {"Eve Adams", "it", "https://example.test/e1"}, // whitespace collapsed; empty first name loses
		"zed@example.test": {"Zed", "", ""},
		"fe@example.test":  {"Fé", "fr", ""},
	}
	for email, w := range want {
		s, ok := byEmail[email]
		if !ok {
			t.Errorf("%s missing", email)
			continue
		}
		if s.Name != w.name {
			t.Errorf("%s name %q want %q", email, s.Name, w.name)
		}
		lang, _ := s.Attribs["lang"].(string)
		if lang != w.lang {
			t.Errorf("%s lang %q want %q", email, lang, w.lang)
		}
		if _, has := s.Attribs["lang"]; w.lang == "" && has {
			t.Errorf("%s must carry no lang key", email)
		}
		url, _ := s.Attribs["rewards_site_url"].(string)
		if url != w.url {
			t.Errorf("%s url %q want %q", email, url, w.url)
		}
		if _, has := s.Attribs["rewards_site_url"]; w.url == "" && has {
			t.Errorf("%s must carry no url key", email)
		}
		if !s.Backfill {
			t.Errorf("%s must be backfill", email)
		}
	}
	// Order is first-seen.
	if out.Subs[0].Email != "ann@example.test" || out.Subs[6].Email != "fe@example.test" {
		t.Errorf("order not first-seen: %s .. %s", out.Subs[0].Email, out.Subs[6].Email)
	}

	if len(out.Duplicates) != 5 {
		t.Errorf("duplicates %d want 5: %+v", len(out.Duplicates), out.Duplicates)
	}
	for _, d := range out.Duplicates {
		if d.Email == "ann@example.test" && (len(d.Rows) != 2 || d.Rows[0] != 2 || d.Rows[1] != 3) {
			t.Errorf("ann rows %v", d.Rows)
		}
	}
	// I16 -- the skip pattern, plus invalid and empty emails, each counted with a reason.
	if len(out.Skipped) != 3 {
		t.Fatalf("skipped %d want 3: %+v", len(out.Skipped), out.Skipped)
	}
	if s := out.Skipped[0]; s.Row != 13 || s.Email != "canceled-abc@canceled.local" || !strings.Contains(s.Reason, "skip pattern") {
		t.Errorf("skip pattern row: %+v", s)
	}
	if s := out.Skipped[1]; s.Row != 14 || s.Reason != "invalid email" {
		t.Errorf("invalid email row: %+v", s)
	}
	if s := out.Skipped[2]; s.Row != 15 {
		t.Errorf("empty email row: %+v", s)
	}
	if out.LangLess != 1 || len(out.Unmapped) != 1 || out.Unmapped[0].Email != "zed@example.test" || out.Unmapped[0].Locale != "zz-ZZ" {
		t.Errorf("lang-less %d unmapped %+v", out.LangLess, out.Unmapped)
	}
	if len(out.NonASCIINames) != 1 || out.NonASCIINames[0] != "Fé" {
		t.Errorf("non-ascii names %v", out.NonASCIINames)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings %v", out.Warnings)
	}
}

// Optional columns missing from the header warn; a missing email column is an error.
func TestPresetTransformHeaders(t *testing.T) {
	p, im := testPreset(t), testImporter(t)
	out, err := p.Transform(im, []byte("EARNER_EMAIL\nx@example.test\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Subs) != 1 || out.Subs[0].Name != "X" {
		t.Errorf("subs %+v", out.Subs)
	}
	if len(out.Warnings) != 3 {
		t.Errorf("want 3 warnings (name, locale, attrib column), got %v", out.Warnings)
	}
	if _, err := p.Transform(im, []byte("name,locale\nx,en\n")); err == nil || !strings.Contains(err.Error(), "email column") {
		t.Errorf("missing email column must error, got %v", err)
	}
}

// I13 -- the session options come from the preset alone, and a hash mismatch is rejected
// before anything is touched (PrepareImport never reaches the DB, which is nil here).
func TestPresetSessionOptAndHash(t *testing.T) {
	p, im := testPreset(t), testImporter(t)
	opt := p.SessionOpt("090426_rewards_bundle_list.csv", 42)
	if opt.Mode != ModeSubscribe || opt.SubStatus != "confirmed" || !opt.Backfill || opt.Merge != MergeFill ||
		opt.Overwrite || opt.OverwriteUserInfo || opt.OverwriteSubStatus || len(opt.ListIDs) != 1 || opt.ListIDs[0] != 42 {
		t.Errorf("session opt not fixed by the preset: %+v", opt)
	}

	data := []byte("earner_email\nx@example.test\n")
	if _, err := PrepareImport(nil, nil, im, p, "090426_rewards_bundle_list.csv", data, "deadbeef"); err != ErrHashMismatch {
		t.Errorf("want ErrHashMismatch, got %v", err)
	}
	if h := ContentHash(data); len(h) != 64 || h != ContentHash(data) {
		t.Errorf("hash shape %q", h)
	}
	// The matching hash passes the check and proceeds to the transform (nil DB is fine for a
	// file whose filename does not match: the filename check runs before any DB use).
	if _, err := PrepareImport(nil, nil, im, p, "nope.csv", data, ContentHash(data)); err == nil || err == ErrHashMismatch {
		t.Errorf("want filename error after a matching hash, got %v", err)
	}
}

// I15 -- a malformed preset fails to load (the caller hides the feature and logs).
func TestParsePresetsErrors(t *testing.T) {
	good := testPresetJSON
	cases := map[string]string{
		"bad json":            `[{`,
		"bad regex":           strings.Replace(good, `_list( \\(`, `_list(( \\(`, 1),
		"template group":      strings.Replace(good, "{date} Rewards-{Kind}", "{date} Rewards-{Nope}", 1),
		"unknown merge":       strings.Replace(good, `"merge": "fill"`, `"merge": "overwrite"`, 1),
		"unknown dedupe":      strings.Replace(good, `"name": "longest-first"`, `"name": "shortest"`, 1),
		"bad skip pattern":    strings.Replace(good, `"@canceled\\.local$"`, `"@canceled\\.local($"`, 1),
		"missing email":       strings.Replace(good, `"email": "earner_email", `, ``, 1),
		"unknown column role": strings.Replace(good, `"locale": "earner_locale"`, `"locale": "earner_locale", "phone": "p"`, 1),
		"bad status":          strings.Replace(good, `"subscription_status": "confirmed"`, `"subscription_status": "maybe"`, 1),
		"bad list type":       strings.Replace(good, `"list_type": "private"`, `"list_type": "secret"`, 1),
		"attrib lang":         strings.Replace(good, `"rewards_site_url"`, `"lang"`, 1),
		"bad key":             strings.Replace(good, `"key": "rewards"`, `"key": "Re wards"`, 1),
		"duplicate key":       strings.TrimSuffix(good, "]") + "," + strings.TrimPrefix(good, "["),
	}
	for name, js := range cases {
		if _, err := ParsePresets([]byte(js), models.CampaignLangs); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	// Empty and null are "no presets", not errors.
	for _, js := range []string{"", "[]", "null"} {
		ps, err := ParsePresets([]byte(js), models.CampaignLangs)
		if err != nil || len(ps) != 0 {
			t.Errorf("%q: %v %v", js, ps, err)
		}
	}
	// Absent optional fields take safe defaults (backfill on).
	ps, err := ParsePresets([]byte(`[{"key":"k","name":"N","columns":{"email":"e"},"list_name_pattern":"^(?P<d>.+)\\.csv$","list_name_template":"{d}","merge":"fill"}]`), models.CampaignLangs)
	if err != nil {
		t.Fatal(err)
	}
	if p := ps[0]; !*p.Backfill || p.ListType != "private" || p.ListOptin != "single" || p.SubscriptionStatus != "confirmed" || p.Dedupe.Name != "longest-first" {
		t.Errorf("defaults: %+v", p)
	}
}
