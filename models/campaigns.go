package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
	txttpl "text/template"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

const (
	CampaignStatusDraft         = "draft"
	CampaignStatusScheduled     = "scheduled"
	CampaignStatusRunning       = "running"
	CampaignStatusPaused        = "paused"
	CampaignStatusFinished      = "finished"
	CampaignStatusCancelled     = "cancelled"
	CampaignTypeRegular         = "regular"
	CampaignTypeOptin           = "optin"
	CampaignContentTypeRichtext = "richtext"
	CampaignContentTypeHTML     = "html"
	CampaignContentTypeMarkdown = "markdown"
	CampaignContentTypePlain    = "plain"
	CampaignContentTypeVisual   = "visual"
)

// Campaigns represents a slice of Campaigns.
type Campaigns []Campaign

// Campaign represents an e-mail campaign.
type Campaign struct {
	Base
	CampaignMeta

	UUID              string          `db:"uuid" json:"uuid"`
	Type              string          `db:"type" json:"type"`
	Name              string          `db:"name" json:"name"`
	Subject           string          `db:"subject" json:"subject"`
	FromEmail         string          `db:"from_email" json:"from_email"`
	Body              string          `db:"body" json:"body"`
	BodySource        null.String     `db:"body_source" json:"body_source"`
	AltBody           null.String     `db:"altbody" json:"altbody"`
	SendAt            null.Time       `db:"send_at" json:"send_at"`
	Status            string          `db:"status" json:"status"`
	ContentType       string          `db:"content_type" json:"content_type"`
	Tags              pq.StringArray  `db:"tags" json:"tags"`
	Headers           Headers         `db:"headers" json:"headers"`
	Attribs           JSON            `db:"attribs" json:"attribs"`
	TemplateID        null.Int        `db:"template_id" json:"template_id"`
	Messenger         string          `db:"messenger" json:"messenger"`
	Archive           bool            `db:"archive" json:"archive"`
	ArchiveSlug       null.String     `db:"archive_slug" json:"archive_slug"`
	ArchiveTemplateID null.Int        `db:"archive_template_id" json:"archive_template_id"`
	ArchiveMeta       json.RawMessage `db:"archive_meta" json:"archive_meta"`

	// FrozenTemplateBody (fork, erinos template freeze) is the resolved template body
	// snapshotted onto the row on the campaign's first transition to 'running'; NULL
	// until then. The fetch queries COALESCE it ahead of the live template body, so a
	// started campaign renders what was approved even if the shared template is later
	// edited. Never client-writable (json:"-"; update-campaign does not set it).
	FrozenTemplateBody null.String `db:"frozen_template_body" json:"-"`

	// Fork (evergreen) -- once started, an evergreen campaign never finishes and keeps
	// sending to subscribers who join its target list after started_at, SendDelaySecs
	// after they join. The three reserved fields are read-only null in this milestone.
	Evergreen        bool        `db:"evergreen" json:"evergreen"`
	SendDelaySecs    int64       `db:"send_delay_secs" json:"send_delay_secs"`
	ParentCampaignID null.Int    `db:"parent_campaign_id" json:"parent_campaign_id"`
	VariantGroupID   null.String `db:"variant_group_id" json:"variant_group_id"`
	VariantIndex     null.Int    `db:"variant_index" json:"variant_index"`
	// Prepared is set by the manager once inline images, the template and media have
	// been resolved on this instance (evergreen re-pipe cache). Never persisted.
	Prepared bool `db:"-" json:"-"`

	// TemplateBody is joined in from templates by the next-campaigns query.
	TemplateBody        string             `db:"template_body" json:"-"`
	ArchiveTemplateBody string             `db:"archive_template_body" json:"-"`
	Tpl                 *template.Template `json:"-"`
	SubjectTpl          *txttpl.Template   `json:"-"`
	PreheaderTpl        *txttpl.Template   `json:"-"`
	AltBodyTpl          *template.Template `json:"-"`

	// HeaderTpls is holds optionally {{ templated }} campaign headers.
	HeaderTpls []map[string]*txttpl.Template `json:"-"`

	// List of media (attachment) IDs obtained from the next-campaign query
	// while sending a campaign.
	MediaIDs pq.Int64Array `json:"-" db:"media_id"`

	// Fetched bodies of the attachments.
	Attachments []Attachment `json:"-" db:"-"`

	// Pseudofield for getting the total number of subscribers
	// in searches and queries.
	Total int `db:"total" json:"-"`

	// Warnings are ephemeral send-quality notices (Gmail clip size, embedded-image
	// lint, missing preheader) computed fresh in the save/status/test handlers and
	// embedded in the response campaign object. Never persisted — deliberately NOT
	// stored in Attribs, which is client-owned round-tripped data; a client that
	// round-trips warnings back is harmlessly ignored.
	Warnings []string `db:"-" json:"warnings,omitempty"`
}

