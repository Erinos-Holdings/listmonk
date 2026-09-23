-- Fork (brand health, integrations BRAND-HEALTH-SPEC D2/D3/D6). The fork stores and renders the
-- documents the BrandHealth Lambda computes and derives nothing from them beyond the row keys.
-- The default-sender row is the newest row flagged is_default (never a slug), and a list's brand
-- is list_brand_tag(tags) (v6.2.12). NO COLON IN ANY COMMENT LINE (goyesql tags).

-- name: upsert-brand-health
-- One statement over the whole batch, so a batch is all-or-nothing by construction. The handler
-- validates every row and refuses a (brand, day) pair twice in one batch before this runs.
INSERT INTO brand_health (brand, day, status, is_default, doc, computed_at)
    SELECT r->>'brand', (r->>'day')::DATE, r->>'status', (r->>'default')::BOOLEAN, r, NOW()
    FROM JSONB_ARRAY_ELEMENTS($1::JSONB) AS r
    ON CONFLICT (brand, day) DO UPDATE SET status = EXCLUDED.status, is_default = EXCLUDED.is_default,
        doc = EXCLUDED.doc, computed_at = NOW();

-- name: get-brand-health-latest
-- The latest row per brand, each with lists[] -- the lists tagged with the brand, plus every
-- untagged list on the default-sender row. $1 = all lists permitted, $2 = the permitted list ids.
WITH latest AS (
    SELECT DISTINCT ON (brand) brand, day, is_default, doc FROM brand_health ORDER BY brand, day DESC
),
dflt AS (
    SELECT brand, day FROM brand_health WHERE is_default ORDER BY day DESC, brand LIMIT 1
),
visible AS (
    SELECT id, name, list_brand_tag(tags) AS tag FROM lists
    WHERE CASE WHEN $1 = TRUE THEN TRUE ELSE id = ANY($2::INT[]) END
)
SELECT latest.doc || JSONB_BUILD_OBJECT('lists', COALESCE((
        SELECT JSONB_AGG(JSONB_BUILD_OBJECT('id', v.id, 'name', v.name) ORDER BY v.name, v.id) FROM visible v
        WHERE v.tag = latest.brand
            OR (v.tag IS NULL AND EXISTS (SELECT 1 FROM dflt WHERE dflt.brand = latest.brand AND dflt.day = latest.day))
    ), '[]'::JSONB)) AS doc
    FROM latest ORDER BY latest.brand;

-- name: get-brand-health-history
-- One brand's rows over the last $2 days (today included), newest first.
SELECT doc FROM brand_health WHERE brand = $1 AND day > (CURRENT_DATE - $2::INT) ORDER BY day DESC;
