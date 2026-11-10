package main

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D6/D7: the standard-journey-list
// lock. A list is a STANDARD JOURNEY LIST when its STORED join_trigger kind is first_order,
// no_purchase or post_purchase, or it carries a reengage: tag (the sunset queues -- minted only at
// create, never removable). isJourneyLockedList is the ONE definition; every list response carries
// its result as journey_locked so the form mirrors the server instead of re-deriving it. SKU lists
// are never locked.
//
// A caller without lists:journeys (Super Admin and the three system API roles hold it, v6.2.36)
// may not edit, delete or hand-manage the membership of a locked list, set a standard kind on any
// list, or create a list carrying a reengage: tag. The whole request is refused (403
// lists.journeyLocked) before anything is applied, never a partial apply. The lock never blocks a
// suppression (unsubscribe, hold, blocklist, brand unsubscribe, subscriber deletion), the Active
// switch, a write-back (sunset, trigger-sync) or a brand re-projection.

import (
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// isStandardTriggerKind reports a join_trigger kind that makes a list a standard journey list.
func isStandardTriggerKind(kind string) bool {
	return kind == joinTriggerFirstOrder || isStepTriggerKind(kind)
}

// isJourneyLockedList is the ONE definition of a standard journey list (D6).
func isJourneyLockedList(l models.List) bool {
	if isStandardTriggerKind(joinTriggerKind(l.JoinTrigger)) {
		return true
	}
	for _, t := range l.Tags {
		if strings.HasPrefix(strings.TrimSpace(t), reengageTagPrefix) {
			return true
		}
	}
	return false
}

// markJourneyLocked sets journey_locked on list responses (I14).
func markJourneyLocked(lists []models.List) {
	for i := range lists {
		lists[i].JourneyLocked = isJourneyLockedList(lists[i])
	}
}

// canManageJourneys reports whether the caller holds lists:journeys.
func canManageJourneys(user auth.User) bool {
	return user.HasPerm(auth.PermListJourneys)
}

// journeyLockedErr is the refusal naming the list.
func (a *App) journeyLockedErr(name string) error {
	return echo.NewHTTPError(http.StatusForbidden, a.i18n.Ts("lists.journeyLocked", "name", name))
}

// refuseJourneyLocked refuses a caller without lists:journeys when any of the lists is locked. Run
// after the caller's own list-permission check.
func (a *App) refuseJourneyLocked(user auth.User, ids ...int) error {
	if canManageJourneys(user) || len(ids) == 0 {
		return nil
	}
	lists, err := a.core.GetListsByOptin(ids, "")
	if err != nil {
		return err
	}
	for _, l := range lists {
		if isJourneyLockedList(l) {
			return a.journeyLockedErr(l.Name)
		}
	}
	return nil
}

// refuseJourneyMembershipChange (D7 M2/M3) refuses a caller without lists:journeys when a
// subscriber update whose lists REPLACE the subscriptions would add or drop a locked list.
// resulting is the request's (permission-filtered) lists; permitted is the set the update's
// delete is confined to (empty = every list, the update-subscriber-with-lists convention). A
// round-trip keeping the person's locked memberships unchanged passes.
func (a *App) refuseJourneyMembershipChange(user auth.User, subID int, resulting, permitted []int) error {
	if canManageJourneys(user) {
		return nil
	}
	current, err := a.core.GetSubscriberListIDs(subID)
	if err != nil {
		return err
	}
	in := func(id int, set []int) bool {
		for _, x := range set {
			if x == id {
				return true
			}
		}
		return false
	}
	var touched []int
	for _, id := range resulting {
		if !in(id, current) {
			touched = append(touched, id) // an add
		}
	}
	for _, id := range current {
		if !in(id, resulting) && (len(permitted) == 0 || in(id, permitted)) {
			touched = append(touched, id) // a drop
		}
	}
	return a.refuseJourneyLocked(user, touched...)
}