// CampaignMeta contains fields tracking a campaign's progress.
type CampaignMeta struct {
	CampaignID int `db:"campaign_id" json:"-"`
	Views      int `db:"views" json:"views"`
	Clicks     int `db:"clicks" json:"clicks"`
	Bounces    int `db:"bounces" json:"bounces"`

	// This is a list of {list_id, name} pairs unlike Subscriber.Lists[]
	// because lists can be deleted after a campaign is finished, resulting
	// in null lists data to be returned. For that reason, campaign_lists maintains
	// campaign-list associations with a historical record of id + name that persist
	// even after a list is deleted.
	Lists types.JSONText `db:"lists" json:"lists"`
	Media types.JSONText `db:"media" json:"media"`

	StartedAt null.Time `db:"started_at" json:"started_at"`
	ToSend    int       `db:"to_send" json:"to_send"`
	Sent      int       `db:"sent" json:"sent"`
}

// SendFailure (fork, SEND-RETRY-SPEC D5) is one recipient a campaign could not reach: a
// message that exhausted the SMTP pool's attempts (Stage "send") or that never reached
// the queue because its template failed to render (Stage "render"). Rows land in
// campaign_send_failures and are the recovery list behind a sent < to_send shortfall;
// nothing re-sends them automatically.
type SendFailure struct {
	CampaignID   int    `db:"campaign_id" json:"campaign_id"`
	SubscriberID int    `db:"subscriber_id" json:"subscriber_id"`
	Email        string `db:"email" json:"email"`
	Stage        string `db:"stage" json:"stage"`
	Error        string `db:"error" json:"error"`
}

const (
	SendFailureStageSend   = "send"
	SendFailureStageRender = "render"
	// SendFailureStageSendUnconfirmed is a send whose error surfaced AFTER the message data
	// was written: the server may have accepted it. Re-sending such a recipient risks a
	// duplicate; the manager treats it as attempted (an evergreen claim is consumed).
	SendFailureStageSendUnconfirmed = "send-unconfirmed"
)

// ErrMessageMaybeDelivered is wrapped into a messenger's Push error when the failure came
// after the message body was handed to the server (fork, SEND-RETRY-SPEC D10 / review F1).
// The email messenger translates smtppool's sentinel into this one so the manager stays
// messenger-agnostic.
var ErrMessageMaybeDelivered = errors.New("message may have been delivered")

// GetIDs returns the list of campaign IDs.
func (camps Campaigns) GetIDs() []int {
	IDs := make([]int, len(camps))
	for i, c := range camps {
		IDs[i] = c.ID
	}

	return IDs
}

