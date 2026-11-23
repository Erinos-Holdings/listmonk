package core

// Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D2) -- the ONE home of the campaign grouping
// rule in SQL. Campaign mode collapses campaigns into reporting rows: a SET (the campaigns sharing
// group brand, code key and name key) and a SERIES (the sets sharing group brand and code key). The
// expressions below are interpolated at boot or at call time into every statement that groups --
// the Campaigns list's grouped statement (QueryCampaignGroups), the Analytics picker's grouped
// statement (cmd/init.go, QueryAnalyticsCampaignsGrouped) and get-campaign-matrix (its cbrand CTE
// and the set_key / series_key columns). No SQL file, view or migration carries a second copy; the
// JS half of the name key is frontend/src/campaignGroups.mjs::nameKey, pinned with this one by
// frontend/src/testdata/campaign-groups.json (I1). Nothing is stored (D4): keys are computed at
// read time.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// GroupBrandSQL is the group brand of the campaign whose id is idExpr: the ONE brand of its ACTIVE
// lists (list_brand_tag, 'unattributed' and NULL excluded), NULL with zero or several -- the Campaign
// Matrix's displayed brand and the grouping brand are this one expression (I9).
func GroupBrandSQL(idExpr string) string {
	return "(SELECT CASE WHEN COUNT(DISTINCT list_brand_tag(gl.tags)) = 1 THEN MIN(list_brand_tag(gl.tags)) END" +
		" FROM campaign_lists gcl JOIN lists gl ON (gl.id = gcl.list_id)" +
		" WHERE gcl.campaign_id = " + idExpr + " AND gl.status = 'active'" +
		" AND list_brand_tag(gl.tags) IS NOT NULL AND list_brand_tag(gl.tags) <> 'unattributed')"
}

// NameKeySQL is the name key of the text expression expr (D3): every run of U+0020, U+0009, U+000A,
// U+000D, U+000C, U+000B and U+00A0 becomes one U+0020 (TRANSLATE maps the six others to a space,
// then runs of spaces collapse), the one leading and one trailing space left are removed, then
// LOWER (the database's UTF-8 locale). No other character is touched (U+2003 is kept).
func NameKeySQL(expr string) string {
	return "LOWER(BTRIM(REGEXP_REPLACE(TRANSLATE(" + expr +
		", CHR(9) || CHR(10) || CHR(13) || CHR(12) || CHR(11) || CHR(160), '      '), ' +', ' ', 'g'), ' '))"
}

// CodeKeySQL is the code key of the campaigns row aliased alias: the name key of attribs.promo_code
// when that is a JSON string AND the campaign is not evergreen (an automation's Code forms no series,
// review R8), else NULL.
func CodeKeySQL(alias string) string {
	return "(CASE WHEN NOT " + alias + ".evergreen AND JSONB_TYPEOF(" + alias + ".attribs->'promo_code') = 'string' THEN " +
		NameKeySQL(alias+".attribs->>'promo_code'") + " END)"
}

// SetKeySQL is the opaque set key (D7) from the group brand, code key and name key expressions,
// joined with U+001F; NULL without a brand (a campaign with no group brand is never grouped).
func SetKeySQL(brand, code, name string) string {
	return "(CASE WHEN " + brand + " IS NOT NULL THEN " + brand + " || CHR(31) || COALESCE(" + code + ", '') || CHR(31) || " + name + " END)"
}

// SeriesKeySQL is the opaque series key (D7); NULL without a brand or a code.
func SeriesKeySQL(brand, code string) string {
	return "(CASE WHEN " + brand + " IS NOT NULL AND " + code + " IS NOT NULL THEN " + brand + " || CHR(31) || " + code + " END)"
}

