package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
