-- Fork (campaign review, integrations CAMPAIGN-INSPECT-SPEC D5). The fork stores the review
-- Lambda's reports verbatim and the append-only disposition log; it computes the verdict in Go
-- (internal/review) and nothing else. NO COLON IN ANY COMMENT LINE (goyesql tags).

-- name: insert-campaign-review
INSERT INTO campaign_reviews (campaign_id, bundle_hash, job_id, status, requested_by)
    VALUES ($1, $2, $3, 'running', $4)
    RETURNING *;

-- name: update-campaign-review-progress
-- Only a running row moves; zero rows = not running (the handler answers 409).
UPDATE campaign_reviews SET progress = $3, updated_at = NOW()
    WHERE campaign_id = $1 AND job_id = $2 AND status = 'running'
    RETURNING *;

-- name: update-campaign-review-result
-- complete (report), failed (error) or stale. Only a running row moves.
UPDATE campaign_reviews SET status = $3, report = $4, error = $5, updated_at = NOW()
    WHERE campaign_id = $1 AND job_id = $2 AND status = 'running'
    RETURNING *;

-- name: get-campaign-review-latest
-- The newest row of a campaign, whatever its status.
SELECT * FROM campaign_reviews WHERE campaign_id = $1 ORDER BY requested_at DESC, id DESC LIMIT 1;

-- name: get-campaign-review-by-hash
-- The newest COMPLETE row for a campaign at a bundle hash -- a newer failed or stale row for the
-- same hash does not hide it (the gate, D6).
SELECT * FROM campaign_reviews WHERE campaign_id = $1 AND bundle_hash = $2 AND status = 'complete'
    ORDER BY requested_at DESC, id DESC LIMIT 1;

-- name: get-campaign-review-previous
-- The newest complete row of a campaign requested before the given job (the A-tier cache, D14).
SELECT * FROM campaign_reviews WHERE campaign_id = $1 AND status = 'complete'
    AND requested_at < (SELECT requested_at FROM campaign_reviews WHERE job_id = $2)
    ORDER BY requested_at DESC, id DESC LIMIT 1;

-- name: get-campaign-review-by-job
SELECT * FROM campaign_reviews WHERE campaign_id = $1 AND job_id = $2;

-- name: expire-running-reviews
-- A running row older than 6 minutes is dead (the Lambda's own deadline is 4.5 minutes).
UPDATE campaign_reviews SET status = 'failed', error = 'timeout', updated_at = NOW()
    WHERE campaign_id = $1 AND status = 'running' AND requested_at < NOW() - INTERVAL '6 minutes';

-- name: insert-review-disposition
-- Append-only. The Accept-risk log is this table.
INSERT INTO campaign_review_dispositions (campaign_id, bundle_hash, item_key, rubric_id, action, note, user_id, username)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    RETURNING *;

-- name: get-review-dispositions
-- Every row of a campaign, newest first (the core folds to the latest per key).
SELECT * FROM campaign_review_dispositions WHERE campaign_id = $1 ORDER BY created_at DESC, id DESC;

-- name: get-review-disposition-stats
-- Per rubric id, how often each action was taken, over how many campaigns, and when last.
SELECT rubric_id,
    COUNT(*) FILTER (WHERE action = 'accept') AS accepts,
    COUNT(*) FILTER (WHERE action = 'fixme') AS fixmes,
    COUNT(*) FILTER (WHERE action = 'fixed') AS fixed,
    COUNT(DISTINCT campaign_id) AS campaigns,
    MAX(created_at) AS last
    FROM campaign_review_dispositions GROUP BY rubric_id ORDER BY accepts DESC, rubric_id;

-- name: upsert-structure-verification
INSERT INTO campaign_structure_verifications (fingerprint, verified_at, test_id, components, verified_by)
    VALUES ($1, NOW(), $2, $3, $4)
    ON CONFLICT (fingerprint) DO UPDATE SET verified_at = NOW(), test_id = EXCLUDED.test_id,
        components = EXCLUDED.components, verified_by = EXCLUDED.verified_by
    RETURNING *;

-- name: get-structure-verification
SELECT * FROM campaign_structure_verifications WHERE fingerprint = $1;

-- name: get-structure-verifications
-- Every record, newest first (D4.1 names what differs from the newest; D4.2 unions them).
SELECT * FROM campaign_structure_verifications ORDER BY verified_at DESC LIMIT 200;
