package core

import (
	"net/http"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type listType struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
	Type string `json:"type"`
}

// GetLists gets all lists optionally filtered by type and status.
func (c *Core) GetLists(typ, status string, getAll bool, permittedIDs []int) ([]models.List, error) {
	out := []models.List{}

	if err := c.q.GetLists.Select(&out, typ, status, "id", getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	// Replace null tags.
	for i, l := range out {
		if l.Tags == nil {
			out[i].Tags = []string{}
		}

		// Total counts.
		for _, c := range l.SubscriberCounts {
			out[i].SubscriberCount += c
		}
	}

	return out, nil
}

// QueryLists gets multiple lists based on multiple query params. Along with the  paginated and sliced
// results, the total number of lists in the DB is returned. Fork (global brand, GLOBAL-BRAND-SPEC
// D6) -- noBrand keeps only the lists with no brand tag.
func (c *Core) QueryLists(searchStr, typ, optin, status string, tags []string, orderBy, order string, getAll bool, permittedIDs []int, noBrand bool, offset, limit int) ([]models.List, int, error) {
	_ = c.refreshCache(matListSubStats, false)

	if tags == nil {
		tags = []string{}
	}

	var (
		out            = []models.List{}
		queryStr, stmt = makeSearchQuery(searchStr, orderBy, order, c.q.QueryLists, listQuerySortFields)
	)
	if err := c.db.Select(&out, stmt, 0, "", queryStr, typ, optin, status, pq.StringArray(tags), getAll, pq.Array(permittedIDs), offset, limit, noBrand); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total

		// Replace null tags.
		for i, l := range out {
			if l.Tags == nil {
				out[i].Tags = []string{}
			}
		}
	}

	return out, total, nil
}

// GetList gets a list by its ID or UUID.
func (c *Core) GetList(id int, uuid string) (models.List, error) {
	var uu any
	if uuid != "" {
		uu = uuid
	}

	var res []models.List
	queryStr, stmt := makeSearchQuery("", "", "", c.q.QueryLists, nil)
	if err := c.db.Select(&res, stmt, id, uu, queryStr, "", "", "", pq.StringArray{}, true, nil, 0, 1, false); err != nil {
		c.log.Printf("error fetching lists: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}

	if len(res) == 0 {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	out := res[0]
	if out.Tags == nil {
		out.Tags = []string{}
	}
	// Total counts.
	for _, c := range out.SubscriberCounts {
		out.SubscriberCount += c
	}

	return out, nil
}

// GetListsByOptin returns lists by optin type.
func (c *Core) GetListsByOptin(ids []int, optinType string) ([]models.List, error) {
	out := []models.List{}
	if err := c.q.GetListsByOptin.Select(&out, optinType, pq.Array(ids), nil); err != nil {
		c.log.Printf("error fetching lists for opt-in: %s", pqErrMsg(err))
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetListTypes returns lists by their IDs or UUIDs.
// If ids is given, then the map returned has the list IDs as keys,
// otherwise, they have UUIDs as the keys.
// Note: This is a really weird and awkward API. Ideally, Go Generics
// should've somehow supported generic struct methods.
func (c *Core) GetListTypes(ids []int, uuids []string) (map[any]string, error) {
	res := []listType{}

	out := map[any]string{}
	if err := c.q.GetListTypes.Select(&res, pq.Array(ids), pq.StringArray(uuids)); err != nil {
		c.log.Printf("error fetching list types: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	isIDs := ids != nil
	for _, r := range res {
		if isIDs {
			out[r.ID] = r.Type
		} else {
			out[r.UUID] = r.Type
		}
	}

	return out, nil
}

// CreateList creates a new list.
func (c *Core) CreateList(l models.List) (models.List, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}

	if l.Type == "" {
		l.Type = models.ListTypePrivate
	}
	if l.Optin == "" {
		l.Optin = models.ListOptinSingle
	}
	if l.Status == "" {
		l.Status = models.ListStatusActive
	}

	// Insert and read ID.
	var newID int
	l.UUID = uu.String()
	if err := c.q.CreateList.Get(&newID, l.UUID, l.Name, l.Type, l.Optin, l.Status, pq.StringArray(normalizeListTags(l.Tags)), l.Description); err != nil {
		c.log.Printf("error creating list: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	return c.GetList(newID, "")
}

// UpdateList updates a given list. l.Tags is stored as given (the caller has already added the
// brand projection when the request named a brand); keepReserved (the request carried no brand)
// keeps the list's current brand:/from:/site: tags, merged in the one update-list statement
// (BRAND-PICKER-SPEC D2).
func (c *Core) UpdateList(id int, l models.List, keepReserved bool) (models.List, error) {
	res, err := c.q.UpdateList.Exec(id, l.Name, l.Type, l.Optin, l.Status, pq.StringArray(normalizeListTags(l.Tags)), l.Description, keepReserved)
	if err != nil {
		c.log.Printf("error updating list: %v", err)
		return models.List{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.list}", "error", pqErrMsg(err)))
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.list}"))
	}

	return c.GetList(id, "")
}

// DeleteList deletes a list.
func (c *Core) DeleteList(id int) error {
	return c.DeleteLists([]int{id}, "", true, nil)
}

// DeleteLists deletes multiple lists.
func (c *Core) DeleteLists(ids []int, query string, getAll bool, permittedIDs []int) error {
	var queryStr string

	if len(ids) > 0 {
		queryStr = ""
	} else {
		queryStr = makeSearchString(query)
	}

	if _, err := c.q.DeleteLists.Exec(pq.Array(ids), queryStr, getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error deleting lists: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	return nil
}

// Fork (BRAND-PICKER-SPEC D4) -- the locked lists (models.LockedListNames).

type lockedList struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// LockedListsOf returns the names of the given lists that are locked, in id order.
func (c *Core) LockedListsOf(ids []int) ([]string, error) {
	var res []lockedList
	if err := c.q.GetLockedLists.Select(&res, pq.Array(ids), pq.StringArray(models.LockedListNames)); err != nil {
		c.log.Printf("error reading locked lists: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	out := make([]string, 0, len(res))
	for _, r := range res {
		out = append(out, r.Name)
	}
	return out, nil
}

// CountListsByName returns how many lists carry exactly this name.
func (c *Core) CountListsByName(name string) (int, error) {
	var n int
	if err := c.q.CountListsByName.Get(&n, name); err != nil {
		c.log.Printf("error counting lists: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	return n, nil
}

// ListsMatchingDeleteQuery returns the ids and names a by-query DeleteLists would remove (the
// delete-lists predicate exactly), so the caller can refuse it whole when one is locked.
func (c *Core) ListsMatchingDeleteQuery(query string, getAll bool, permittedIDs []int) ([]int, []string, error) {
	var res []lockedList
	if err := c.q.GetDeleteListsByQ.Select(&res, makeSearchString(query), getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error reading lists to delete: %v", err)
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	ids := make([]int, 0, len(res))
	names := make([]string, 0, len(res))
	for _, r := range res {
		ids = append(ids, r.ID)
		names = append(names, r.Name)
	}
	return ids, names, nil
}
