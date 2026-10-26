package models

import (
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	txttpl "text/template"
	"time"

	null "gopkg.in/volatiletech/null.v6"
)

const (
	BaseTpl                    = "base"
	ContentTpl                 = "content"
	TemplateTypeCampaign       = "campaign"
	TemplateTypeCampaignVisual = "campaign_visual"
	TemplateTypeTx             = "tx"
)

// Fork (official footer, integrations OFFICIAL-FOOTER-SPEC D10). A template whose name carries
// this prefix is an OFFICIAL block: the visual builder's OfficialFooter block renders it (resolved
// by name) inside every campaign that carries one. The prefix IS the marker -- the template's
// lang/brand columns drive nothing -- and IsOfficialTemplate is the one rule.
const OfficialTemplatePrefix = "Official_"

// IsOfficialTemplate reports whether a template name marks an official block.
func IsOfficialTemplate(name string) bool {
	return strings.HasPrefix(name, OfficialTemplatePrefix)
}

// reOfficialFooterBlock finds an OfficialFooter block in a builder document (body_source JSON).
var reOfficialFooterBlock = regexp.MustCompile(`"type"\s*:\s*"OfficialFooter"`)

// HasOfficialFooterBlock reports whether a builder document holds an OfficialFooter block -- an
// official template may not (a reference must never recurse).
func HasOfficialFooterBlock(bodySource string) bool {
	return reOfficialFooterBlock.MatchString(bodySource)
}

// Template represents a reusable e-mail template.
type Template struct {
	Base

	Name string `db:"name" json:"name"`
	// Subject is only for type=tx.
	Subject    string      `db:"subject" json:"subject"`
	Type       string      `db:"type" json:"type"`
	Body       string      `db:"body" json:"body,omitempty"`
	BodySource null.String `db:"body_source" json:"body_source,omitempty"`
	IsDefault  bool        `db:"is_default" json:"is_default"`
	// Brand is the Templates form's "Brand swatches" dropdown selection (fork, EDITOR-POLISH-SPEC
	// D4) — editor-only metadata, does not feed campaign brand derivation. '' means no brand.
	Brand string `db:"brand" json:"brand"`

	// Lang (fork, integrations LIST-GRID-SPEC D12) is the language the template's body is
	// written in -- one of CampaignLangs, stored "en" when absent. Importing a visual template
	// into a campaign sets the campaign's language to it (frontend, Editor.vue). The wrapper
	// template_id of a campaign does NOT drive language. Wrappers are chrome.
	Lang string `db:"lang" json:"lang"`

	// Only relevant to tx (transactional) templates.
	SubjectTpl  *txttpl.Template   `json:"-"`
	Tpl         *template.Template `json:"-"`
	Attachments []Attachment       `json:"-"`
}

// Compile compiles a template body and subject (only for tx templates) and
// caches the templat references to be executed later.
func (t *Template) Compile(f template.FuncMap) error {
	tpl, err := template.New(BaseTpl).Funcs(f).Parse(t.Body)
	if err != nil {
		return fmt.Errorf("error compiling transactional template: %v", err)
	}
	t.Tpl = tpl

	// If the subject line has a template string, compile it.
	if hasTplExpr(t.Subject) {
		subj := t.Subject

		subjTpl, err := txttpl.New(BaseTpl).Funcs(txttpl.FuncMap(f)).Parse(subj)
		if err != nil {
			return fmt.Errorf("error compiling subject: %v", err)
		}
		t.SubjectTpl = subjTpl
	}

	return nil
}

type CampaignStats struct {
	ID        int       `db:"id" json:"id"`
	Status    string    `db:"status" json:"status"`
	ToSend    int       `db:"to_send" json:"to_send"`
	Sent      int       `db:"sent" json:"sent"`
	Started   null.Time `db:"started_at" json:"started_at"`
	UpdatedAt null.Time `db:"updated_at" json:"updated_at"`
	Rate      int       `json:"rate"`
	NetRate   int       `json:"net_rate"`
}

type CampaignAnalyticsCount struct {
	CampaignID int       `db:"campaign_id" json:"campaign_id"`
	Count      int       `db:"count" json:"count"`
	Timestamp  time.Time `db:"timestamp" json:"timestamp"`
}

// AnalyticsBucketLayout is the wire form of a CampaignAnalyticsCount bucket: wall-clock digits with
// NO offset (fork, campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D5).
const AnalyticsBucketLayout = "2006-01-02T15:04:05"

