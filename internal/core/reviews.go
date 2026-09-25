package core

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (campaign review, integrations CAMPAIGN-INSPECT-SPEC D2/D5/D12). The review Lambda computes
// the report; the fork starts a job, stores what the Lambda writes back, keeps the append-only
// disposition log, and computes the verdict (internal/review). Nothing here touches the send path.

// ReviewConfig is the live gate configuration, read from koanf by the handler on every call (the
// go-live switch is a settings save + reload, D19).
type ReviewConfig struct {
	URL    string
	Secret string
	// Client posts the job; nil = an 8 s client (the brandtheme precedent).
	Client *http.Client
	// Now stamps X-Review-Timestamp; nil = time.Now.
	Now func() time.Time
}

// ReviewJob is the body the fork POSTs to the Lambda's Function URL (D2).
type ReviewJob struct {
	JobID       string `json:"jobId"`
	CampaignID  int    `json:"campaignId"`
	BundleHash  string `json:"bundleHash"`
	RequestedBy string `json:"requestedBy"`
}

// DispositionIn is one checklist decision as the window posts it.
type DispositionIn struct {
	Key      string `json:"key"`
	RubricID string `json:"rubric_id"`
	Action   string `json:"action"`
	Note     string `json:"note"`
}

// ReviewState is what GET .../reviews/latest returns: the newest row, the whole disposition log
// (newest first), the fork's verdict over that row, and the campaign's CURRENT bundle hash (the
// window and the campaign page read "edited since" from row.bundle_hash != current_hash).
type ReviewState struct {
	Review       *models.CampaignReview     `json:"review"`
	Dispositions []models.ReviewDisposition `json:"dispositions"`
	Verdict      review.VerdictResult       `json:"verdict"`
	CurrentHash  string                     `json:"current_hash"`
}

var reviewClient = &http.Client{Timeout: 8 * time.Second}

// CampaignListIDs are a campaign's target list ids (from its {id, name} lists JSON).
func CampaignListIDs(cm models.Campaign) []int {
	var ls []struct {
		ID int `json:"id"`
	}
	if len(cm.Lists) > 0 {
		_ = json.Unmarshal(cm.Lists, &ls)
	}
	out := make([]int, 0, len(ls))
	for _, l := range ls {
		if l.ID > 0 {
			out = append(out, l.ID)
		}
	}
	return out
}

// CampaignBundleHash is review.BundleHash over a campaign row and its lists.
func CampaignBundleHash(cm models.Campaign) string {
	return review.BundleHash(cm, CampaignListIDs(cm))
}

func jsonOrNil(b json.RawMessage) any {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return string(b)
}

func (c *Core) reviewErr(err error, action string) error {
	c.log.Printf("error %s campaign review: %v", action, err)
	return echo.NewHTTPError(http.StatusInternalServerError,
		c.i18n.Ts("globals.messages.errorFetching", "name", "campaign review", "error", pqErrMsg(err)))
}

// StartCampaignReview hashes the saved campaign, expires dead running rows, refuses a live one
// (409), inserts a running row and POSTs the signed job. A non-202 (or a transport error) marks
// the row failed and answers 502 campaigns.reviewUnavailable.
func (c *Core) StartCampaignReview(cm models.Campaign, requestedBy string, cfg ReviewConfig) (models.CampaignReview, error) {
	if cfg.URL == "" {
		return models.CampaignReview{}, echo.NewHTTPError(http.StatusServiceUnavailable, c.i18n.T("campaigns.reviewNotConfigured"))
	}

	if _, err := c.q.ExpireRunningReviews.Exec(cm.ID); err != nil {
		return models.CampaignReview{}, c.reviewErr(err, "expiring")
	}
	var latest models.CampaignReview
	if err := c.q.GetCampaignReviewLatest.Get(&latest, cm.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.CampaignReview{}, c.reviewErr(err, "fetching")
	} else if err == nil && latest.Status == models.ReviewStatusRunning {
		return models.CampaignReview{}, echo.NewHTTPError(http.StatusConflict, c.i18n.T("campaigns.reviewRunning"))
	}

	hash := CampaignBundleHash(cm)
	jobID := uuid.Must(uuid.NewV4()).String()
	var row models.CampaignReview
	if err := c.q.InsertCampaignReview.Get(&row, cm.ID, hash, jobID, requestedBy); err != nil {
		return models.CampaignReview{}, c.reviewErr(err, "inserting")
	}

	body, _ := json.Marshal(ReviewJob{JobID: jobID, CampaignID: cm.ID, BundleHash: hash, RequestedBy: requestedBy})
	if err := postReviewJob(cfg, body); err != nil {
		c.log.Printf("campaign %d: review job %s not accepted: %v", cm.ID, jobID, err)
		var failed models.CampaignReview
		if err2 := c.q.UpdateCampaignReviewResult.Get(&failed, cm.ID, jobID, models.ReviewStatusFailed, nil, "not accepted: "+err.Error()); err2 != nil {
			c.log.Printf("campaign %d: marking review %s failed: %v", cm.ID, jobID, err2)
		}
		return models.CampaignReview{}, echo.NewHTTPError(http.StatusBadGateway, c.i18n.T("campaigns.reviewUnavailable"))
	}
	return row, nil
}

