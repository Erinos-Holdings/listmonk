// Package linkresolve (fork, erinos personalized-button click tracking — CLICK-TRACKING-SPEC
// §3.2) holds the PURE logic behind dynamic tracked links: a tracked link whose stored URL is
// an unexpanded Go template expression (`{{ .Subscriber.Attribs.site }}`) is resolved against
// the clicking subscriber at redirect time, validated, and — when it cannot be resolved — sent
// to the campaign's brand fallback. UTM tagging of the final destination lives here too.
//
// The package deliberately has NO store, App, models or i18n dependency: nothing in it can write
// `links`, and every rule is testable without a database (spec I9, review F7). Callers
// (cmd/public.go, internal/manager, cmd/campaigns.go) are thin.
package linkresolve

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"
)

// DummyUUID is listmonk's placeholder UUID for previews and archive pages. A dynamic link
// carrying it can never be resolved (there is no subscriber behind it).
const DummyUUID = "00000000-0000-0000-0000-000000000000"

// Kind classifies why a dynamic link could not be resolved, so callers can log the common
// case (a subscriber simply lacks the attribute) at info and the surprising ones at warn.
type Kind int

const (
	// KindParse — the expression does not parse with the D4 resolver (unknown function,
	// bad syntax). Every click on the link falls back; the Start-time check (D11) refuses
	// such campaigns before they send.
	KindParse Kind = iota + 1
	// KindExec — the expression parsed but failed at execution (a method call on a nil,
	// an index out of range, ...).
	KindExec
	// KindEmpty — rendered to nothing or to text/template's `<no value>`: the missing-attrib
	// case, expected in normal operation.
	KindEmpty
	// KindInvalid — rendered to something that is not an absolute http(s) URL (a relative
	// path, `javascript:`, a bare word, a mailto:). Treated as unresolvable (D3).
	KindInvalid
)

func (k Kind) String() string {
	switch k {
	case KindParse:
		return "parse"
	case KindExec:
		return "exec"
	case KindEmpty:
		return "empty"
	case KindInvalid:
		return "invalid"
	}
	return "unknown"
}

// Error is the typed failure Resolve returns.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Kind.String()
	}
	return e.Kind.String() + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// IsDynamic reports whether a stored link URL is a template expression rather than a
// literal URL — the one-row-per-button rule (D1) registers the unexpanded text.
func IsDynamic(s string) bool {
	return strings.Contains(s, "{{")
}

// Parse compiles a link expression with the D4 resolver: text/template, an EMPTY FuncMap and
// no options. Only `.Subscriber.*`, Go's builtins (`or`, `and`, `printf`, `urlquery`, `index`,
// `print`, `len`, ...) and pipelines of those are available. Campaign context, sprig, `Safe`
// and `TrackLink` are NOT — so recursion into the tracking function is impossible by
// construction, and a stored URL that names any of them fails here (I7).
func Parse(expr string) (*template.Template, error) {
	return template.New("link").Funcs(template.FuncMap{}).Parse(expr)
}

// Resolve renders a dynamic link expression against a subscriber and validates the result.
// sub is exposed to the template as `.Subscriber` and is the ONLY data (D4); pass the
// *models.Subscriber the click belongs to. On success the returned destination is an absolute
// http(s) URL (D3). On failure the *Error's Kind says why, and the caller applies the brand
// fallback (D2).
func Resolve(expr string, sub any) (string, error) {
	tpl, err := Parse(expr)
	if err != nil {
		return "", &Error{Kind: KindParse, Err: err}
	}

	var b bytes.Buffer
	if err := tpl.Execute(&b, map[string]any{"Subscriber": sub}); err != nil {
		return "", &Error{Kind: KindExec, Err: err}
	}

	out := strings.TrimSpace(b.String())
	if out == "" || out == "<no value>" || strings.Contains(out, "<no value>") {
		return "", &Error{Kind: KindEmpty}
	}

	if !IsAbsoluteHTTP(out) {
		return "", &Error{Kind: KindInvalid, Err: fmt.Errorf("not an absolute http(s) URL: %q", out)}
	}

	return out, nil
}

// IsAbsoluteHTTP reports whether s parses as a URL with scheme http or https and a non-empty
// host — the only destinations a redirect may send a subscriber to (D3), and the rule
// `app.link_fallback_url` and the `site:` list tag are validated against on save (I7a).
func IsAbsoluteHTTP(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}

// Brand is the brand-mapping input Fallback derives a destination from. It mirrors the
// fields cmd/campaigns_brand.go's resolveBrandMapping produces for a campaign's target lists,
// carried as plain values so this package needs none of cmd's types.
type Brand struct {
	// Mapped is false when no target list carries brand tags (an unmapped campaign).
	Mapped bool
	// Slug is the `brand:` tag value. The default `curated` slug is treated as unmapped.
	Slug string
	// FromAddress is the BARE address behind the `from:` tag (`hello@thirstygirlhydration.com`),
	// already stripped of any display name by cmd's bareAddress helper — never a raw tag value.
	FromAddress string
	// Site is the `site:` list tag value when present; it wins over the from-domain derivation.
	Site string
	// Err is true when the mapping could not be resolved at all (half-tagged, multi-brand,
	// unknown From, deleted list). Any error → the global fallback setting (D2).
	Err bool
}