// GroupedStmt (D5/D6) wraps a campaign statement's text -- its trailing ';' trimmed -- as the grouped
// statement: over the ids the inner statement returns it computes the keys, forms the top-level rows
// (series; sets outside a series; singletons), keeps -- when the hit switch is on -- only the
// top-level rows containing a hit id, orders them by orderSQL (over the columns of t: name,
// created_at / updated_at = the members' MAX, status = the members' MIN, maxid) and pages them. The
// inner statement keeps its own $1..$n; the wrapper's parameters follow it:
//
//	$n+1 hit ids, $n+2 the hit switch, $n+3 offset, $n+4 limit (< 1 = none).
//
// Each row is kind (series | set | campaign), brand, code (as stored on the newest member), step
// (when every member agrees), name (the newest member's -- max created_at, then max id), ids (newest
// first), sets (a series' sets as JSON -- name, step, ids -- by step NULL last, then newest; NULL
// otherwise) and total (the top-level row count, COUNT(*) OVER ()).
func GroupedStmt(inner string, n int, orderSQL string) string {
	in := strings.TrimRight(strings.TrimSpace(inner), "; \n\t")
	brand := GroupBrandSQL("c.id")
	code := CodeKeySQL("c")
	name := NameKeySQL("c.name")
	newest := "ORDER BY k.created_at DESC, k.id DESC"
	return fmt.Sprintf(`WITH b AS (%[1]s),
m AS (
    SELECT c.id, c.name, c.status, c.created_at, c.updated_at,
        CASE WHEN JSONB_TYPEOF(c.attribs->'promo_code') = 'string' THEN c.attribs->>'promo_code' END AS code_raw,
        CASE WHEN c.attribs->'promo_step' = '1'::JSONB THEN 1 WHEN c.attribs->'promo_step' = '2'::JSONB THEN 2
            WHEN c.attribs->'promo_step' = '3'::JSONB THEN 3 END AS step,
        %[2]s AS gbrand, %[3]s AS gcode, %[4]s AS gname
    FROM campaigns c WHERE c.id IN (SELECT id FROM b)
),
k AS (
    SELECT m.*,
        CASE WHEN m.gbrand IS NULL THEN 'c' || m.id::TEXT
            WHEN m.gcode IS NOT NULL THEN 's' || %[5]s
            ELSE 'e' || %[6]s END AS tkey,
        CASE WHEN m.gbrand IS NULL THEN 'c' || m.id::TEXT ELSE %[6]s END AS skey
    FROM m
),
sets AS (
    SELECT k.tkey, k.skey,
        (ARRAY_AGG(k.name %[7]s))[1] AS name,
        CASE WHEN COUNT(k.step) = COUNT(*) AND COUNT(DISTINCT k.step) = 1 THEN MIN(k.step) END AS step,
        ARRAY_AGG(k.id %[7]s) AS ids,
        MAX(k.created_at) AS newest, MAX(k.id) AS maxid
    FROM k GROUP BY k.tkey, k.skey
),
t AS (
    SELECT k.tkey,
        CASE WHEN LEFT(k.tkey, 1) = 's' THEN 'series' WHEN COUNT(*) > 1 THEN 'set' ELSE 'campaign' END AS kind,
        MIN(k.gbrand) AS brand,
        (ARRAY_AGG(k.code_raw %[7]s))[1] AS code,
        CASE WHEN COUNT(k.step) = COUNT(*) AND COUNT(DISTINCT k.step) = 1 THEN MIN(k.step) END AS step,
        (ARRAY_AGG(k.name %[7]s))[1] AS name,
        ARRAY_AGG(k.id %[7]s) AS ids,
        MAX(k.created_at) AS created_at, MAX(k.updated_at) AS updated_at, MIN(k.status) AS status, MAX(k.id) AS maxid,
        BOOL_OR(k.id = ANY($%[8]d::INT[])) AS hit
    FROM k GROUP BY k.tkey
)
SELECT t.kind, t.brand, t.code, t.step, t.name, t.ids,
    CASE WHEN t.kind = 'series' THEN (
        SELECT JSON_AGG(JSON_BUILD_OBJECT('name', s.name, 'step', s.step, 'ids', s.ids)
            ORDER BY s.step ASC NULLS LAST, s.newest DESC, s.maxid DESC)
        FROM sets s WHERE s.tkey = t.tkey) END AS sets,
    COUNT(*) OVER () AS total
FROM t
WHERE (NOT $%[9]d::BOOLEAN OR t.hit)
ORDER BY %[10]s
OFFSET $%[11]d::INT LIMIT (CASE WHEN $%[12]d::INT < 1 THEN NULL ELSE $%[12]d::INT END)`,
		in, brand, code, name, SeriesKeySQL("m.gbrand", "m.gcode"), SetKeySQL("m.gbrand", "m.gcode", "m.gname"),
		newest, n+1, n+2, orderSQL, n+3, n+4)
}