// MarshalJSON (fork, CAMPAIGN-SEND-SERIES-SPEC D5) -- the analytics statements bucket in Mountain
// wall-clock (`... AT TIME ZONE 'America/Denver'`, a NAIVE timestamp). The driver hands a naive
// timestamp back as a zero-offset time.Time, which the default encoder would write with a "Z" --
// a browser would then shift it to its own zone. So the bucket is written as its digits alone,
// without an offset, and the page shows it verbatim (Mountain in every browser). Never emit an
// offset here and never convert the column back to timestamptz.
func (c CampaignAnalyticsCount) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"campaign_id":%d,"count":%d,"timestamp":"%s"}`,
		c.CampaignID, c.Count, c.Timestamp.Format(AnalyticsBucketLayout))), nil
}

// UnmarshalJSON is MarshalJSON's inverse (a Go reader of the API, the handler tests): the
// offset-less bucket parses as wall-clock digits (in UTC, carrying no zone meaning); an RFC 3339
// value is still accepted.
func (c *CampaignAnalyticsCount) UnmarshalJSON(b []byte) error {
	var raw struct {
		CampaignID int    `json:"campaign_id"`
		Count      int    `json:"count"`
		Timestamp  string `json:"timestamp"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	ts, err := time.Parse(AnalyticsBucketLayout, raw.Timestamp)
	if err != nil {
		if ts, err = time.Parse(time.RFC3339Nano, raw.Timestamp); err != nil {
			return err
		}
	}
	c.CampaignID, c.Count, c.Timestamp = raw.CampaignID, raw.Count, ts
	return nil
}

type CampaignAnalyticsLink struct {
	URL   string `db:"url" json:"url"`
	Count int    `db:"count" json:"count"`
}

// CampaignAnalyticsShopLink is one row of the shop-link attribution analytics (fork, shop-link
// attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC D3/D4): Kind is the clicking subscriber's
// stored shop-link kind for the link's brand (self, creator, founder, corporate, or any other
// stored string verbatim), "none" for a known subscriber with no entry, "unknown" for a click
// with no subscriber. Kinds and counts only -- no PII.
//
// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D5) -- Audience is the SUM of the selected
// campaigns' send-time snapshot rows for the kind (0 for a kind absent from an existing snapshot),
// nil on every row when no selected campaign has a snapshot and nil for "unknown" always;
// Snapshots is how many of the selected campaigns have a snapshot, repeated on every row.
type CampaignAnalyticsShopLink struct {
	Kind      string `db:"kind" json:"kind"`
	Count     int    `db:"count" json:"count"`
	Audience  *int   `db:"audience" json:"audience"`
	Snapshots int    `db:"snapshots" json:"snapshots"`
}

// CampaignAnalyticsCountry is one row of the per-country analytics (fork, location stats):
// Country is the ISO 3166-1 alpha-2 code, or "" for unknown.
type CampaignAnalyticsCountry struct {
	Country string `db:"country" json:"country"`
	Views   int    `db:"views" json:"views"`
	Clicks  int    `db:"clicks" json:"clicks"`
}

// CampaignAnalyticsClient is one row of the per-email-client analytics (fork, client stats,
// integrations CLIENT-STATS-SPEC D5): Client is a closed-vocabulary token (cmd/public.go
// classifyClient), or "" for unknown. Shared by the campaign analytics and the Dashboard panel.
type CampaignAnalyticsClient struct {
	Client string `db:"client" json:"client"`
	Views  int    `db:"views" json:"views"`
	Clicks int    `db:"clicks" json:"clicks"`
}

// DashboardClients is the Dashboard's client panel (fork, client stats, CLIENT-STATS-SPEC D6): the
// rows, and Scoped -- true when they cover a scope (a list-scoped user's permitted lists, or the
// global brand's list_id, GLOBAL-BRAND-SPEC D5) rather than every campaign.
type DashboardClients struct {
	Scoped  bool                      `json:"scoped"`
	Clients []CampaignAnalyticsClient `json:"clients"`
}

// DashboardCountries is the Dashboard's Location panel (fork, dashboard counts, integrations
// DASHBOARD-COUNTS-SPEC R4): the DashboardClients shape with per-country rows.
type DashboardCountries struct {
	Scoped    bool                       `json:"scoped"`
	Countries []CampaignAnalyticsCountry `json:"countries"`
}

// AnalyticsCampaign is one row of the Campaign Analytics picker (fork, brand analytics,
// BRAND-ANALYTICS-SPEC D3): exactly the fields the picker needs, nothing a creator must not read.
type AnalyticsCampaign struct {
	ID        int       `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Status    string    `db:"status" json:"status"`
	Evergreen bool      `db:"evergreen" json:"evergreen"`
	StartedAt null.Time `db:"started_at" json:"started_at"`
	CreatedAt null.Time `db:"created_at" json:"created_at"`
	// The denominator of the page's rates (integrations CAMPAIGN-RATES-SPEC, analytics follow-up).
	Sent int `db:"sent" json:"sent"`
}

type CampaignViewExport struct {
	CampaignID     int       `db:"campaign_id"`
	CampaignUUID   string    `db:"campaign_uuid"`
	CampaignName   string    `db:"campaign_name"`
	SubscriberID   int       `db:"subscriber_id"`
	SubscriberUUID string    `db:"subscriber_uuid"`
	Email          string    `db:"email"`
	SubscriberName string    `db:"subscriber_name"`
	CreatedAt      time.Time `db:"created_at"`
}

type CampaignClickExport struct {
	CampaignID     int       `db:"campaign_id"`
	CampaignUUID   string    `db:"campaign_uuid"`
	CampaignName   string    `db:"campaign_name"`
	SubscriberID   int       `db:"subscriber_id"`
	SubscriberUUID string    `db:"subscriber_uuid"`
	Email          string    `db:"email"`
	SubscriberName string    `db:"subscriber_name"`
	URL            string    `db:"url"`
	CreatedAt      time.Time `db:"created_at"`
}