// LoadStats lazy loads campaign stats onto a list of campaigns.
func (camps Campaigns) LoadStats(stmt *sqlx.Stmt) error {
	var meta []CampaignMeta
	if err := stmt.Select(&meta, pq.Array(camps.GetIDs())); err != nil {
		return err
	}

	if len(camps) != len(meta) {
		return errors.New("campaign stats count does not match")
	}

	for i, c := range meta {
		if c.CampaignID == camps[i].ID {
			camps[i].Lists = c.Lists
			camps[i].Views = c.Views
			camps[i].Clicks = c.Clicks
			camps[i].Bounces = c.Bounces
			camps[i].Media = c.Media
		}
	}

	return nil
}

// CompileTemplate compiles a campaign body template into its base
// template and sets the resultant template to Campaign.Tpl.
func (c *Campaign) CompileTemplate(f template.FuncMap) error {
	// If the subject line has a template string, compile it.
	if hasTplExpr(c.Subject) {
		subj := c.Subject
		for _, r := range regTplFuncs {
			subj = r.regExp.ReplaceAllString(subj, r.replace)
		}

		var txtFuncs map[string]any = f
		subjTpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(subj)
		if err != nil {
			return fmt.Errorf("error compiling subject: %v", err)
		}
		c.SubjectTpl = subjTpl
	}

	// If the preheader has a template string, compile it like the subject.
	if p := c.Preheader(); hasTplExpr(p) {
		for _, r := range regTplFuncs {
			p = r.regExp.ReplaceAllString(p, r.replace)
		}

		var txtFuncs map[string]any = f
		phTpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(p)
		if err != nil {
			return fmt.Errorf("error compiling preheader: %v", err)
		}
		c.PreheaderTpl = phTpl
	}

	// Compile the base template.
	body := c.TemplateBody

	if body == "" || c.ContentType == CampaignContentTypeVisual {
		body = `{{ template "content" . }}`

		// Fork (visual tracking) -- the visual builder emits no {{ TrackView }}
		// (and template_id is NULL by construction), so append the open pixel to
		// the base. The live-tag guard keeps a document that already carries the
		// marker (upstream's default visual footer) from double-pixelling; the
		// bare string "TrackView" in prose or a URL must not suppress it.
		if c.ContentType == CampaignContentTypeVisual && !regLiveTrackView.MatchString(c.Body) {
			body += `{{ TrackView }}`
		}
	}

	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	baseTPL, err := template.New(BaseTpl).Funcs(f).Parse(body)
	if err != nil {
		return fmt.Errorf("error compiling base template: %v", err)
	}

	// If the format is markdown, convert Markdown to HTML.
	if c.ContentType == CampaignContentTypeMarkdown {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(c.Body), &b); err != nil {
			return err
		}
		body = b.String()
	} else {
		body = c.Body
	}

	// Fork (visual tracking) -- the visual builder has no tracking affordance and
	// emits plain double-quoted hrefs; wrap them in {{ TrackLink }} at compile so
	// clicks register. Local variable only: stored fields must stay unmutated (the
	// evergreen prepared-cache hash reads them).
	body = TransformTrackLinks(body, c.ContentType)

	// Compile the campaign message.
	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	msgTpl, err := template.New(ContentTpl).Funcs(f).Parse(body)
	if err != nil {
		return fmt.Errorf("error compiling message: %v", err)
	}

	out, err := baseTPL.AddParseTree(ContentTpl, msgTpl.Tree)
	if err != nil {
		return fmt.Errorf("error inserting child template: %v", err)
	}
	c.Tpl = out

	if hasTplExpr(c.AltBody.String) {
		b := c.AltBody.String
		for _, r := range regTplFuncs {
			b = r.regExp.ReplaceAllString(b, r.replace)
		}
		bTpl, err := template.New(ContentTpl).Funcs(f).Parse(b)
		if err != nil {
			return fmt.Errorf("error compiling alt plaintext message: %v", err)
		}
		c.AltBodyTpl = bTpl
	}

	// Compile any header values that contain template expressions.
	for _, set := range c.Headers {
		for _, val := range set {
			if hasTplExpr(val) {
				c.HeaderTpls = make([]map[string]*txttpl.Template, len(c.Headers))
				break
			}
		}
		if c.HeaderTpls != nil {
			break
		}
	}
	if c.HeaderTpls != nil {
		var txtFuncs map[string]any = f
		for i, set := range c.Headers {
			c.HeaderTpls[i] = make(map[string]*txttpl.Template, len(set))
			for hdr, val := range set {
				if !hasTplExpr(val) {
					continue
				}
				tpl, err := txttpl.New(ContentTpl).Funcs(txtFuncs).Parse(val)
				if err != nil {
					return fmt.Errorf("error compiling header %q: %v", hdr, err)
				}
				c.HeaderTpls[i][hdr] = tpl
			}
		}
	}

	return nil
}

