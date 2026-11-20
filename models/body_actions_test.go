package models

// Fork (integrations SAFE-ACTION-BODY-SPEC I1, I2, I3, I6, I12) -- BodyActions reports every
// action that is not a plain value, on Go's parse of the text the compile parses; it never
// executes, writes or fails; its wire shape is one fixture shared with the integrations repo.

import (
	"encoding/json"
	"html/template"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Masterminds/sprig/v3"
)

// bodyActionFuncs is the stub function map the report runs with in these tests: the manager's
// campaign functions (stubs, never called) plus sprig. The report depends on the map — an
// unknown function is a compile-error — so the fixture is read with exactly this map.
func bodyActionFuncs() template.FuncMap {
	f := template.FuncMap{
		"TrackLink":      func(url string, _ any) string { return url },
		"TrackView":      func(_ any) template.HTML { return "" },
		"UnsubscribeURL": func(_ any) string { return "" },
		"ManageURL":      func(_ any) string { return "" },
		"OptinURL":       func(_ any) string { return "" },
		"MessageURL":     func(_ any) string { return "" },
		"JoinURL":        func(_ any) string { return "" },
		"ArchiveURL":     func() string { return "" },
		"RootURL":        func() string { return "" },
		"Date":           func(string) string { return "" },
		"L":              func() any { return nil },
		"Safe":           func(s string) template.HTML { return template.HTML(s) },
	}
	maps.Copy(f, sprig.GenericFuncMap())
	return f
}

// The Stage 2 record's hostile stored bodies (reviews/SAFE-ACTION-BODY-stage2.md F1, F2, F5),
// in the shape the builder stores them.
const (
	// F1: a Button url typed with the entity text — the anchor href and the VML marker both
	// hold it with &amp;quot;. No {{ is stored for it.
	bodyF1 = `<a href="https://a.example/&amp;quot; . | or (printf &amp;quot;--%c%cdiv style=font-size:80px;width:2000px%cBIG%c/div%c%c!--&amp;quot; 62 60 62 60 62 60 | Safe) &amp;quot;" target="_blank">Go</a>` +
		`{{ Safe "\x3c!--[if mso]\x3e\x3cv:roundrect href=\x22" }}<span data-lm-vml-href="https://a.example/&amp;quot; . | or (printf &amp;quot;--%c%cdiv style=font-size:80px;width:2000px%cBIG%c/div%c%c!--&amp;quot; 62 60 62 60 62 60 | Safe) &amp;quot;"></span>{{ Safe "\x22\x3e\x3c/v:roundrect\x3e\x3c![endif]--\x3e" }}`
	wantF1 = `{{TrackLink "https://a.example/" . | or (printf "--%c%cdiv style=font-size:80px;width:2000px%cBIG%c/div%c%c!--" 62 60 62 60 62 60 | Safe) "" .}}`

	// F2: three blocks — Html, a Button, Html — whose href text the dynamic-href wrap turns into
	// a string, making the stored {{ if false }} … {{ end }} real.
	bodyF2 = `<table><tr><td><p style="margin:0">Open href="}}{{" {{ if false }} y "}}</p></td></tr></table>` +
		`<table><tr><td><a href="https://a.example/button">Button</a></td></tr></table>` +
		`<table><tr><td><p style="margin:0">Close href="}}{{" {{ end }} y "}}</p></td></tr></table>`

	// F5 case 1: a builder-form Safe literal whose quote entities Go decodes into a pipeline.
	bodyF5 = `<p style="margin:0">Hand {{ Safe "\x3c!--&quot; | and &quot;&quot; | print &quot;\x3cdiv style=font-size:80px;width:2000px\x3eBIG\x3c/div\x3e&quot; | Safe | and &quot;--\x3e" }}</p>`
	wantF5 = `{{Safe "\x3c!--" | and "" | print "\x3cdiv style=font-size:80px;width:2000px\x3eBIG\x3c/div\x3e" | Safe | and "--\x3e"}}`

	bodyDIV = `"\x3cdiv style=font-size:80px;width:2000px\x3eBIG\x3c/div\x3e"`
)

