-- media
-- name: insert-media
-- Fork (media tags) -- MEDIA-TAGS-SPEC 3.3. $7 is the normalized tag set from the upload form.
INSERT INTO media (uuid, filename, thumb, content_type, provider, meta, tags, created_at) VALUES($1, $2, $3, $4, $5, $6, $7, NOW()) RETURNING id;

-- name: query-media
-- Fork (media tags) -- MEDIA-TAGS-SPEC D4. The tag filter is OR (array overlap), deliberately
-- unlike the lists query's AND containment. $5 is the selected tags, $6 adds untagged rows,
-- and with neither there is no tag filter at all. The filename search ANDs with all of it.
-- A NULL $5 reads as empty, so a nil bind can never blank the page.
-- The LIMIT CASE follows campaigns.sql so per_page=all (limit below 1) returns every row.
SELECT COUNT(*) OVER () AS total, * FROM media
    WHERE ($1 = '' OR filename ILIKE $1) AND provider=$2
      AND (
        (CARDINALITY(COALESCE($5::VARCHAR(100)[], '{}')) = 0 AND NOT $6::BOOLEAN)
        OR tags && $5::VARCHAR(100)[]
        OR ($6::BOOLEAN AND CARDINALITY(tags) = 0)
      )
    ORDER BY created_at DESC OFFSET $3 LIMIT (CASE WHEN $4 < 1 THEN NULL ELSE $4 END);

-- name: get-media
SELECT * FROM media WHERE
    CASE
        WHEN $1 > 0 THEN id = $1
        WHEN $2 != '' THEN uuid = $2::UUID
        WHEN $3 != '' THEN filename = $3
        ELSE false
    END;

-- name: delete-media
DELETE FROM media WHERE id=$1 RETURNING filename;


-- name: update-media-meta
-- Fork (dark-mode readiness) -- DARK-MODE-SPEC D4/D5. Merges a key set into media.meta so a
-- re-classification never drops width/height (or anything a later feature adds).
UPDATE media SET meta = meta || $2 WHERE id = $1;

-- name: update-media-tags
-- Fork (media tags) -- MEDIA-TAGS-SPEC 3.4. Replaces the tag set and touches nothing else,
-- so meta (the dark-mode verdict) survives a tag edit and a reprocess survives a tag set.
UPDATE media SET tags = $2::VARCHAR(100)[] WHERE id = $1 RETURNING *;

-- name: get-media-tags
-- Fork (media tags) -- MEDIA-TAGS-SPEC D8. Each distinct tag once, with its row count, for
-- the picker's autocomplete.
SELECT tag, COUNT(*) AS count FROM media, UNNEST(tags) AS tag WHERE provider=$1 GROUP BY tag ORDER BY tag;