// groupOrderSQL is the grouped list's ORDER BY (D5) for the list's whitelisted sort field and
// direction (already normalised by makeSearchQuery's rules): name -> the name key of the row's name;
// created_at / updated_at -> the members' MAX; status -> the members' MIN in the enum's declaration
// order; ties -> the row's max member id, in the same direction.
func groupOrderSQL(orderBy, order string) string {
	if !strSliceContains(orderBy, campQuerySortFields) {
		orderBy = "created_at"
	}
	if order != SortAsc && order != SortDesc {
		order = SortDesc
	}
	expr := "t." + orderBy
	if orderBy == "name" {
		expr = NameKeySQL("t.name")
	}
	return expr + " " + order + ", t.maxid " + order
}

// AnalyticsGroupedOrderSQL is the picker's fixed top-level order (D6): newest first, the picker's
// own order (created_at DESC, id DESC) over the members' MAX.
const AnalyticsGroupedOrderSQL = "t.created_at DESC, t.maxid DESC"

// QueryCampaignGroups (fork, campaign mode, integrations CAMPAIGN-MODE-SPEC D5) is the grouped
// Campaigns list: (1) the hits -- whenever searchStr is non-empty, the ids query-campaigns returns
// with every filter and the search, no paging (none = no rows); (2) the groups -- the loaded
// query-campaigns text wrapped by GroupedStmt (the CountCampaignStatuses precedent: one predicate
// home), bound with every filter, an EMPTY search, offset 0 and no limit, keeping the top-level
// rows that contain a hit and paging top-level rows; (3) the members -- QueryCampaigns with ids =
// the union of member ids, the empty search and every other filter incl. status, no paging, so each
// member is exactly the ungrouped list's row. Returns the top-level rows (members newest first) and
// the top-level total.
func (c *Core) QueryCampaignGroups(searchStr string, statuses, tags []string, orderBy, order string, getAll bool, permittedLists []int, evergreen *bool, listIDs, ids []int, lang string, offset, limit int) ([]models.CampaignGroup, int, error) {
	hitOn := strings.TrimSpace(searchStr) != ""
	hits := []int{}
	if hitOn {
		queryStr, stmt := makeSearchQuery(searchStr, "created_at", SortDesc, c.q.QueryCampaigns, campQuerySortFields)
		if err := c.db.Select(&hits, IDsStmt(stmt), queryCampaignsArgs(statuses, tags, queryStr, getAll, permittedLists, evergreen, listIDs, ids, lang, 0, 0)...); err != nil {
			c.log.Printf("error fetching campaign group hits: %v", err)
			return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
		}
		if len(hits) == 0 {
			return []models.CampaignGroup{}, 0, nil
		}
	}

	_, stmt := makeSearchQuery("", "created_at", SortDesc, c.q.QueryCampaigns, campQuerySortFields)
	stmt = GroupedStmt(stmt, 12, groupOrderSQL(orderBy, order))
	args := append(queryCampaignsArgs(statuses, tags, "", getAll, permittedLists, evergreen, listIDs, ids, lang, 0, 0),
		pq.Array(hits), hitOn, offset, limit)

	var groups []models.CampaignGroup
	if err := c.db.Select(&groups, stmt, args...); err != nil {
		c.log.Printf("error fetching campaign groups: %v", err)
		return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}
	if len(groups) == 0 {
		return []models.CampaignGroup{}, 0, nil
	}

	union := []int{}
	for _, g := range groups {
		for _, id := range g.IDs {
			union = append(union, int(id))
		}
	}
	members, _, err := c.QueryCampaigns("", statuses, tags, "created_at", SortDesc, getAll, permittedLists, evergreen, listIDs, union, lang, 0, 0)
	if err != nil {
		return nil, 0, err
	}
	byID := make(map[int]models.Campaign, len(members))
	for _, m := range members {
		byID[m.ID] = m
	}

	for i := range groups {
		if err := groups[i].ParseSets(); err != nil {
			c.log.Printf("error reading campaign group sets: %v", err)
			return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
				c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", err.Error()))
		}
		groups[i].Campaigns = models.Campaigns{}
		for _, id := range groups[i].IDs {
			if m, ok := byID[int(id)]; ok {
				groups[i].Campaigns = append(groups[i].Campaigns, m)
			}
		}
	}

	return groups, groups[0].Total, nil
}