func bodyKinds(r BodyActionReport) []string {
	out := []string{}
	for _, a := range r.Actions {
		out = append(out, a.Kind)
	}
	return out
}

type bodyActionRow struct {
	name, ct, body string
	kinds          []string
	text           string // when set, the first finding's text
}

func bodyActionRows() []bodyActionRow {
	v := CampaignContentTypeVisual
	return []bodyActionRow{
		// §1.2 — reported.
		{"pipe safe", v, `<p>A {{ ` + bodyDIV + ` | Safe }} z</p>`, []string{"safe"}, ""},
		{"safe of print", v, `<p>A {{ Safe (print ` + bodyDIV + `) }} z</p>`, []string{"safe"}, ""},
		{"or of safe", v, `<p>{{ or (Safe "x") "" }}</p>`, []string{"safe"}, ""},
		{"raw string literal", v, "<p>{{ Safe `<div>x</div>` }}</p>", []string{"safe"}, ""},
		{"declared safe", v, `<p>{{ $x := Safe "x" }}</p>`, []string{"variable"}, ""},
		{"condition across blocks", v, `<p>Open {{ if false }} one</p><a href="https://a.example/">B</a><p>Close {{ end }} three</p>`, []string{"if"}, "{{if false}}"},
		{"condition in a block", v, `<p>Hi {{ if .Subscriber.FirstName }}friend{{ else }}there{{ end }}, welcome.</p>`, []string{"if"}, "{{if .Subscriber.FirstName}}"},
		{"define and template", v, `<p>First {{ define "x" }}<u>moved</u>{{ end }} tail</p><p>Second {{ template "x" . }} tail</p>`, []string{"define", "template"}, "x"},
		{"block", v, `<p>{{ block "b" . }}<u>x</u>{{ end }}</p>`, []string{"define", "template"}, "b"},
		{"comment", v, `<p>{{/* }} <div>x</div> */}}</p>`, []string{"comment"}, ""},
		{"range", v, `<p>{{ range .Subscriber.Attribs }}x{{ end }}</p>`, []string{"range"}, ""},
		{"with", v, `<p>{{ with .Subscriber.Attribs }}x{{ end }}</p>`, []string{"with"}, ""},
		{"F1 button url", v, bodyF1, []string{"safe"}, wantF1},
		{"F2 hidden condition", v, bodyF2, []string{"if"}, "{{if false}}"},
		{"F5 order case", v, bodyF5, []string{"safe"}, wantF5},
		{"undefined function", v, `<p>{{ NoSuchFunc }}</p>`, []string{"compile-error"}, ""},
		// I1 additions.
		{"if without space", v, `<p>{{if(false)}}x{{end}}</p>`, []string{"if"}, ""},
		{"trimmed else", v, `<p>{{ if .X }}a{{- else -}}b{{ end }}</p>`, []string{"if"}, ""},
		{"trim tab if", v, "<p>{{-\tif .X }}a{{ end }}</p>", []string{"if"}, ""},
		{"trim line-break comment", v, "<p>{{-\n/* c */}}</p>", []string{"comment"}, ""},
		{"else if", v, `<p>{{ if .X }}a{{ else if .Y }}b{{ end }}</p>`, []string{"if", "if"}, ""},
		{"safe on line two", v, "<p>{{ print\n" + bodyDIV + " | Safe }}</p>", []string{"safe"}, ""},
		{"break and continue", v, `<p>{{ range .Subscriber.Attribs }}{{ break }}{{ continue }}{{ end }}</p>`, []string{"range", "break", "continue"}, ""},
		// §1.2 — not reported.
		{"plain values", v, `<p>Hi {{ .Subscriber.FirstName }} {{ or .Subscriber.FirstName &quot;there&quot; }} <a href="{{ UnsubscribeURL }}">u</a> {{ Date "2006" }}</p>`, nil, ""},
		{"tracked static href", v, `<a href="https://a.example/?a=1&amp;b=2">x</a>`, nil, ""},
		{"builder literal", v, `{{ Safe "\x3c!--[if\x20mso]\x3e" }}<p>x</p>{{ Safe "\x3c![endif]--\x3e" }}`, nil, ""},
		{"attribs field named Safe", v, `<p>{{ .Subscriber.Attribs.Safe }}</p>`, nil, ""},
		{"string Stay Safe", v, `<p>{{ or .X "Stay Safe" }}</p>`, nil, ""},
		{"markdown body", CampaignContentTypeMarkdown, "# Hi {{ .Subscriber.FirstName }}\n\nText.", nil, ""},
		{"empty body", v, ``, nil, ""},
		// Each content type with one plain value.
		{"plain visual", v, `<p>{{ .Subscriber.FirstName }}</p>`, nil, ""},
		{"plain html", CampaignContentTypeHTML, `<p>{{ .Subscriber.FirstName }}</p>`, nil, ""},
		{"plain richtext", CampaignContentTypeRichtext, `<p>{{ .Subscriber.FirstName }}</p>`, nil, ""},
		{"plain markdown", CampaignContentTypeMarkdown, `{{ .Subscriber.FirstName }}`, nil, ""},
		{"plain plain", CampaignContentTypePlain, `Hi {{ .Subscriber.FirstName }}`, nil, ""},
		{"plain body condition", CampaignContentTypePlain, `Hi {{ if .X }}a{{ end }}`, []string{"if"}, ""},
		// I12 (Stage 2b) — R1: the literal form is exempt in visual only.
		{"literal visual", v, `<p>{{ Safe "\x3cb\x3e" }}</p>`, nil, ""},
		{"literal html", CampaignContentTypeHTML, `<p>{{ Safe "\x3cb\x3e" }}</p>`, []string{"safe"}, ""},
		{"literal richtext", CampaignContentTypeRichtext, `<p>{{ Safe "\x3cb\x3e" }}</p>`, []string{"safe"}, ""},
		{"literal markdown", CampaignContentTypeMarkdown, `{{ Safe "\x3cb\x3e" }}`, []string{"safe"}, ""},
		{"literal plain", CampaignContentTypePlain, `{{ Safe "\x3cb\x3e" }}`, []string{"safe"}, ""},
		{"trimmed literal visual", v, `<p>{{- Safe "x" -}}</p>`, nil, ""},
		// The pair of the integrations row: Go's literal form with an unescaped `<` is exempt in
		// visual (the integrations shape reads it — there it is unread:body in the canvas).
		{"literal unescaped visual", v, `<p>{{ Safe "<b>" }}</p>`, nil, ""},
		{"parenthesised safe", v, `<p>{{ (Safe "x") }}</p>`, []string{"safe"}, ""},
		{"index list safe", v, `<p>{{ index (list (Safe "x")) 0 }}</p>`, []string{"safe"}, ""},
		// R2: a string that swallows stored markup, or hides it.
		{"len raw string", v, "<p>Open {{ len `}} one</p><table><tr><td>x</td></tr></table><p>Close ` }} three</p>", []string{"string"}, ""},
		{"or braces", v, `<p>{{ or .X "}}" }}</p>`, []string{"string"}, ""},
		{"or markup", v, `<p>{{ or .X "<b>" }}</p>`, []string{"string"}, ""},
		{"tracklink exemption", v, `<p>{{ TrackLink "}}{{" . }}</p>`, nil, ""},
		{"tracklink raw string", v, "<p>{{ TrackLink `}}` . }}</p>", []string{"string"}, ""},
		{"if eq bracket", v, `<p>{{ if eq .X "<" }}a{{ end }}</p>`, []string{"if", "string"}, ""},
		// R3: a field chain reaching the compiled template.
		{"tpl option", v, `<p>{{ .Campaign.Tpl.Option "missingkey=error" }}</p>`, []string{"engine"}, ""},
		{"tpl tree parse", v, `<p>{{ and true (.Campaign.Tpl.Tree.Parse "x" "" "" nil) }}</p>`, []string{"engine"}, ""},
		{"compile template", v, `<p>{{ .Campaign.CompileTemplate (dict) }}</p>`, []string{"engine"}, ""},
		{"variable tpl", v, `<p>{{ $c := .Campaign }}{{ $c.Tpl.Name }}</p>`, []string{"variable", "engine"}, ""},
		{"campaign name", v, `<p>{{ .Campaign.Name }}</p>`, nil, ""},
		{"assign", v, `<p>{{ $x := 0 }}{{ $x = 1 }}</p>`, []string{"variable", "variable"}, ""},
		{"range int", v, `<p>{{ range 3 }}x{{ end }}</p>`, []string{"range"}, ""},
		// R7: a body that is only {{define "content"}} — pinned, so the stated gap is deliberate.
		{"define content only", v, "\n{{define \"content\"}}<b>x</b>{{end}}\n", nil, ""},
	}
}