func postReviewJob(cfg ReviewConfig, body []byte) error {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	client := reviewClient
	if cfg.Client != nil {
		client = cfg.Client
	}
	ts := now().Unix()
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Review-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Review-Signature", review.Sign(cfg.Secret, ts, body))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// GetLatestReview returns the newest row (nil when none) with the 6-minute expiry applied on read,
// the disposition log, and the fork's verdict over that row.
func (c *Core) GetLatestReview(cm models.Campaign) (ReviewState, error) {
	out := ReviewState{Dispositions: []models.ReviewDisposition{}, CurrentHash: CampaignBundleHash(cm)}
	if _, err := c.q.ExpireRunningReviews.Exec(cm.ID); err != nil {
		return out, c.reviewErr(err, "expiring")
	}
	var row models.CampaignReview
	if err := c.q.GetCampaignReviewLatest.Get(&row, cm.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, c.reviewErr(err, "fetching")
	} else if err == nil {
		out.Review = &row
	}
	ds, err := c.GetReviewDispositions(cm.ID)
	if err != nil {
		return out, err
	}
	out.Dispositions = ds
	if out.Review != nil {
		out.Verdict = review.Verdict(json.RawMessage(out.Review.Report), ds)
	} else {
		out.Verdict = review.VerdictResult{Verdict: review.VerdictNone, Blockers: []review.Blocker{}}
	}
	return out, nil
}

// GetReviewDispositions returns a campaign's whole disposition log, newest first.
func (c *Core) GetReviewDispositions(campID int) ([]models.ReviewDisposition, error) {
	ds := []models.ReviewDisposition{}
	if err := c.q.GetReviewDispositions.Select(&ds, campID); err != nil {
		return nil, c.reviewErr(err, "fetching dispositions for")
	}
	return ds, nil
}

// GetReviewByHash returns the newest COMPLETE row for (campaign, hash), or nil.
func (c *Core) GetReviewByHash(campID int, hash string) (*models.CampaignReview, error) {
	var row models.CampaignReview
	if err := c.q.GetCampaignReviewByHash.Get(&row, campID, hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, c.reviewErr(err, "fetching")
	}
	return &row, nil
}

// GetPreviousReview returns the newest complete row requested before jobID (D14's cache), or nil.
func (c *Core) GetPreviousReview(campID int, jobID string) (*models.CampaignReview, error) {
	var row models.CampaignReview
	if err := c.q.GetCampaignReviewPrevious.Get(&row, campID, jobID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, c.reviewErr(err, "fetching")
	}
	return &row, nil
}

// GetReviewByJob returns one row, or nil.
func (c *Core) GetReviewByJob(campID int, jobID string) (*models.CampaignReview, error) {
	var row models.CampaignReview
	if err := c.q.GetCampaignReviewByJob.Get(&row, campID, jobID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, c.reviewErr(err, "fetching")
	}
	return &row, nil
}

// UpdateReviewProgress stores a running row's progress. A row that is not running is a 409.
func (c *Core) UpdateReviewProgress(campID int, jobID string, progress json.RawMessage) (models.CampaignReview, error) {
	var row models.CampaignReview
	if len(bytes.TrimSpace(progress)) == 0 {
		progress = json.RawMessage(`{}`)
	}
	if err := c.q.UpdateCampaignReviewProgress.Get(&row, campID, jobID, string(progress)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, echo.NewHTTPError(http.StatusConflict, c.i18n.T("campaigns.reviewNotRunning"))
		}
		return row, c.reviewErr(err, "updating")
	}
	return row, nil
}

