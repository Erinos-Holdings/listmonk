-- Fork (system health, integrations SES-HEALTH-SPEC D1/D6). The fork stores the system-wide
-- documents the integrations BrandHealth Lambda computes (one per kind per day) and derives
-- nothing from them beyond the row keys. NO COLON IN ANY COMMENT LINE (goyesql tags).

-- name: upsert-system-health
-- One statement over the whole batch, so a batch is all-or-nothing by construction. The handler
-- validates every row and refuses a (kind, day) pair twice in one batch before this runs.
INSERT INTO system_health (kind, day, status, doc, computed_at)
    SELECT r->>'kind', (r->>'day')::DATE, r->>'status', r, NOW()
    FROM JSONB_ARRAY_ELEMENTS($1::JSONB) AS r
    ON CONFLICT (kind, day) DO UPDATE SET status = EXCLUDED.status, doc = EXCLUDED.doc, computed_at = NOW();

-- name: get-system-health-history
-- One kind's rows over the last $2 days (today included), newest first. An unknown kind is
-- simply no rows.
SELECT doc FROM system_health WHERE kind = $1 AND day > (CURRENT_DATE - $2::INT) ORDER BY day DESC;
