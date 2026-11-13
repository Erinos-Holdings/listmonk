package main

import (
	"net/http"

	"github.com/knadh/listmonk/internal/joinpage"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (join link) -- integrations listmonk/JOIN-LINK-SPEC. The {{ JoinURL }} route,
// /subscription/join/:campUUID/:subUUID, behind hasUUID + hasSub (the GET also noIndex). A former
// customer who never subscribed clicks a win-back campaign's Join link; the GET shows the brand
// and address and one Confirm form, the POST joins them to the brand's master list as a new
// subscriber. The policy (which page, the language, the brand name, the copy) is
// internal/joinpage; the plan and the write are the get-join-plan and join-brand queries.

// joinTpl is the data of join.html (confirm / already / cannot) and joined.html (the result).
type joinTpl struct {
	publicTpl
	State string
	Copy  joinpage.Copy
}

// JoinPage is the GET. It NEVER writes (D2, I1): it reads the plan and renders it -- no HEAD route
// exists (echo answers HEAD with 405), so a link scanner's GET or HEAD cannot join anyone.
func (a *App) JoinPage(c echo.Context) error {
	plan, err := a.core.GetJoinPlan(c.Param("subUUID"), c.Param("campUUID"))
	if err != nil {
		return a.joinError(c)
	}
	return a.renderJoin(c, plan, joinpage.State(plan.Outcome))
}

// JoinConfirm is the POST. Only a plan of `join` writes, through join-brand (one statement that
// re-evaluates the plan); a lost race (no row returned) re-reads the plan and renders that
// outcome (D7.3). A second Confirm finds `already` and writes nothing (D8).
func (a *App) JoinConfirm(c echo.Context) error {
	var (
		subUUID  = c.Param("subUUID")
		campUUID = c.Param("campUUID")
	)
	plan, err := a.core.GetJoinPlan(subUUID, campUUID)
	if err != nil {
		return a.joinError(c)
	}
	if plan.Outcome != models.JoinOutcomeJoin {
		return a.renderJoin(c, plan, joinpage.State(plan.Outcome))
	}

	res, err := a.core.JoinBrand(subUUID, campUUID)
	if err != nil {
		return a.joinError(c)
	}
	if res.Joined {
		return a.renderJoin(c, plan, joinpage.StateJoined)
	}

	again, err := a.core.GetJoinPlan(subUUID, campUUID)
	if err != nil {
		return a.joinError(c)
	}
	return a.renderJoin(c, again, joinpage.State(again.Outcome))
}

// renderJoin renders state for plan in the page language (D10), HTTP 200 for every outcome (D11).
func (a *App) renderJoin(c echo.Context, plan models.JoinPlan, state string) error {
	lang := joinpage.PageLang(plan.Lang, plan.SubLang)
	cp := joinpage.For(lang, joinpage.BrandName(plan.FromEmail, plan.Brand), plan.Email)

	out := joinTpl{State: state, Copy: cp}
	out.Lang = lang
	tpl := "join"
	switch state {
	case joinpage.StateJoined:
		tpl = "joined"
		out.Title = cp.DoneHeading
	case joinpage.StateConfirm:
		out.Title = cp.Heading
	case joinpage.StateAlready:
		out.Title = cp.DoneHeading
	}
	return c.Render(http.StatusOK, tpl, out)
}

// joinError is the generic error page (a read or write failure, never a refusal).
func (a *App) joinError(c echo.Context) error {
	return c.Render(http.StatusInternalServerError, tplMessage,
		makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
}
