package main

// Fork (integrations SAFE-ACTION-BODY-SPEC D2) -- POST /api/campaigns/actions: the report of
// every template action in a posted body that is not a plain value (models.BodyActions, the
// rule's one home). Report-only: it reads no campaign, runs no query, writes nothing, logs no
// body and executes no template. Behind exactly GET /api/campaigns/:id's permissions.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// maxBodyActionsBytes caps the request. The fork has no size limit on a campaign save, so a
// larger body can exist: over the cap the answer is a 200 report with one `error` finding, so
// that campaign's Inspect ends unverified, never failed (SAFE-ACTION-BODY-SPEC R8).
const maxBodyActionsBytes = 4 << 20

type bodyActionsReq struct {
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

// decodeBodyActionsReq reads the capped request. over reports a body over the cap; err is a
// body that is not JSON.
func decodeBodyActionsReq(w http.ResponseWriter, r io.ReadCloser) (req bodyActionsReq, over bool, err error) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r, maxBodyActionsBytes))
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return req, true, nil
		}
		return req, false, err
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return req, false, err
	}
	return req, false, nil
}

// PostCampaignActions answers {"data": <models.BodyActionReport>} for the posted body.
func (a *App) PostCampaignActions(c echo.Context) error {
	req, over, err := decodeBodyActionsReq(c.Response(), c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if over {
		return c.JSON(http.StatusOK, okResp{models.BodyActionsFailure("body over 4 MiB")})
	}
	// A local zero-value campaign carrying the two fields: TemplateFuncs only builds the map;
	// its closures run at execute time, which never happens here.
	camp := models.Campaign{Body: req.Body, ContentType: req.ContentType}
	return c.JSON(http.StatusOK, okResp{models.BodyActions(req.Body, req.ContentType, a.manager.TemplateFuncs(&camp))})
}
