// Fork (campaign lists, integrations CAMPAIGN-LISTS-SPEC D4, I7). The pure half of ListSelector's
// pick: in single mode (the campaign form -- a campaign targets exactly one list, refused 400
// campaigns.oneList on the server) a pick REPLACES the selection; otherwise (Subscriber form, bulk
// list, Import) it appends, as upstream does. Tested by listSelection.test.mjs.

// nextListSelection is the selection after `picked` is chosen from the dropdown.
export function nextListSelection(selected, picked, single) {
  if (single) return [picked];
  return [...(Array.isArray(selected) ? selected : []), picked];
}

export default nextListSelection;
