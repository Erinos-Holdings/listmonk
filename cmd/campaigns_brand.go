package main

import (
	"errors"
	"regexp"
	"strings"

	"github.com/knadh/listmonk/internal/messenger/email"
)

// List-scoped From address and brand tag.
//
// A list carries its brand mapping as two tags:
//
//	brand:liyora
//	from:hello@liyorahair.com
//
// Both a campaign's From address and its `brand` SES message tag are derived from them, so
// neither is hand-typed on the campaign. This file is the ENFORCEMENT half. The campaign editor
// derives the same values for convenience, but that is Vue and is disposable at listmonk v7;
// this is the only part that constrains API and scripted sends.
//
// WHY IT EXISTS: nothing in listmonk validates a campaign's From address against its target
// lists. The editor's From is free text, and `from_addresses` on an SMTP block is ROUTING input
// rather than an allowlist -- an unmatched From keeps the full pool and picks at random. With a
// per-identity IAM policy a wrong From at least died with `554 Access denied`; once the sending
// policy became a region-scoped wildcard, a mis-addressed campaign instead SENDS SUCCESSFULLY:
// correctly DKIM-signed, SPF-aligned, dmarc=pass, as the wrong brand, to the right subscribers,
// with no error in any log or metric. At three brands that is a procedural check; at the ~100
// this platform is meant to carry it is not survivable.
const (
	brandTagPrefix = "brand:"
	fromTagPrefix  = "from:"

	// The SES message-tag header, and the tag key within it that carries the brand. SES reads
	// message tags off this header; the CloudWatch event destination dimensions on `brand`.
	sesTagHeader = "X-SES-MESSAGE-TAGS"
	brandTagKey  = "brand"

	// The slug for a campaign whose lists carry no brand tags.
	//
	// AN UNMAPPED CAMPAIGN GETS THE DEFAULT SLUG, NEVER NO TAG AT ALL. Otherwise every internal
	// seed send lands in the `unattributed` CloudWatch dimension and fires the
	// Listmonk-Unattributed-Sends alarm as *designed behaviour* -- and an alarm that fires on
	// routine activity gets muted, at which point it has stopped being a backstop too.
	// `unattributed` must stay reserved for genuine mistakes.
	defaultBrandSlug = "curated"
)

// SES message tag values accept only alphanumerics, `-` and `_`.
//
// This is checked here, and not only in the editor, because LIST TAGS ARE STORED VERBATIM --
// there is no normalisation or validation anywhere in listmonk's list handling, which is the
// property this whole design rests on. Left unchecked, `brand:Thirsty Girl` emits
// `X-SES-MESSAGE-TAGS: brand=Thirsty Girl` and SES rejects the message AT SEND TIME, on a
// campaign nobody touched, weeks after someone edited a list tag. That is the same
// hand-typed-value-with-no-validation failure this feature exists to remove, merely relocated
// from the campaign to the list.
var reBrandSlug = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// brandMapping is the single brand + From address that a campaign's target lists describe.
// `mapped` is false when no target list carries brand tags at all, which is not an error: the
// internal seed list and the bounce simulator are deliberately unmapped and must keep working.
type brandMapping struct {
	brand     string
	fromEmail string
	listName  string
	mapped    bool
}

