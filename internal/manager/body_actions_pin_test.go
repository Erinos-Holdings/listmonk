package manager

// Fork (integrations SAFE-ACTION-BODY-SPEC D3, I5) -- the facts models.BodyActions rests on,
// pinned by test rather than prose:
//   - the raw-markup functions: over the campaign function map (sprig included), the names whose
//     first result is an html/template content type are exactly Safe and TrackView. A new
//     function or a sprig upgrade that adds one fails here until BodyActions names it (or this
//     list records why it carries no author text, as TrackView does: it prints the pixel only);
//   - no function is named break or continue (Go lexes them as keywords only while no function
//     has the name, and the report's parses pass no functions);
//   - the data pin (R3): every exported method, and every exported field of a type a template
//     can walk into, of the data a campaign executes against is listed below as `engine` (named
//     in BodyActions' rule 3 — asserted) or `inert` with a reason. A struct field is reachable
//     from a template only by name, so rule 3's name test is complete while this pin holds.

import (
	"encoding/json"
	"html/template"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

func pinManager() *Manager {
	m := &Manager{
		cfg:   Config{UnsubURL: "https://lm.test/subscription/%s/%s"},
		log:   log.New(os.Stderr, "", 0),
		links: map[string]string{},
	}
	m.tplFuncs = m.makeGnericFuncMap()
	return m
}

// htmlContentTypes are html/template's seven content types (printed unescaped).
var htmlContentTypes = map[reflect.Type]bool{
	reflect.TypeOf(template.HTML("")):     true,
	reflect.TypeOf(template.HTMLAttr("")): true,
	reflect.TypeOf(template.JS("")):       true,
	reflect.TypeOf(template.JSStr("")):    true,
	reflect.TypeOf(template.CSS("")):      true,
	reflect.TypeOf(template.URL("")):      true,
	reflect.TypeOf(template.Srcset("")):   true,
}

// TestBodyActionsPlainPinType -- Stage 4 V7(b): a field typed as a content type (or a slice of one)
// is not plain, so a new such field on the data fails the data pin until it is listed.
func TestBodyActionsPlainPinType(t *testing.T) {
	for ct := range htmlContentTypes {
		if plainPinType(ct) || plainPinType(reflect.SliceOf(ct)) {
			t.Errorf("%s reads as plain", ct)
		}
	}
	if !plainPinType(reflect.TypeOf("")) {
		t.Error("string reads as not plain")
	}
}

func TestBodyActionsRawMarkupFunctions(t *testing.T) {
	m := pinManager()
	camp := models.Campaign{}
	funcs := m.TemplateFuncs(&camp)

	content := htmlContentTypes
	var raw []string
	for name, fn := range funcs {
		ft := reflect.TypeOf(fn)
		if ft == nil || ft.Kind() != reflect.Func {
			t.Fatalf("%s is not a function", name)
		}
		if ft.NumOut() > 0 && content[ft.Out(0)] {
			raw = append(raw, name)
		}
		if name == "break" || name == "continue" {
			t.Errorf("a function is named %q: Go would stop lexing it as a keyword", name)
		}
	}
	sort.Strings(raw)
	// Safe: the author's raw markup — models.BodyActions rule 2. TrackView: the open pixel
	// only, no author text.
	if want := []string{"Safe", "TrackView"}; !reflect.DeepEqual(raw, want) {
		t.Fatalf("raw-markup functions %v, want %v — name the new one in models.BodyActions", raw, want)
	}
}

// bodyActionsDataPin lists every exported method, and every exported non-plain field, of
// models.Campaign, models.Subscriber and CampaignMessage (keyed "Type.Name").
var bodyActionsDataPin = map[string]string{
	// engine: the compiled template itself, shared by every recipient (models rule 3).
	"Campaign.Tpl":             "engine",
	"Campaign.SubjectTpl":      "engine",
	"Campaign.PreheaderTpl":    "engine",
	"Campaign.AltBodyTpl":      "engine",
	"Campaign.HeaderTpls":      "engine",
	"Campaign.CompileTemplate": "engine",

	// inert: data the template reads as values.
	"Campaign.Base":              "inert: id and timestamps",
	"Campaign.CampaignMeta":      "inert: counters",
	"Campaign.Attachments":       "inert: attachment data, read-only",
	"Campaign.ReviewPassed":      "inert: a *bool courtesy flag",
	"Campaign.ConvertContent":    "inert: returns a converted copy of the body; mutates nothing",
	"Campaign.Preheader":         "inert: returns a string",
	"Campaign.Lang":              "inert: returns a string",
	"Campaign.SmartSending":      "inert: returns a bool",
	"Campaign.SilentSkip":        "inert: returns a bool",
	"Subscriber.Base":            "inert: id and timestamps",
	"Subscriber.FirstName":       "inert: returns a string",
	"Subscriber.LastName":        "inert: returns a string",
	"CampaignMessage.Campaign":   "inert: the campaign itself; its own members are pinned above",
	"CampaignMessage.Subscriber": "inert: the subscriber itself; its own members are pinned above",
	"CampaignMessage.Subject":    "inert: returns a copy",
	"CampaignMessage.Body":       "inert: returns a copy",
	"CampaignMessage.AltBody":    "inert: returns a copy",
}

// plainPinType is a field type a template reads as a value (no method or field below it that
// reaches the engine): scalars, strings, times, null.*, the JSON maps, Headers, and slices of
// those.
// Stage 4 V7(b): the seven html/template content types are strings that print UNESCAPED — never plain.
func plainPinType(t reflect.Type) bool {
	if htmlContentTypes[t] {
		return false
	}
	switch t {
	case reflect.TypeOf(time.Time{}), reflect.TypeOf(models.JSON{}), reflect.TypeOf(models.StringIntMap{}),
		reflect.TypeOf(models.Headers{}), reflect.TypeOf(json.RawMessage{}):
		return true
	}
	if t.PkgPath() == "gopkg.in/volatiletech/null.v6" {
		return true
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	case reflect.Map:
		return t.Key().Kind() == reflect.String && plainPinType(t.Elem())
	case reflect.Slice, reflect.Array:
		return plainPinType(t.Elem())
	case reflect.Interface:
		return t.NumMethod() == 0 && t.String() == "interface {}"
	}
	return false
}

func TestBodyActionsDataPin(t *testing.T) {
	found := map[string]bool{}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(models.Campaign{}),
		reflect.TypeOf(models.Subscriber{}),
		reflect.TypeOf(CampaignMessage{}),
	} {
		name := typ.Name()
		for _, f := range reflect.VisibleFields(typ) {
			if !f.IsExported() || plainPinType(f.Type) {
				continue
			}
			found[name+"."+f.Name] = true
		}
		pt := reflect.PointerTo(typ)
		for i := 0; i < pt.NumMethod(); i++ {
			found[name+"."+pt.Method(i).Name] = true
		}
	}
	for k := range found {
		v, ok := bodyActionsDataPin[k]
		if !ok {
			t.Errorf("%s is reachable from a campaign template and is not in the data pin: list it as engine (and in models.BodyActions rule 3) or inert with a reason", k)
		} else if v != "engine" && !strings.HasPrefix(v, "inert: ") {
			t.Errorf("%s: %q is neither engine nor inert: <reason>", k, v)
		}
	}
	for k := range bodyActionsDataPin {
		if !found[k] {
			t.Errorf("%s is in the data pin but no longer on the data — remove it", k)
		}
	}

	// The engine entries and BodyActions' rule 3 name the same names.
	var engine []string
	for k, v := range bodyActionsDataPin {
		if v == "engine" {
			engine = append(engine, k[strings.Index(k, ".")+1:])
		}
	}
	sort.Strings(engine)
	rule3 := models.BodyActionEngineNames()
	sort.Strings(rule3)
	if !reflect.DeepEqual(engine, rule3) {
		t.Fatalf("data pin engine names %v, models.BodyActions rule 3 %v", engine, rule3)
	}
}
