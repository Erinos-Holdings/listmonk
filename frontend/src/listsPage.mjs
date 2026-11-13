// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC D7, I9). The Lists page's "View system
// lists" toggle: default OFF, persisted per viewer through the prefs (key lists.showSystem). OFF
// sends hide_system=true, which the server answers with the listing minus its system_hidden lists
// (the query-lists predicate, the SQL twin of cmd isHiddenSystemList). Tested by listsPage.test.mjs.

export const SHOW_SYSTEM_PREF = 'lists.showSystem';

// showSystemLists is the toggle's value for a stored pref: true only for exactly true -- unset
// (null) and anything else is the default, off.
export function showSystemLists(prefValue) {
  return prefValue === true;
}

// systemListsParams is the Lists query parameters for the toggle: hide_system=true exactly when it
// is off; nothing when on (today's listing).
export function systemListsParams(showSystem) {
  return showSystemLists(showSystem) ? {} : { hide_system: true };
}
