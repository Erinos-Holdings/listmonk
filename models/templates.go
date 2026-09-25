package models

import (
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

type CampaignAnalyticsLink struct {
	URL   string `db:"url" json:"url"`
	Count int    `db:"count" json:"count"`
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
