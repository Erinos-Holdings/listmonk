package main

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/linkresolve"
	"github.com/knadh/listmonk/models"
)

// linkFallbacks (fork, CLICK-TRACKING-SPEC §3.2) is the App-independent glue between the pure
// internal/linkresolve rules and the database: it derives a campaign's brand fallback URL from
// its target lists' tags (the same mapping that derives the From address and the SES brand
// tag — cmd/campaigns_brand.go) and the D7 storefront-host union for UTM tagging, and caches
// both. It is built BEFORE the campaign manager (whose send-time D1 branch needs the fallback)
// and shared with the App (whose LinkRedirect needs everything).
//
// Caching: tags, lists and campaign names are editable after a send, so every entry here is a
// cache with bounded staleness — the spec accepts restart-bounded; this refreshes after
// cacheTTL as well, which is strictly fresher and lets a `site:` tag or a new brand's list take
// effect without a restart.
type linkFallbacks struct {
	core *core.Core
	i18n *i18n.I18n
	ko   *koanf.Koanf
	log  *log.Logger

	mu     sync.Mutex
	byID   map[int]cachedFallback
	byUUID map[string]cachedCampaign
	hosts  []string
	hostAt time.Time
}

type cachedFallback struct {
	url string
	// Fork (naming standard, NAMING-STANDARD-SPEC §4.5) -- the campaign's resolved brand slug,
	// cached beside the URL, trusted only when mapped (the list mapping resolved). A failed or
	// unmapped read is never trusted for the slug: utmSlug re-resolves on the next click.
	slug   string
	mapped bool
	at     time.Time
}

type cachedCampaign struct {
	camp models.Campaign
	at   time.Time
}

const linkCacheTTL = 5 * time.Minute

func newLinkFallbacks(co *core.Core, i *i18n.I18n, ko *koanf.Koanf, lo *log.Logger) *linkFallbacks {
	return &linkFallbacks{
		core:   co,
		i18n:   i,
		ko:     ko,
		log:    lo,
		byID:   map[int]cachedFallback{},
		byUUID: map[string]cachedCampaign{},
	}
}

// setting is the live app.link_fallback_url value.
func (l *linkFallbacks) setting() string {
	return l.ko.String("app.link_fallback_url")
}

// campaignListIDs reads the {id, name} pairs a campaign row carries in Lists.
func campaignListIDs(c *models.Campaign) []int {
	if c == nil || len(c.Lists) == 0 {
		return nil
	}
	var lists []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(c.Lists, &lists); err != nil {
		return nil
	}
	out := make([]int, 0, len(lists))
	for _, x := range lists {
		if x.ID > 0 {
			out = append(out, x.ID)
		}
	}
	return out
}

// forCampaign returns the fallback destination for unresolvable dynamic links in a campaign
// (D2): the brand site derived from its lists' tags, else app.link_fallback_url, else "" (the
// caller renders the error page). c may lack Lists (the manager's next-campaigns row does
// not carry them); the campaign is then loaded by id.
func (l *linkFallbacks) forCampaign(c *models.Campaign) string {
	if c == nil || c.ID == 0 {
		return linkresolve.Fallback(linkresolve.Brand{}, l.setting())
	}

	l.mu.Lock()
	if e, ok := l.byID[c.ID]; ok && time.Since(e.at) < linkCacheTTL {
		l.mu.Unlock()
		return e.url
	}
	l.mu.Unlock()

	return l.resolve(c).url
}

// resolve reads the campaign's brand mapping and caches the fallback URL with the slug (fork,
// NAMING-STANDARD-SPEC §4.5): the slug is recorded only when the mapping resolved.
func (l *linkFallbacks) resolve(c *models.Campaign) cachedFallback {
	ids := campaignListIDs(c)
	if len(ids) == 0 {
		if full, err := l.core.GetCampaign(c.ID, "", ""); err == nil {
			ids = campaignListIDs(&full)
		}
	}

	b := linkresolve.Brand{}
	if m, err := resolveBrandMappingWith(l.core, l.i18n, ids); err != nil {
		l.log.Printf("campaign %d: brand mapping unavailable for link fallback, using app.link_fallback_url: %v", c.ID, err)
		b.Err = true
	} else {
		b.Mapped = m.mapped
		b.Slug = m.brand
		b.FromAddress = bareAddress(m.fromEmail)
		b.Site = m.site
	}

	e := cachedFallback{url: linkresolve.Fallback(b, l.setting()), at: time.Now()}
	if !b.Err && b.Mapped && b.Slug != "" {
		e.slug, e.mapped = b.Slug, true
	}

	l.mu.Lock()
	l.byID[c.ID] = e
	l.mu.Unlock()
	return e
}