// IDsStmt wraps a campaign statement's text (its trailing ';' trimmed) as its id set, so the planner
// drops every output but id: the grouped list's hit step and (cmd/init.go,
// QueryAnalyticsCampaignIDs) the grouped picker's (review S2).
func IDsStmt(listStmt string) string {
	return "SELECT id FROM (" + strings.TrimRight(strings.TrimSpace(listStmt), "; \n\t") + ") q"
}

// QueryAnalyticsCampaignGroups (fork, campaign mode, integrations CAMPAIGN-MODE-SPEC D6) is the
// picker's grouped search: the three steps of QueryCampaignGroups over query-analytics-campaigns --
// the hits with the search (ids only, QueryAnalyticsCampaignIDs), the groups (QueryAnalyticsCampaignsGrouped, prepared at boot) over the
// empty search, BOTH with $7 NULL (no hit cap, so an older matching member is never cut), and only
// limit (per_page) limiting top-level rows; then the members' picker rows by id. Scoping (the
// permission lists, started-only, the brand scope) is the inner statement's, unchanged.
func (c *Core) QueryAnalyticsCampaignGroups(searchStr string, allCampaigns bool, permittedLists []int, startedOnly bool, limit int, scopeLists []int) ([]models.AnalyticsCampaignGroup, error) {
	if permittedLists == nil {
		permittedLists = []int{}
	}
	if scopeLists == nil {
		scopeLists = []int{}
	}
	fail := func(err error) error {
		c.log.Printf("error fetching analytics campaign groups: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}

	hitOn := strings.TrimSpace(searchStr) != ""
	hits := []int{}
	if hitOn {
		// The ids only (QueryAnalyticsCampaignIDs, review S2): no per-row skip counts for every match.
		if err := c.q.QueryAnalyticsCampaignIDs.Select(&hits, pq.Array([]int{}), makeSearchString(searchStr), analyticsIDMatch(searchStr),
			allCampaigns, pq.Array(permittedLists), startedOnly, nil, pq.Array(scopeLists)); err != nil {
			return nil, fail(err)
		}
		if len(hits) == 0 {
			return []models.AnalyticsCampaignGroup{}, nil
		}
	}

	var groups []models.AnalyticsCampaignGroup
	if err := c.q.QueryAnalyticsCampaignsGrouped.Select(&groups, pq.Array([]int{}), "", 0,
		allCampaigns, pq.Array(permittedLists), startedOnly, nil, pq.Array(scopeLists),
		pq.Array(hits), hitOn, 0, limit); err != nil {
		return nil, fail(err)
	}
	if len(groups) == 0 {
		return []models.AnalyticsCampaignGroup{}, nil
	}

	union := []int{}
	for _, g := range groups {
		for _, id := range g.IDs {
			union = append(union, int(id))
		}
	}
	members, err := c.QueryAnalyticsCampaigns(union, "", allCampaigns, permittedLists, startedOnly, 0, scopeLists)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]models.AnalyticsCampaign, len(members))
	for _, m := range members {
		byID[m.ID] = m
	}

	for i := range groups {
		if err := groups[i].ParseSets(); err != nil {
			return nil, fail(err)
		}
		groups[i].Campaigns = []models.AnalyticsCampaign{}
		for _, id := range groups[i].IDs {
			if m, ok := byID[int(id)]; ok {
				groups[i].Campaigns = append(groups[i].Campaigns, m)
			}
		}
		groups[i].Count = len(groups[i].Campaigns)
	}

	return groups, nil
}