// Fork (visual tracking + click tracking) -- regexps for CompileTemplate's tracked-link
// transform (CLICK-TRACKING-SPEC §3.1).
var (
	// A live {{ TrackView }} tag (any spacing) already in the document body. Deliberately
	// narrower than a bare strings.Contains: the word "TrackView" in prose, a URL fragment
	// or a Safe payload must not suppress the base pixel.
	regLiveTrackView = regexp.MustCompile(`{{\s*TrackView`)

	// A plain double-quoted absolute href, the only form the builder's
	// renderToStaticMarkup emits. Safe payloads escape their quotes (href=\"...\"), so
	// nothing inside one can match; single-quoted or spaced hrefs miss (not corrupt).
	regVisualHref = regexp.MustCompile(`href="(https?://[^"]*)"`)

	// A double-quoted href whose value carries a template expression — a personalized
	// button URL (`{{ .Subscriber.Attribs.site }}`, or the `or` idiom with a literal
	// fallback). Same quoting rule as regVisualHref, so Safe payloads never match.
	regDynamicHref = regexp.MustCompile(`href="([^"]*{{[^"]*)"`)

	// The builder's VML href marker: outlook.ts emits the mso <v:roundrect> href value
	// OUTSIDE the Safe string literal as an empty span carrying it in an attribute (the one
	// shape Editor.vue's beautifier never line-wraps — spec §3.3). Replaced here by a
	// TrackLink call; the Safe payloads on either side supply the surrounding href="…".
	// Whitespace around and inside the span is consumed too: Editor.vue's tag-padding regexp
	// exempts <span but NOT </span>, so a format switch leaves a newline + indent after the
	// marker (verified 2026-09-05 against js-beautify with Editor.vue's settings) which would
	// otherwise render INSIDE the VML href value.
	regVMLHrefMarker = regexp.MustCompile(`\s*<span\s+data-lm-vml-href="([^"]*)"\s*>\s*</span>\s*`)

	// Template functions that own their own semantics and must never be wrapped into a
	// TrackLink string literal: a TrackLink (nesting), and listmonk's URL functions, which the
	// D4 resolver (empty FuncMap) cannot evaluate — wrapping `{{ UnsubscribeURL }}` would
	// turn every unsubscribe link in an Html block into a fallback redirect.
	regReservedTplFunc = regexp.MustCompile(`{{\s*(TrackLink|TrackView|UnsubscribeURL|ManageURL|OptinURL|MessageURL)\b`)
)

