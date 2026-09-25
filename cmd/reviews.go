package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (campaign review, integrations CAMPAIGN-INSPECT-SPEC D2/D6/D7/D12). The Inspect button,
// the review Lambda's write-back, the checklist window's reads and decisions, and the status gate.
// The gate is ON only while app.review_url is non-empty (read live from koanf -- the go-live switch
// is a settings save + app reload, D19); off, every path here but the reads is inert and the Start
// path is exactly the pre-release one.

var reReviewJobID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var reFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

// reviewConfig is the live gate configuration.
func reviewConfig() core.ReviewConfig {
	return core.ReviewConfig{URL: ko.String("app.review_url"), Secret: ko.String("app.review_hmac_secret")}
}

// reviewEnabled reports whether the gate is on.
func reviewEnabled() bool {
	return ko.String("app.review_url") != ""
}

// StartCampaignReview (POST /api/campaigns/:id/reviews) inspects the SAVED campaign: the SPA saves
// first, exactly as Start does (D12).
func (a *App) StartCampaignReview(c echo.Context) error {
	id := getID(c)
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}
	cfg := reviewConfig()
	if cfg.URL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, a.i18n.T("campaigns.reviewNotConfigured"))
	}
	cm, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}
	user := auth.GetUser(c)
	row, err := a.core.StartCampaignReview(cm, user.Username, cfg)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{row})
}

// GetCampaignReviewLatest (GET /api/campaigns/:id/reviews/latest) -- the window's and the campaign
// page's one read: the newest row, the disposition log, the fork's verdict, the current hash.
func (a *App) GetCampaignReviewLatest(c echo.Context) error {
	id := getID(c)
	if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
		return err
	}
	cm, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}
	out, err := a.core.GetLatestReview(cm)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// GetCampaignReviewPrevious (GET /api/campaigns/:id/reviews/previous?before=<jobId>) -- the review
// Lambda's A-tier cache source (D14). null when there is none.
func (a *App) GetCampaignReviewPrevious(c echo.Context) error {
	id := getID(c)
	before := c.QueryParam("before")
	if !reReviewJobID.MatchString(before) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "before"))
	}
	row, err := a.core.GetPreviousReview(id, before)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{row})
}

// validateCompleteReport checks the SHAPE of a complete report (the rubric's id list is the
// Lambda's, D17): v == 1, the bundle hash is the row's, items non-empty, each with id/tier/verdict.
func validateCompleteReport(report json.RawMessage, rowHash string) error {
	var r struct {
		V          *int   `json:"v"`
		BundleHash string `json:"bundleHash"`
		Items      []struct {
			ID      string `json:"id"`
			Tier    string `json:"tier"`
			Verdict string `json:"verdict"`
		} `json:"items"`
	}
	if err := json.Unmarshal(report, &r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "report is not a JSON object")
	}
	if r.V == nil || *r.V != 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "report.v must be 1")
	}
	if r.BundleHash != rowHash {
		return echo.NewHTTPError(http.StatusBadRequest, "report.bundleHash does not match the review")
	}
	if len(r.Items) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "report.items is empty")
	}
	for i, it := range r.Items {
		if it.ID == "" || it.Tier == "" || it.Verdict == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "report.items["+itoaInt(i)+"] needs id, tier and verdict")
		}
	}
	return nil
}

func itoaInt(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// PatchCampaignReview (PATCH /api/campaigns/:id/reviews/:jobId) -- the review Lambda's write-back:
// {status:"running", progress} | {status:"complete", report} | {status:"failed", error} |
// {status:"stale"}. Only a running row moves; anything else is a 409 (a replayed job, or a job
// the 6-minute expiry already failed).
func (a *App) PatchCampaignReview(c echo.Context) error {
	id := getID(c)
	jobID := c.Param("jobId")
	if !reReviewJobID.MatchString(jobID) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "jobId"))
	}
	var req struct {
		Status   string          `json:"status"`
		Progress json.RawMessage `json:"progress"`
		Report   json.RawMessage `json:"report"`
		Error    string          `json:"error"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "body"))
	}
	row, err := a.core.GetReviewByJob(id, jobID)
	if err != nil {
		return err
	}
	if row == nil {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", "review"))
	}

	var out models.CampaignReview
	switch req.Status {
	case models.ReviewStatusRunning:
		out, err = a.core.UpdateReviewProgress(id, jobID, req.Progress)
	case models.ReviewStatusComplete:
		if row.Status != models.ReviewStatusRunning {
			return echo.NewHTTPError(http.StatusConflict, a.i18n.T("campaigns.reviewNotRunning"))
		}
		if err := validateCompleteReport(req.Report, row.BundleHash); err != nil {
			return err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, req.Report); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "report is not valid JSON")
		}
		out, err = a.core.CompleteReview(id, jobID, models.ReviewStatusComplete, compact.Bytes(), "")
	case models.ReviewStatusFailed:
		msg := req.Error
		if msg == "" {
			msg = "failed"
		}
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		out, err = a.core.CompleteReview(id, jobID, models.ReviewStatusFailed, nil, msg)
	case models.ReviewStatusStale:
		out, err = a.core.CompleteReview(id, jobID, models.ReviewStatusStale, nil, "edited since the inspection started")
	default:
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "status"))
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// PostReviewDispositions (POST /api/campaigns/:id/reviews/dispositions) appends checklist decisions.
func (a *App) PostReviewDispositions(c echo.Context) error {
	id := getID(c)
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}
	var req struct {
		Items []core.DispositionIn `json:"items"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "items"))
	}
	user := auth.GetUser(c)
	out, err := a.core.AddDispositions(id, user.ID, user.Username, req.Items)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// GetReviewStats (GET /api/campaigns/reviews/stats) -- the Accept-risk rate per rubric id.