// DefaultBrandSlug is cmd's default slug for an unmapped campaign; it maps to the global
// fallback, not to a brand site.
const DefaultBrandSlug = "curated"

// Fallback derives the destination an unresolvable dynamic link redirects to (D2):
// the `site:` tag when present, else `https://<domain of the from: address>`, else — for an
// unmapped campaign, the default brand, or any mapping error — the `app.link_fallback_url`
// setting. Returns "" when nothing applies (the setting is blank), which the caller renders
// as the public error page rather than a redirect.
func Fallback(b Brand, setting string) string {
	setting = strings.TrimSpace(setting)
	if !IsAbsoluteHTTP(setting) {
		setting = ""
	}

	if b.Err || !b.Mapped || b.Slug == "" || b.Slug == DefaultBrandSlug {
		return setting
	}

	if site := strings.TrimSpace(b.Site); IsAbsoluteHTTP(site) {
		return site
	}

	if d := addressDomain(b.FromAddress); d != "" {
		return "https://" + d
	}

	return setting
}

// addressDomain returns the lower-cased domain of a bare `local@domain` address, or "". A
// `Display Name <local@domain>` value is tolerated defensively (the bracketed part is used),
// but callers pass bare addresses -- cmd's bareAddress is the From parser, not this.
func addressDomain(addr string) string {
	addr = strings.TrimSpace(addr)
	if lt := strings.LastIndex(addr, "<"); lt >= 0 {
		if gt := strings.Index(addr[lt:], ">"); gt > 0 {
			addr = addr[lt+1 : lt+gt]
		}
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(addr[at+1:]))
}

// HostUnion builds the D7 storefront-host union: the domain of every configured sending
// address, the host of every `site:` list tag, and the `app.utm_hosts` extras. Lower-cased,
// de-duplicated, sorted; empty and unparsable entries are dropped.
func HostUnion(fromAddresses, siteURLs, extras []string) []string {
	seen := map[string]struct{}{}
	add := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimSuffix(h, ".")
		if h == "" {
			return
		}
		seen[h] = struct{}{}
	}

	for _, a := range fromAddresses {
		add(addressDomain(a))
	}
	for _, s := range siteURLs {
		if u, err := url.Parse(strings.TrimSpace(s)); err == nil {
			add(u.Hostname())
		}
	}
	for _, e := range extras {
		e = strings.TrimSpace(e)
		// Accept a bare host or a URL.
		if strings.Contains(e, "://") {
			if u, err := url.Parse(e); err == nil {
				add(u.Hostname())
			}
			continue
		}
		add(e)
	}

	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// HostMatches reports whether host is one of the union's hosts or a subdomain of one