// TransformTrackLinks applies the compile-time tracked-link rewrites to a campaign body and
// returns the result. LOCAL VARIABLE ONLY at the call site: stored fields stay unmutated (the
// evergreen prepared-cache hash reads them). Exported so the Start-time expression check
// (cmd/campaigns.go, spec §3.5) can run the identical transform on the SOURCE body and read
// the TrackLink arguments before they become /link/ URLs.
//
//   - visual: static absolute hrefs → {{ TrackLink "<url>" . }} (erinos.62, unchanged);
//     dynamic hrefs (value contains `{{`) → {{ TrackLink "<unexpanded text>" . }} so the
//     expression registers ONCE as its own text and resolves per click (D1).
//   - every non-plain type: the builder's <span data-lm-vml-href> marker → a TrackLink call
//     (static value tracked, dynamic value unexpanded). Not only visual, because Editor.vue's
//     visual→HTML format switch keeps the compiled body, marker included, under
//     content_type=html (review F4).
//   - plain: untouched.
func TransformTrackLinks(body, contentType string) string {
	if contentType == CampaignContentTypePlain {
		return body
	}
	// The marker pass runs FIRST: `data-lm-vml-href="…"` ends in `href="…"`, so the two href
	// regexps below would otherwise match inside the marker's attribute and wrap its value
	// in place, leaving a literal <span> in the VML href.
	body = rewriteVMLHrefMarkers(body)
	if contentType == CampaignContentTypeVisual {
		body = rewriteVisualTrackLinks(body)
		body = rewriteDynamicHrefs(body)
	}
	return body
}