// TestBodyActionsTable -- I1 (the report is exact on Go's parse) and I2 (no row that compiles
// reports a mismatch: the report's text is the compile's text).
func TestBodyActionsTable(t *testing.T) {
	f := bodyActionFuncs()
	for _, row := range bodyActionRows() {
		rep := BodyActions(row.body, row.ct, f)
		got := bodyKinds(rep)
		want := row.kinds
		if want == nil {
			want = []string{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: kinds %v, want %v (report %+v)", row.name, got, want, rep)
			continue
		}
		if row.text != "" && rep.Actions[0].Text != row.text {
			t.Errorf("%s: text %q, want %q", row.name, rep.Actions[0].Text, row.text)
		}
		if rep.V != 1 || rep.Total != len(want) {
			t.Errorf("%s: v %d total %d", row.name, rep.V, rep.Total)
		}
		if (rep.Total == 0) != (rep.Digest == "") {
			t.Errorf("%s: digest %q for total %d", row.name, rep.Digest, rep.Total)
		}
		for _, a := range rep.Actions {
			if a.Kind == "mismatch" {
				t.Errorf("%s: I2 mismatch", row.name)
			}
		}
	}
}

// TestBodyActionsNeverExecutes -- I3: no template function is called, a broken function map is
// one error finding (no panic), an uncompilable body one compile-error, the cap and the cut.
func TestBodyActionsNeverExecutes(t *testing.T) {
	calls := 0
	f := bodyActionFuncs()
	f["TrackLink"] = func(url string, _ any) string { calls++; return url }
	f["TrackView"] = func(_ any) template.HTML { calls++; return "" }
	f["Safe"] = func(s string) template.HTML { calls++; return template.HTML(s) }
	rep := BodyActions(`<a href="https://a.example/">x</a>{{ TrackView }}{{ TrackLink "https://b.example/" . }}{{ Safe "x" }}{{ `+bodyDIV+` | Safe }}`, CampaignContentTypeVisual, f)
	if calls != 0 {
		t.Fatalf("%d template function calls", calls)
	}
	if rep.Total != 1 || rep.Actions[0].Kind != "safe" {
		t.Fatalf("report %+v", rep)
	}

	bad := bodyActionFuncs()
	bad["NotAFunc"] = "a string"
	rep = BodyActions(`<p>x</p>`, CampaignContentTypeVisual, bad)
	if rep.Total != 1 || rep.Actions[0].Kind != "error" || rep.Digest == "" {
		t.Fatalf("not-a-function map: %+v", rep)
	}

	rep = BodyActions(`<p>{{ if }}</p>`, CampaignContentTypeVisual, bodyActionFuncs())
	if rep.Total != 1 || rep.Actions[0].Kind != "compile-error" {
		t.Fatalf("uncompilable: %+v", rep)
	}

	rep = BodyActions(strings.Repeat(`{{if true}}x{{end}}`, 60), CampaignContentTypeVisual, bodyActionFuncs())
	if rep.Total != 60 || len(rep.Actions) != 50 {
		t.Fatalf("cap: total %d actions %d", rep.Total, len(rep.Actions))
	}

	// A 250-byte action whose text has a multi-byte rune across byte 200.
	prefix := `{{Safe (print "`
	long := prefix + strings.Repeat("a", 199-len(prefix)) + strings.Repeat("é", 30) + `")}}`
	rep = BodyActions(`<p>`+long+`</p>`, CampaignContentTypeVisual, bodyActionFuncs())
	if rep.Total != 1 {
		t.Fatalf("long: %+v", rep)
	}
	txt := rep.Actions[0].Text
	if len(long) <= 200 || len(txt) > 200 || !utf8.ValidString(txt) || !strings.HasPrefix(long, txt) {
		t.Fatalf("cut: %d bytes, valid %v, %q", len(txt), utf8.ValidString(txt), txt)
	}
}

// TestBodyActionsDigest -- I12 (R4): the digest covers every finding, uncut and uncapped.
func TestBodyActionsDigest(t *testing.T) {
	f := bodyActionFuncs()
	fifty := strings.Repeat(`{{if true}}x{{end}}`, 50)
	a := BodyActions(fifty+`{{if true}}x{{end}}`, CampaignContentTypeVisual, f)
	b := BodyActions(fifty+`{{if false}}x{{end}}`, CampaignContentTypeVisual, f)
	if !reflect.DeepEqual(a.Actions, b.Actions) || a.Digest == b.Digest {
		t.Fatalf("51st finding: same actions %v, digests %s %s", reflect.DeepEqual(a.Actions, b.Actions), a.Digest, b.Digest)
	}
	pad := strings.Repeat("a", 220)
	c := BodyActions(`<p>{{ Safe (print "`+pad+`x") }}</p>`, CampaignContentTypeVisual, f)
	d := BodyActions(`<p>{{ Safe (print "`+pad+`y") }}</p>`, CampaignContentTypeVisual, f)
	if !reflect.DeepEqual(c.Actions, d.Actions) || c.Digest == d.Digest {
		t.Fatalf("after byte 200: same actions %v, digests %s %s", reflect.DeepEqual(c.Actions, d.Actions), c.Digest, d.Digest)
	}
	if e := BodyActions(`<p>x</p>`, CampaignContentTypeVisual, f); e.Digest != "" || e.Actions == nil {
		t.Fatalf("empty report: %+v", e)
	}
}

// TestBodyActionsFixture -- I6: the wire shape is one fixture on both sides. This file reads
// models/testdata/body-actions.json; the integrations repo's
// tests/fixtures/campaign-review/body-actions.json is a byte-identical copy read by
// tests/lib/campaign-review/body-actions.test.ts. Change them together.
func TestBodyActionsFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/body-actions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Name        string          `json:"name"`
			ContentType string          `json:"content_type"`
			Body        string          `json:"body"`
			Report      json.RawMessage `json:"report"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 5 {
		t.Fatalf("%d cases", len(fx.Cases))
	}
	f := bodyActionFuncs()
	for _, c := range fx.Cases {
		got, _ := json.Marshal(BodyActions(c.Body, c.ContentType, f))
		var want, have any
		_ = json.Unmarshal(c.Report, &want)
		_ = json.Unmarshal(got, &have)
		if !reflect.DeepEqual(want, have) {
			t.Errorf("%s: report %s, want %s", c.Name, got, c.Report)
		}
	}
}