// (`shop.curatedfor.you` matches `curatedfor.you`; `notcuratedfor.you` does not).
func HostMatches(host string, hosts []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// utmOrder is the emission order for the D8 parameters; any other configured key follows,
// sorted, so the query string is deterministic.
var utmOrder = []string{"utm_source", "utm_medium", "utm_campaign", "utm_content"}

// ApplyUTM appends the UTM parameters to dest when its host is in the storefront-host union
// and dest carries no `utm_*` parameter already (D7). Otherwise dest is returned unchanged.
// Values are URL-encoded; the fragment is preserved. Applies equally to static, dynamic and
// fallback destinations — the caller decides whether UTM is enabled at all.
func ApplyUTM(dest string, hosts []string, params map[string]string) string {
	if len(params) == 0 || len(hosts) == 0 {
		return dest
	}

	u, err := url.Parse(dest)
	if err != nil || !IsAbsoluteHTTP(dest) {
		return dest
	}
	if !HostMatches(u.Hostname(), hosts) {
		return dest
	}

	q := u.Query()
	for k := range q {
		if strings.HasPrefix(strings.ToLower(k), "utm_") {
			return dest
		}
	}

	// Deterministic order: the four standard keys first, then the rest sorted.
	keys := make([]string, 0, len(params))
	for _, k := range utmOrder {
		if _, ok := params[k]; ok {
			keys = append(keys, k)
		}
	}
	var rest []string
	for k := range params {
		if !containsString(utmOrder, k) {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	var sb strings.Builder
	sb.WriteString(u.RawQuery)
	for _, k := range keys {
		v := params[k]
		if strings.TrimSpace(k) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(k))
		sb.WriteByte('=')
		sb.WriteString(url.QueryEscape(v))
	}
	u.RawQuery = sb.String()

	return u.String()
}

// CampaignInfo is the campaign context the UTM parameter templates may reference.
type CampaignInfo struct {
	ID   int
	Name string
}

// RenderUTMParams renders the `app.utm_params` templates (`{{ .Campaign.Name }}`,
// `{{ .Campaign.ID }}` available) into concrete values for one campaign. A template that
// fails to parse or execute is dropped (logged by the caller if it cares) rather than
// emitted raw; a key whose value renders empty is kept empty.
func RenderUTMParams(tpls map[string]string, c CampaignInfo) map[string]string {
	out := make(map[string]string, len(tpls))
	for k, v := range tpls {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !strings.Contains(v, "{{") {
			out[k] = v
			continue
		}
		t, err := template.New("utm").Funcs(template.FuncMap{}).Parse(v)
		if err != nil {
			continue
		}
		var b bytes.Buffer
		if err := t.Execute(&b, map[string]any{"Campaign": c}); err != nil {
			continue
		}
		out[k] = b.String()
	}
	return out
}

// ValidateUTMParams checks that every `app.utm_params` template parses with the resolver
// (so a typo cannot silently drop a parameter at click time). Returns the offending key.
func ValidateUTMParams(tpls map[string]string) (string, error) {
	for k, v := range tpls {
		if strings.TrimSpace(k) == "" {
			return k, errors.New("empty parameter name")
		}
		if _, err := template.New("utm").Funcs(template.FuncMap{}).Parse(v); err != nil {
			return k, err
		}
	}
	return "", nil
}

// Check is the Start-time analysis of one tracked-link expression (D11): whether it parses
// with the resolver, every field reference not rooted at `.Subscriber` (a warning — it renders
// empty at click time), and every `.Subscriber.Attribs.<key>` it reads (for the coverage
// warning). Both `.Subscriber.Attribs.key` and `index .Subscriber.Attribs "key"` are seen.
func Check(expr string) (others []string, attribKeys []string, err error) {
	tpl, err := Parse(expr)
	if err != nil {
		return nil, nil, err
	}
	if tpl.Tree == nil || tpl.Tree.Root == nil {
		return nil, nil, nil
	}

	seenOther := map[string]struct{}{}
	seenKey := map[string]struct{}{}
	var walk func(n parse.Node)
	walk = func(n parse.Node) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case *parse.ListNode:
			if v == nil {
				return
			}
			for _, c := range v.Nodes {
				walk(c)
			}
		case *parse.ActionNode:
			walk(v.Pipe)
		case *parse.PipeNode:
			if v == nil {
				return
			}
			for _, c := range v.Cmds {
				walk(c)
			}
		case *parse.CommandNode:
			// index .Subscriber.Attribs "key"
			if len(v.Args) >= 3 {
				if id, ok := v.Args[0].(*parse.IdentifierNode); ok && id.Ident == "index" {
					if f, ok := v.Args[1].(*parse.FieldNode); ok && len(f.Ident) == 2 &&
						f.Ident[0] == "Subscriber" && f.Ident[1] == "Attribs" {
						if s, ok := v.Args[2].(*parse.StringNode); ok {
							seenKey[s.Text] = struct{}{}
						}
					}
				}
			}
			for _, c := range v.Args {
				walk(c)
			}
		case *parse.FieldNode:
			classify(v.Ident, seenOther, seenKey)
		case *parse.ChainNode:
			// ($x).Field or (pipeline).Field — the chained idents are relative to an
			// arbitrary value; only the root node's own references are classified.
			walk(v.Node)
		case *parse.VariableNode:
			// $.Subscriber.Attribs.x — $ is the root data.
			if len(v.Ident) > 1 && v.Ident[0] == "$" {
				classify(v.Ident[1:], seenOther, seenKey)
			}
		case *parse.IfNode:
			walk(v.Pipe)
			walk(v.List)
			walk(v.ElseList)
		case *parse.RangeNode:
			walk(v.Pipe)
			walk(v.List)
			walk(v.ElseList)
		case *parse.WithNode:
			walk(v.Pipe)
			walk(v.List)
			walk(v.ElseList)
		case *parse.TemplateNode:
			walk(v.Pipe)
		}
	}
	walk(tpl.Tree.Root)

	for k := range seenOther {
		others = append(others, k)
	}
	for k := range seenKey {
		attribKeys = append(attribKeys, k)
	}
	sort.Strings(others)
	sort.Strings(attribKeys)
	return others, attribKeys, nil
}

func classify(ident []string, others, keys map[string]struct{}) {
	if len(ident) == 0 {
		return
	}
	if ident[0] != "Subscriber" {
		others["."+strings.Join(ident, ".")] = struct{}{}
		return
	}
	if len(ident) >= 3 && ident[1] == "Attribs" {
		keys[ident[2]] = struct{}{}
	}
}

func containsString(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
