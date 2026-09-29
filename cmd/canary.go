package main

// Fork (render canary, integrations INSPECT-SCOPE-SPEC §2.1) -- the server half.
//
// The builder half (frontend/email-builder/test/build-canary.cjs, run in build-image.yml after
// build-frontend and before pack-bin) packs render-canary.json beside the served bundle: every
// canary corpus document, its compiled HTML, the raw non-visual canary body and the builder's
// per-type hashes. The send path transforms compiled HTML further (TrackView, TransformTrackLinks,
// rewriteDynamicHrefs -- models/campaigns.go), so at startup, once the manager exists, every
// document's HTML (and the body) is rendered here through the campaign PREVIEW path
// (CompileTemplate(manager.TemplateFuncs(&camp)) + NewCampaignMessage) with a FIXED synthetic
// campaign and subscriber UUID and no href normalisation, and hashed per key with the builder's
// grammar. A key's hash therefore moves exactly when the compiled or send-rendered output of a
// document containing that block type moves; an editor-only release moves none, so structure
// records survive it (S1).
//
// STARTUP WRITES AND RUNTIME STATE. The canary's links go through TrackLink like any preview's,
// so EVERY boot and every settings-save reload upserts them into the links table (create-link is
// an upsert on url: the `canary.invalid` hrefs and the fixed Official_ refs' links, each row
// written once and its UUID stable after). The server hashes therefore depend on those link rows
// persisting, and on runtime settings the send path reads -- the root URL (tracked-link, pixel,
// unsubscribe and message URLs) and privacy.individual_tracking (the subscriber UUID in tracked
// URLs): changing either moves every key and voids every structure record. That is conservative
// (a record never over-vouches), and integrations runbook hazard 93 records it. A panic inside a
// template function surfaces as a render error (text/template recovers it), so a bad canary is a
// 503 and a fail-closed review, never a blocked boot.
//
// GET /api/campaigns/render-canary (campaigns:review) returns the startup result; a missing or
// unparseable file, or a render error, is a 503 -- the review then fails closed (S11).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
)

const (
	renderCanaryPath = "/admin/static/email-builder/render-canary.json"
	canaryCampUUID   = "00000000-0000-4000-8000-00000000ca11"
	canarySubUUID    = "00000000-0000-4000-8000-000000000501"
	// canaryBodyKey is the key grammar's non-visual entry (canary/body.html).
	canaryBodyKey = "Body"
)

var canarySubscriber = models.Subscriber{
	UUID:    canarySubUUID,
	Email:   "canary@canary.invalid",
	Name:    "Canary Subscriber",
	Attribs: models.JSON{},
}

// renderCanaryFile is the builder half's file.
type renderCanaryFile struct {
	Version   int `json:"version"`
	Documents map[string]struct {
		Doc  json.RawMessage `json:"doc"`
		HTML string          `json:"html"`
	} `json:"documents"`
	Body  string            `json:"body"`
	Items map[string]string `json:"items"`
}

// renderCanary is what the endpoint serves.
type renderCanary struct {
	Version      int                        `json:"version"`
	Items        map[string]string          `json:"items"`
	BuilderItems map[string]string          `json:"builderItems"`
	Documents    map[string]json.RawMessage `json:"documents"`
}

// canaryRenderFunc renders one body of the given content type through the send path.
type canaryRenderFunc func(body, contentType string) (string, error)

// blockTypes is every `type` a builder document carries (the root EmailLayout included).
func blockTypes(doc json.RawMessage) (map[string]bool, error) {
	var blocks map[string]struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(doc, &blocks); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, b := range blocks {
		if b.Type != "" {
			out[b.Type] = true
		}
	}
	return out, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// canaryItems applies the key grammar: items[T] = sha256 over the rendered HTML of every document
// containing T, in file-stem order, each preceded by "\n--<stem>--\n"; Body = sha256(rendered body).
func canaryItems(types map[string]map[string]bool, html map[string]string, body string) map[string]string {
	stems := make([]string, 0, len(html))
	for s := range html {
		stems = append(stems, s)
	}
	sort.Strings(stems)
	all := map[string]bool{}
	for _, s := range stems {
		for t := range types[s] {
			all[t] = true
		}
	}
	out := map[string]string{}
	for t := range all {
		var b strings.Builder
		for _, s := range stems {
			if types[s][t] {
				b.WriteString("\n--" + s + "--\n")
				b.WriteString(html[s])
			}
		}
		out[t] = sha256Hex(b.String())
	}
	out[canaryBodyKey] = sha256Hex(body)
	return out
}

// buildRenderCanary parses the builder file and renders every document (and the body) with render.
func buildRenderCanary(raw []byte, render canaryRenderFunc) (*renderCanary, error) {
	var f renderCanaryFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("render-canary.json: %v", err)
	}
	if f.Version != 1 || len(f.Documents) == 0 || len(f.Items) == 0 {
		return nil, errors.New("render-canary.json: not a version 1 canary with documents and items")
	}
	types := map[string]map[string]bool{}
	html := map[string]string{}
	docs := map[string]json.RawMessage{}
	for stem, d := range f.Documents {
		t, err := blockTypes(d.Doc)
		if err != nil {
			return nil, fmt.Errorf("render-canary.json: document %s: %v", stem, err)
		}
		out, err := render(d.HTML, models.CampaignContentTypeVisual)
		if err != nil {
			return nil, fmt.Errorf("render canary document %s: %v", stem, err)
		}
		types[stem], html[stem], docs[stem] = t, out, d.Doc
	}
	body, err := render(f.Body, models.CampaignContentTypeHTML)
	if err != nil {
		return nil, fmt.Errorf("render canary body: %v", err)
	}
	return &renderCanary{Version: f.Version, Items: canaryItems(types, html, body), BuilderItems: f.Items, Documents: docs}, nil
}

// canaryRender is the campaign preview path (cmd/campaigns.go PreviewCampaign) over a synthetic
// campaign and subscriber with fixed UUIDs.
func (a *App) canaryRender(body, contentType string) (string, error) {
	camp := models.Campaign{
		UUID:        canaryCampUUID,
		Name:        "render canary",
		Subject:     "render canary",
		FromEmail:   "Canary <canary@canary.invalid>",
		Body:        body,
		ContentType: contentType,
		Messenger:   "email",
	}
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		return "", err
	}
	msg, err := a.manager.NewCampaignMessage(&camp, canarySubscriber)
	if err != nil {
		return "", err
	}
	return string(msg.Body()), nil
}

// initRenderCanary reads the packed file and renders it; an error is logged and served as a 503.
func initRenderCanary(fs stuffbin.FileSystem, render canaryRenderFunc) (*renderCanary, error) {
	raw, err := fs.Read(renderCanaryPath)
	if err != nil {
		return nil, fmt.Errorf("render canary: %s not found: %v", renderCanaryPath, err)
	}
	return buildRenderCanary(raw, render)
}

// GetRenderCanary (GET /api/campaigns/render-canary) -- the startup render canary, or 503.
func (a *App) GetRenderCanary(c echo.Context) error {
	if a.renderCanary == nil {
		msg := "render canary unavailable"
		if a.renderCanaryErr != nil {
			msg += ": " + a.renderCanaryErr.Error()
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, msg)
	}
	return c.JSON(http.StatusOK, okResp{a.renderCanary})
}
