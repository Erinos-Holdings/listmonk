-- links
-- name: create-link
INSERT INTO links (uuid, url) VALUES($1, $2) ON CONFLICT (url) DO UPDATE SET url=EXCLUDED.url RETURNING uuid;

-- name: get-link-url
SELECT url FROM links WHERE uuid = $1;

-- name: register-link-click
-- Fork (location stats) -- $4 is the normalized CloudFront-Viewer-Country code, or '' for unknown (stored NULL).
-- Fork (client stats, CLIENT-STATS-SPEC D2/D4) -- $5 is the classified User-Agent token, or '' for unknown (stored NULL).
WITH link AS(
    SELECT id, url FROM links WHERE uuid = $1
)
INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, country, client) VALUES(
    (SELECT id FROM campaigns WHERE uuid = $2),
    (SELECT id FROM subscribers WHERE
        (CASE WHEN $3::TEXT != '' THEN subscribers.uuid = $3::UUID ELSE FALSE END)
    ),
    (SELECT id FROM link),
    NULLIF($4::TEXT, ''),
    NULLIF($5::TEXT, '')
) RETURNING (SELECT url FROM link);