// CompleteReview ends a running row: complete (report), failed (error) or stale. Not running = 409.
func (c *Core) CompleteReview(campID int, jobID, status string, report json.RawMessage, errMsg string) (models.CampaignReview, error) {
	var row models.CampaignReview
	if err := c.q.UpdateCampaignReviewResult.Get(&row, campID, jobID, status, jsonOrNil(report), errMsg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, echo.NewHTTPError(http.StatusConflict, c.i18n.T("campaigns.reviewNotRunning"))
		}
		return row, c.reviewErr(err, "updating")
	}
	return row, nil
}

// AddDispositions appends checklist decisions against the campaign's newest COMPLETE report. It
// refuses (400) an `accept` of an item the report marks acceptable:false, and any key the report
// does not carry except the pseudo-keys A (the AI-unavailable override) and R (the declined
// rendering-matrix offer). Rows are only ever inserted -- the log is append-only (I4).
func (c *Core) AddDispositions(campID int, userID int, username string, items []DispositionIn) ([]models.ReviewDisposition, error) {
	if len(items) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.invalidFields", "name", "items"))
	}
	var row models.CampaignReview
	if err := c.q.GetCampaignReviewLatest.Get(&row, campID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.reviewNoReport"))
		}
		return nil, c.reviewErr(err, "fetching")
	}
	if row.Status != models.ReviewStatusComplete {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.reviewNoReport"))
	}
	keys, err := review.ItemKeys(json.RawMessage(row.Report))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("campaigns.reviewNoReport"))
	}

	for _, it := range items {
		switch it.Action {
		case models.DispositionAccept, models.DispositionFixme, models.DispositionFixed:
		default:
			return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.invalidFields", "name", "action"))
		}
		if it.Key == review.KeyAI || it.Key == review.KeyR {
			continue
		}
		k, ok := keys[it.Key]
		if !ok {
			return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("campaigns.reviewUnknownKey", "key", it.Key))
		}
		if it.Action == models.DispositionAccept && !k.Acceptable {
			return nil, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("campaigns.reviewNotAcceptable", "id", k.ID))
		}
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return nil, c.reviewErr(err, "starting")
	}
	defer tx.Rollback()
	var uid any
	if userID > 0 {
		uid = userID
	}
	out := make([]models.ReviewDisposition, 0, len(items))
	for _, it := range items {
		rubricID := it.RubricID
		if k, ok := keys[it.Key]; ok {
			rubricID = k.ID
		} else if rubricID == "" {
			rubricID = it.Key
		}
		var d models.ReviewDisposition
		if err := tx.Stmtx(c.q.InsertReviewDisposition).Get(&d, campID, row.BundleHash, it.Key, rubricID, it.Action, it.Note, uid, username); err != nil {
			return nil, c.reviewErr(err, "recording a disposition for")
		}
		out = append(out, d)
	}
	if err := tx.Commit(); err != nil {
		return nil, c.reviewErr(err, "committing")
	}
	return out, nil
}

// ReviewStats is the Accept-risk log aggregated per rubric id.
func (c *Core) ReviewStats() ([]models.ReviewStat, error) {
	out := []models.ReviewStat{}
	if err := c.q.GetReviewDispositionStats.Select(&out); err != nil {
		return nil, c.reviewErr(err, "aggregating")
	}
	return out, nil
}

// PutStructureVerification records a clean rendering-matrix read for a fingerprint.
func (c *Core) PutStructureVerification(fp, testID string, components json.RawMessage, by string) (models.StructureVerification, error) {
	var out models.StructureVerification
	if len(bytes.TrimSpace(components)) == 0 {
		components = json.RawMessage(`{}`)
	}
	if err := c.q.UpsertStructureVerification.Get(&out, fp, testID, string(components), by); err != nil {
		return out, c.reviewErr(err, "recording structure for")
	}
	return out, nil
}

// GetStructureVerification returns the record for fp (nil when none) and every record, newest first.
func (c *Core) GetStructureVerification(fp string) (*models.StructureVerification, []models.StructureVerification, error) {
	all := []models.StructureVerification{}
	if err := c.q.GetStructureVerifications.Select(&all); err != nil {
		return nil, nil, c.reviewErr(err, "fetching structure records for")
	}
	var match models.StructureVerification
	if err := c.q.GetStructureVerification.Get(&match, fp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, all, nil
		}
		return nil, nil, c.reviewErr(err, "fetching structure for")
	}
	return &match, all, nil
}
