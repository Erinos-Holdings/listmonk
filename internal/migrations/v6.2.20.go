package migrations

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/stuffbin"
	"github.com/lib/pq"
)

// V6_2_20 is a FORK migration (brand picker, integrations BRAND-PICKER-SPEC D1), not an upstream
// release. It adds the brands table -- a brand's sending identity (slug, From, site) -- and
// backfills one row per distinct brand: tag across lists, from that brand's from: and site: tags.
// Lists are not changed: their reserved tags are already the projection of the row it writes.
//
// The backfill FAILS (the transaction rolls back, nothing is created) naming the offending
// list(s) when the roster is inconsistent: a brand's lists disagree on from: or carry more than
// one distinct site:; a list is half-tagged (brand: without from:, or the reverse), carries two
// brand: or from: tags, or a site: without the pair; two brand: values differ only by case; or a
// brand fails models.BrandProblem(slug, from, site, nil, nil) (migrations receive no SMTP config,
// so the from_addresses check is skipped -- the campaign-save check stays that backstop). The
// inconsistency is fixed by hand before the upgrade, never resolved silently.
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.16 shape): the backfill reads
// lists, and a blocked upgrade fails loudly instead of waiting.
//
// ROLLBACK: revert the release commit in the integrations repo (image pin + the preset seed
// together) and re-apply the host. The older binary ignores the brands table and reads the tags;
// no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.19 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.20. Idempotent: INSERT ... ON CONFLICT DO NOTHING.
func V6_2_20(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(brandsDDL); err != nil {
		return err
	}

	var lists []backfillList
	if err := tx.Select(&lists, `SELECT id, name, COALESCE(tags, '{}') AS tags FROM lists ORDER BY id`); err != nil {
		return err
	}

	rows, err := planBrandBackfill(lists)
	if err != nil {
		return fmt.Errorf("v6.2.20 brands backfill: %w -- fix the list tags by hand, then upgrade again", err)
	}
	for _, b := range rows {
		if _, err := tx.Exec(`INSERT INTO brands (slug, from_email, site) VALUES ($1, $2, NULLIF($3, '')) ON CONFLICT (slug) DO NOTHING`,
			b.slug, b.from, b.site); err != nil {
			return fmt.Errorf("v6.2.20 brands backfill: brand %q: %w", b.slug, err)
		}
	}

	if lo != nil {
		lo.Printf("v6.2.20: brands backfill planned %d row(s)", len(rows))
	}
	return tx.Commit()
}

// brandsDDL is mirrored in schema.sql (the brands table and its LOWER(slug) index).
const brandsDDL = `
CREATE TABLE IF NOT EXISTS brands (
    slug             TEXT PRIMARY KEY,
    from_email       TEXT NOT NULL,
    site             TEXT NULL,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_brands_slug_lower ON brands (LOWER(slug));
`

type backfillList struct {
	ID   int            `db:"id"`
	Name string         `db:"name"`
	Tags pq.StringArray `db:"tags"`
}

type backfillBrand struct {
	slug, from, site string
	lists            []string
}

// listRef names a list in an error.
func listRef(id int, name string) string { return fmt.Sprintf("%d %q", id, name) }

// planBrandBackfill folds the roster into one row per brand, or the first inconsistency.
func planBrandBackfill(lists []backfillList) ([]backfillBrand, error) {
	bySlug := map[string]*backfillBrand{}
	var order []string

	for _, l := range lists {
		var brands, froms, sites []string
		for _, t := range l.Tags {
			t = strings.TrimSpace(t)
			switch {
			case strings.HasPrefix(t, models.BrandTagPrefix):
				brands = append(brands, strings.TrimSpace(strings.TrimPrefix(t, models.BrandTagPrefix)))
			case strings.HasPrefix(t, models.FromTagPrefix):
				froms = append(froms, strings.TrimSpace(strings.TrimPrefix(t, models.FromTagPrefix)))
			case strings.HasPrefix(t, models.SiteTagPrefix):
				sites = append(sites, strings.TrimSpace(strings.TrimPrefix(t, models.SiteTagPrefix)))
			}
		}
		ref := listRef(l.ID, l.Name)
		switch {
		case len(brands) == 0 && len(froms) == 0:
			if len(sites) > 0 {
				return nil, fmt.Errorf("list %s carries a site: tag without the brand: and from: tags", ref)
			}
			continue
		case len(brands) == 0 || len(froms) == 0:
			return nil, fmt.Errorf("list %s is half-tagged (it needs both the brand: and from: tags, or neither)", ref)
		case len(brands) > 1 || len(froms) > 1:
			return nil, fmt.Errorf("list %s carries more than one brand: or from: tag", ref)
		case len(sites) > 1:
			return nil, fmt.Errorf("list %s carries more than one site: tag", ref)
		}

		slug, from := brands[0], froms[0]
		site := ""
		if len(sites) == 1 {
			site = sites[0]
		}

		b, ok := bySlug[slug]
		if !ok {
			b = &backfillBrand{slug: slug, from: from}
			bySlug[slug] = b
			order = append(order, slug)
		}
		if b.from != from {
			return nil, fmt.Errorf("brand %q: lists %s carry from:%s but list %s carries from:%s", slug, strings.Join(b.lists, ", "), b.from, ref, from)
		}
		if site != "" {
			if b.site != "" && b.site != site {
				return nil, fmt.Errorf("brand %q: lists carry more than one distinct site: (%s, %s; list %s)", slug, b.site, site, ref)
			}
			b.site = site
		}
		b.lists = append(b.lists, ref)
	}

	// Two slugs differing only by case would be two SES dimensions nobody can tell apart, and
	// the LOWER(slug) index would refuse the second row anyway.
	byLower := map[string]string{}
	for _, slug := range order {
		k := strings.ToLower(slug)
		if other, ok := byLower[k]; ok {
			return nil, fmt.Errorf("brands %q (lists %s) and %q (lists %s) differ only by case", other, strings.Join(bySlug[other].lists, ", "),
				slug, strings.Join(bySlug[slug].lists, ", "))
		}
		byLower[k] = slug
	}

	sort.Strings(order)
	out := make([]backfillBrand, 0, len(order))
	for _, slug := range order {
		b := bySlug[slug]
		if key, args := models.BrandProblem(b.slug, b.from, b.site, nil, nil); key != "" {
			return nil, fmt.Errorf("brand %q (lists %s) fails validation: %s %s", b.slug, strings.Join(b.lists, ", "), key, strings.Join(args, "="))
		}
		out = append(out, *b)
	}
	return out, nil
}