// rewriteVisualTrackLinks wraps plain absolute hrefs in a visual campaign body with
// full {{ TrackLink "<url>" . }} calls (not the @TrackLink shorthand, whose narrower
// RFC-3986 character class would truncate unusual URLs). Skipped (left plain, never
// corrupted): URLs carrying the @TrackLink token anywhere (suffix is the shorthand,
// left to the existing regTplFuncs pass; mid-URL would let that pass match inside the
// emitted quoted string and nest actions), template expressions, and URLs containing
// a character illegal inside a Go template string literal — backslash, or any control
// character ([^"]* matches newlines, and a raw LF makes the whole template
// uncompilable, which for a stored body means a dead campaign, not a bad link).
func rewriteVisualTrackLinks(body string) string {
	return regVisualHref.ReplaceAllStringFunc(body, func(match string) string {
		url := match[len(`href="`) : len(match)-1]
		if strings.Contains(url, "@TrackLink") || strings.Contains(url, "{{") ||
			strings.Contains(url, `\`) ||
			strings.IndexFunc(url, func(r rune) bool { return r < 0x20 }) >= 0 {
			return match
		}
		return `href="{{ TrackLink "` + url + `" . }}"`
	})
}

// rewriteDynamicHrefs wraps a visual body's dynamic hrefs (spec §3.1, first bullet). The
// value is HTML-entity-decoded first — React entity-escapes attribute values in Button.tsx,
// so a typed `{{ or .Subscriber.Attribs.x "https://…" }}` is stored with &quot; and would
// otherwise parse as `unexpected "&" in operand` (review F2) — then quotes are escaped and
// the text is emitted INSIDE the TrackLink string literal, unexpanded, so TrackLink registers
// the expression itself, once (D1). Returns the match untouched when trackLinkArg refuses it.
func rewriteDynamicHrefs(body string) string {
	return regDynamicHref.ReplaceAllStringFunc(body, func(match string) string {
		raw := match[len(`href="`) : len(match)-1]
		arg, ok := trackLinkArg(raw, true)
		if !ok {
			return match
		}
		return `href="{{ TrackLink "` + arg + `" . }}"`
	})
}

// rewriteVMLHrefMarkers replaces every builder VML href marker with a TrackLink call whose
// argument is the marker's decoded value (static → literal URL, tracked; dynamic → the
// unexpanded expression). A value the skip rules refuse, or a static value that is not an
// absolute http(s) URL (`#`, mailto:), is emitted as its decoded text — never left as a
// literal <span> inside the VML href (I2).
func rewriteVMLHrefMarkers(body string) string {
	return regVMLHrefMarker.ReplaceAllStringFunc(body, func(match string) string {
		m := regVMLHrefMarker.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		decoded := html.UnescapeString(m[1])
		arg, ok := trackLinkArg(m[1], strings.Contains(decoded, "{{"))
		if !ok {
			return decoded
		}
		if !strings.Contains(decoded, "{{") && !regVisualHref.MatchString(`href="`+decoded+`"`) {
			return decoded
		}
		return `{{ TrackLink "` + arg + `" . }}`
	})
}

// trackLinkArg turns a raw href/marker attribute value into the string-literal body of a
// TrackLink call, or reports (ok=false) that the value must be left alone. dynamic says
// whether the value is expected to carry an expression. Entity-decode first (&quot; &amp;
// &lt; &gt; &#39; and the rest of the HTML entity table), then refuse anything that cannot
// live in a Go template string literal or would nest another template function: backslash,
// control characters, the @TrackLink shorthand, a reserved function name, or an unbalanced
// `{{`/`}}` (the attribute regexp stopped at a raw quote inside the expression — wrapping the
// fragment would corrupt it; leaving it plain lets the full FuncMap evaluate it as today).
func trackLinkArg(raw string, dynamic bool) (string, bool) {
	s := html.UnescapeString(raw)
	if strings.Contains(s, "@TrackLink") || strings.Contains(s, `\`) ||
		strings.IndexFunc(s, func(r rune) bool { return r < 0x20 }) >= 0 {
		return "", false
	}
	if dynamic {
		if regReservedTplFunc.MatchString(s) {
			return "", false
		}
		if strings.Count(s, "{{") != strings.Count(s, "}}") {
			return "", false
		}
	}
	return strings.ReplaceAll(s, `"`, `\"`), true
}

// Preheader returns the campaign's preheader (inbox preview) text. It is stored under the
// "preheader" key in the attribs JSON rather than a dedicated column, so the fork carries
// no schema change; attribs already round-trips through the API and every campaign query.
func (c *Campaign) Preheader() string {
	s, _ := c.Attribs["preheader"].(string)
	return strings.TrimSpace(s)
}

// CampaignLangs (fork, erinos multi-language campaigns) is the closed set a campaign's
// attribs.lang may take. Absent = the campaign targets everyone (the pre-fork behaviour).
// The send-time predicates read the value from the campaign row in SQL, so this set must
// agree with what those queries accept — they compare strings, so any value here works.
var CampaignLangs = []string{"en", "es", "fr", "de", "it"}

// Lang returns the campaign's language code from attribs.lang, or "" for everyone.
func (c *Campaign) Lang() string {
	s, _ := c.Attribs["lang"].(string)
	return s
}

// NormalizeLang validates attribs.lang in place. An absent or empty value removes the key
// (the form's "All" option posts ""); anything else must be one of CampaignLangs, exactly
// (lowercase), or ok is false. Attribs may be nil.
func NormalizeLang(attribs JSON) (ok bool) {
	if attribs == nil {
		return true
	}
	v, present := attribs["lang"]
	if !present {
		return true
	}
	s, isStr := v.(string)
	if !isStr {
		return false
	}
	if s == "" {
		delete(attribs, "lang")
		return true
	}
	for _, l := range CampaignLangs {
		if s == l {
			return true
		}
	}
	return false
}

// hasTplExpr checks whether a given string has a Go template expression with {{ and  }}.
func hasTplExpr(s string) bool {
	_, after, ok := strings.Cut(s, "{{")
	return ok && strings.Contains(after, "}}")
}

// ConvertContent converts a campaign's body from one format to another,
// for example, Markdown to HTML.
func (c *Campaign) ConvertContent(from, to string) (string, error) {
	body := c.Body
	for _, r := range regTplFuncs {
		body = r.regExp.ReplaceAllString(body, r.replace)
	}

	// If the format is markdown, convert Markdown to HTML.
	var out string
	if from == CampaignContentTypeMarkdown &&
		(to == CampaignContentTypeHTML || to == CampaignContentTypeRichtext) {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(c.Body), &b); err != nil {
			return out, err
		}
		out = b.String()
	} else {
		return out, errors.New("unknown formats to convert")
	}

	return out, nil
}