// resolveBrandMapping resolves a campaign's list IDs to the one brand/From pair their tags
// describe, or to the reason that cannot be done.
func (a *App) resolveBrandMapping(listIDs []int) (brandMapping, error) {
	out := brandMapping{}
	if len(listIDs) == 0 {
		return out, nil
	}

	// An empty optin type matches every list, so this is "get these lists by ID" -- the only
	// existing accessor that returns whole list rows (tags included) for a set of IDs.
	lists, err := a.core.GetListsByOptin(listIDs, "")
	if err != nil {
		return out, err
	}

	type mapped struct{ name, brand, from string }

	var (
		found      []mapped
		halfTagged []string
	)

	for _, l := range lists {
		var brand, from string
		for _, t := range l.Tags {
			switch {
			case strings.HasPrefix(t, brandTagPrefix):
				brand = strings.TrimSpace(strings.TrimPrefix(t, brandTagPrefix))
			case strings.HasPrefix(t, fromTagPrefix):
				from = strings.TrimSpace(strings.TrimPrefix(t, fromTagPrefix))
			}
		}

		// Neither tag: an unmapped list. It contributes nothing and constrains nothing.
		if brand == "" && from == "" {
			continue
		}

		// One tag but not the other is a MISCONFIGURATION, not an unmapped list -- it is how a
		// brand ends up attributed but wrongly addressed, or vice versa.
		if brand == "" || from == "" {
			halfTagged = append(halfTagged, l.Name)
			continue
		}

		found = append(found, mapped{name: l.Name, brand: brand, from: from})
	}

	if len(halfTagged) > 0 {
		return out, errors.New(a.i18n.Ts("campaigns.brandFromHalfTagged", "lists", strings.Join(halfTagged, ", ")))
	}

	if len(found) == 0 {
		return out, nil
	}

	// Multi-brand campaigns are blocked by decision, not deferred: a cross-brand send has no valid
	// single From. Name both brands rather than silently picking the first.
	var brands, addrs []string
	for _, m := range found {
		if !contains(brands, m.brand) {
			brands = append(brands, m.brand)
		}
		if !contains(addrs, m.from) {
			addrs = append(addrs, m.from)
		}
	}

	if len(brands) > 1 {
		return out, errors.New(a.i18n.Ts("campaigns.brandFromConflict", "brands", strings.Join(brands, ", ")))
	}

	if len(addrs) > 1 {
		return out, errors.New(a.i18n.Ts("campaigns.brandFromAddressConflict", "addresses", strings.Join(addrs, ", ")))
	}

	if !reBrandSlug.MatchString(brands[0]) {
		return out, errors.New(a.i18n.Ts("campaigns.brandFromInvalidSlug", "brand", brands[0], "list", found[0].name))
	}

	// The `from:` tag must name an address that is actually configured for sending. Without this a
	// tag pointing at a domain nobody has verified in SES surfaces as a `554` at send time, on a
	// campaign that looked fine -- and a failed send is never written to the app log.
	//
	// SKIPPED when no SMTP block declares any from_addresses, which is listmonk's default. The
	// field is routing input upstream, so treating an empty one as "nothing is allowed" would
	// break every install that has not opted in.
	if allowed := configuredFromAddresses(); len(allowed) > 0 {
		if _, ok := allowed[email.NormalizeAddr(addrs[0])]; !ok {
			return out, errors.New(a.i18n.Ts("campaigns.brandFromUnknownAddress", "from", addrs[0], "list", found[0].name))
		}
	}

	return brandMapping{brand: brands[0], fromEmail: addrs[0], listName: found[0].name, mapped: true}, nil
}

// setBrandTagHeader merges `brand=<slug>` into a campaign's X-SES-MESSAGE-TAGS header.
//
// THIS IS WHY THE BRAND TAG IS ENFORCED HERE AND NOT ONLY IN THE EDITOR. The campaign editor
// derives the same tag, but it is Vue and upstream deletes the entire admin SPA at v7 -- so an
// editor-only implementation means a campaign created over the API sends untagged today, and
// EVERY campaign sends untagged the moment that UI goes away. A send with no `brand` tag lands in
// the `unattributed` CloudWatch dimension, which is the one bucket that has to stay meaningful.
//
// Headers is a free-text JSON array carrying other headers too, so only the X-SES-MESSAGE-TAGS
// entry is touched and every other entry is passed through unchanged.
func setBrandTagHeader(headers []map[string]string, slug string) []map[string]string {
	out := make([]map[string]string, 0, len(headers)+1)
	found := false

	for _, h := range headers {
		entry := make(map[string]string, len(h))
		for k, v := range h {
			if strings.EqualFold(k, sesTagHeader) {
				found = true
				entry[k] = setBrandInTagValue(v, slug)
			} else {
				entry[k] = v
			}
		}
		out = append(out, entry)
	}

	if !found {
		out = append(out, map[string]string{sesTagHeader: brandTagKey + "=" + slug})
	}

	return out
}

// setBrandInTagValue rewrites the `brand=` pair inside an "a=b, c=d" SES message-tag value,
// leaving any other pair alone. Replacing the whole value would silently drop a second tag
// someone added by hand.
func setBrandInTagValue(value, slug string) string {
	var (
		out      []string
		replaced bool
	)

	for _, pair := range strings.Split(value, ",") {
		p := strings.TrimSpace(pair)
		if p == "" {
			continue
		}

		key := p
		if eq := strings.Index(p, "="); eq != -1 {
			key = p[:eq]
		}

		if strings.EqualFold(strings.TrimSpace(key), brandTagKey) {
			// Drop any duplicate brand= pair rather than emitting two.
			if !replaced {
				out = append(out, brandTagKey+"="+slug)
				replaced = true
			}
			continue
		}

		out = append(out, p)
	}

	if !replaced {
		out = append([]string{brandTagKey + "=" + slug}, out...)
	}

	return strings.Join(out, ", ")
}

// configuredFromAddresses returns the set of from_addresses declared across the enabled SMTP
// blocks, normalised the same way the email messenger normalises them when building its routing
// pools. Read from the live config rather than cached, so it follows a settings reload.
func configuredFromAddresses() map[string]struct{} {
	out := map[string]struct{}{}

	for _, item := range ko.Slices("smtp") {
		if !item.Bool("enabled") {
			continue
		}

		for _, addr := range item.Strings("from_addresses") {
			if k := email.NormalizeAddr(addr); k != "" {
				out[k] = struct{}{}
			}
		}
	}

	return out
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}
