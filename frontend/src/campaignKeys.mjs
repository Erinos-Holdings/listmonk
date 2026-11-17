// Fork (integrations ATTRIBS-CASE-SPEC D1) -- the one camelCase predicate for campaign reads and
// writes (api/index.js). utils.camelKeys renames every key whose path the predicate passes; a
// campaign's `headers`, and every key strictly UNDER `attribs` and `archive_meta`, are stored data
// whose names the server and the review Lambda read verbatim (`attribs.alt_generated`,
// `attribs.journey_seed`), so they are kept. The containers themselves still camelCase
// (`archive_meta` -> `archiveMeta` is what the page reads). `prefix` is the path to the campaign
// object: '' for a single campaign, '.results.*' for the Campaigns page's list.
export const campaignCamel = (prefix) => {
  const headers = `${prefix}.headers`;
  const under = [`${prefix}.attribs.`, `${prefix}.archive_meta.`];
  return (keyPath) => !(
    keyPath === headers
    || keyPath.startsWith(`${headers}.`)
    || under.some((p) => keyPath.startsWith(p))
  );
};

export default campaignCamel;
