// Fork (campaign mode, integrations listmonk/CAMPAIGN-MODE-SPEC D3). The JS half of the campaign
// grouping's name key, re-exported by campaignGroups.mjs (campaignGroups.mjs::nameKey) and imported by
// naming.mjs::duplicateOf. Its own leaf module, importing nothing, because naming.mjs is imported by
// campaignGroups.mjs's own dependencies (campaignMatrix.mjs, langScope.mjs): defining it in
// campaignGroups.mjs would make naming.mjs part of an import cycle. Pinned with the server's
// core.NameKeySQL by testdata/campaign-groups.json (I1) -- change both and the fixture together.
//
// Every run of U+0020, U+0009, U+000A, U+000D, U+000C, U+000B, U+00A0 becomes one U+0020; one leading
// and one trailing U+0020 are removed; then toLowerCase. Nothing else changes (U+2003 is kept).
const RE_SPACE_RUN = /[ \t\n\r\f\v\xa0]+/g;

export function nameKey(s) {
  let k = (s === null || s === undefined ? '' : String(s)).replace(RE_SPACE_RUN, ' ');
  if (k.startsWith(' ')) k = k.slice(1);
  if (k.endsWith(' ')) k = k.slice(0, -1);
  return k.toLowerCase();
}

export default nameKey;