func (a *App) GetReviewStats(c echo.Context) error {
	out, err := a.core.ReviewStats()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// PutStructureVerification (PUT /api/campaigns/structure-verifications) records a clean matrix read (D16).
func (a *App) PutStructureVerification(c echo.Context) error {
	var req struct {
		Fingerprint string          `json:"fingerprint"`
		TestID      string          `json:"test_id"`
		Components  json.RawMessage `json:"components"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "body"))
	}
	if !reFingerprint.MatchString(req.Fingerprint) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "fingerprint"))
	}
	if req.TestID == "" || len(req.TestID) > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "test_id"))
	}
	user := auth.GetUser(c)
	out, err := a.core.PutStructureVerification(req.Fingerprint, req.TestID, req.Components, user.Username)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// GetStructureVerification (GET /api/campaigns/structure-verifications/:fingerprint) -- {match, records}.
func (a *App) GetStructureVerification(c echo.Context) error {
	fp := c.Param("fingerprint")
	if !reFingerprint.MatchString(fp) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "fingerprint"))
	}
	match, all, err := a.core.GetStructureVerification(fp)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{struct {
		Match   *models.StructureVerification  `json:"match"`
		Records []models.StructureVerification `json:"records"`
	}{match, all}})
}

// reviewGate (D6) refuses a transition INTO running or scheduled (Start, Schedule, Resume) unless
// the newest COMPLETE review for the campaign's CURRENT bundle hash has verdict pass. A no-op while
// app.review_url is empty (the dark deploy) and for type=optin (D1.7b). HTTP-handler-only like the
// footer guard: evergreen ticking, internal restarts and test sends never reach it. Runs after the
// footer guard and before the DB write.
func (a *App) reviewGate(id int, next string) error {
	if next != models.CampaignStatusRunning && next != models.CampaignStatusScheduled {
		return nil
	}
	if !reviewEnabled() {
		return nil
	}
	cm, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}
	if cm.Type == models.CampaignTypeOptin {
		return nil
	}

	hash := core.CampaignBundleHash(cm)
	row, err := a.core.GetReviewByHash(id, hash)
	if err != nil {
		return err
	}
	refuse := func(reason string) error {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("campaigns.reviewRequired", "reason", reason))
	}
	if row == nil {
		state, err := a.core.GetLatestReview(cm)
		if err != nil {
			return err
		}
		switch {
		case state.Review == nil:
			return refuse(a.i18n.T("campaigns.reviewReasonNotInspected"))
		case state.Review.Status == models.ReviewStatusRunning:
			return refuse(a.i18n.T("campaigns.reviewReasonRunning"))
		case state.Review.BundleHash != hash:
			return refuse(a.i18n.T("campaigns.reviewReasonEdited"))
		default:
			return refuse(a.i18n.T("campaigns.reviewReasonNotInspected"))
		}
	}

	ds, err := a.core.GetReviewDispositions(id)
	if err != nil {
		return err
	}
	v := review.Verdict(json.RawMessage(row.Report), ds)
	if v.Verdict != review.VerdictPass {
		return refuse(a.i18n.Ts("campaigns.reviewReasonBlocked", "n", itoaInt(len(v.Blockers))))
	}
	return nil
}

// reviewGuardOnEdit (D6's save-path rule, the footer guard's own hole): a SCHEDULED campaign is
// claimed by the scheduler by SQL with no further transition, so while the gate is on a save that
// changes its bundle hash is refused -- Unschedule -> edit -> Inspect -> Schedule is the path. An
// unchanged-hash save passes. `paused` needs no rule: Resume is a status PUT.
func (a *App) reviewGuardOnEdit(stored models.Campaign, storedHash string, incoming campReq) error {
	if !reviewEnabled() || stored.Status != models.CampaignStatusScheduled || stored.Type == models.CampaignTypeOptin {
		return nil
	}
	next := incoming.Campaign
	if next.Attribs == nil {
		// update-campaign COALESCEs a NULL attribs bind: the stored map is what would be kept.
		next.Attribs = stored.Attribs
	}
	if review.BundleHash(next, incoming.ListIDs) != storedHash {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.reviewUnscheduleFirst"))
	}
	return nil
}