// utmSlug is the brand slug the standard utm_campaign carries (NAMING-STANDARD-SPEC §4.5): the
// cached slug while the cached entry is fresh AND mapped; otherwise the mapping is read again (a
// failed or unmapped read is never trusted, so the next click retries) and "" when it still does
// not resolve -- the slug part is then omitted.
func (l *linkFallbacks) utmSlug(c models.Campaign) string {
	if c.ID == 0 {
		return ""
	}
	l.mu.Lock()
	e, ok := l.byID[c.ID]
	l.mu.Unlock()
	if ok && e.mapped && time.Since(e.at) < linkCacheTTL {
		return e.slug
	}
	if r := l.resolve(&c); r.mapped {
		return r.slug
	}
	return ""
}

// campaignByUUID loads (and caches) the campaign row behind a tracked link's campaign UUID —
// LinkRedirect's new lookup (§3.2); the click path previously loaded no campaign at all.
func (l *linkFallbacks) campaignByUUID(uuid string) (models.Campaign, bool) {
	if uuid == "" || uuid == dummyUUID {
		return models.Campaign{}, false
	}

	l.mu.Lock()
	if e, ok := l.byUUID[uuid]; ok && time.Since(e.at) < linkCacheTTL {
		l.mu.Unlock()
		return e.camp, true
	}
	l.mu.Unlock()

	camp, err := l.core.GetCampaign(0, uuid, "")
	if err != nil {
		return models.Campaign{}, false
	}

	l.mu.Lock()
	l.byUUID[uuid] = cachedCampaign{camp: camp, at: time.Now()}
	l.mu.Unlock()
	return camp, true
}

// hostUnion is the D7 storefront-host set: the domain of every configured from_addresses
// entry, every `site:` list-tag host, and the app.utm_hosts extras. Cached for linkCacheTTL.
func (l *linkFallbacks) hostUnion() []string {
	l.mu.Lock()
	if l.hosts != nil && time.Since(l.hostAt) < linkCacheTTL {
		out := l.hosts
		l.mu.Unlock()
		return out
	}
	l.mu.Unlock()

	var froms []string
	for a := range configuredFromAddresses() {
		froms = append(froms, a)
	}

	var sites []string
	if lists, err := l.core.GetLists("", "", true, nil); err == nil {
		for _, ls := range lists {
			if s := siteTagOf(ls.Tags); s != "" {
				sites = append(sites, s)
			}
		}
	} else {
		l.log.Printf("error reading lists for the UTM host union: %v", err)
	}

	hosts := linkresolve.HostUnion(froms, sites, l.ko.Strings("app.utm_hosts"))

	l.mu.Lock()
	l.hosts = hosts
	l.hostAt = time.Now()
	l.mu.Unlock()
	return hosts
}

// usesUTMCampaign reports whether any configured UTM template reads .Campaign.UTMCampaign -- the
// slug read (a brand-mapping lookup on an unmapped campaign) is skipped while the setting is dark.
func usesUTMCampaign(tpls map[string]string) bool {
	for _, v := range tpls {
		if strings.Contains(v, "UTMCampaign") {
			return true
		}
	}
	return false
}

// applyUTM appends the configured UTM parameters to dest (D7/D8) when tagging is enabled and
// the destination host is in the union. camp supplies the name/id the parameter templates read.
func (l *linkFallbacks) applyUTM(dest string, camp models.Campaign) string {
	if !l.ko.Bool("app.utm_enable") {
		return dest
	}
	// Fork (naming standard, NAMING-STANDARD-SPEC D11 §4.5) -- UTMCampaign is filled for every
	// click; only a setting that reads {{ .Campaign.UTMCampaign }} uses it (the release ships dark).
	tpls := l.ko.StringMap("app.utm_params")
	info := linkresolve.CampaignInfo{ID: camp.ID, Name: camp.Name}
	if usesUTMCampaign(tpls) {
		info.UTMCampaign = linkresolve.UTMCampaign(l.utmSlug(camp), camp.Name, camp.Lang())
	}
	params := linkresolve.RenderUTMParams(tpls, info)
	return linkresolve.ApplyUTM(dest, l.hostUnion(), params)
}

// invalidate drops every cached entry (settings reload).
func (l *linkFallbacks) invalidate() {
	l.mu.Lock()
	l.byID = map[int]cachedFallback{}
	l.byUUID = map[string]cachedCampaign{}
	l.hosts = nil
	l.mu.Unlock()
}
