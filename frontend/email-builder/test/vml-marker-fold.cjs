// Shared helper (not a suite): folds the VML href MARKER form the builder emits since the
// click-tracking release —
//   {{ Safe "…href=\"" }}<span data-lm-vml-href="VALUE"></span>{{ Safe "\"…" }}
// — back into the single-payload form the older structural suites assert against,
//   {{ Safe "…href=\"VALUE\"…" }}
// by re-encoding VALUE exactly as makeSafeTemplate would have. Suites that care about the
// marker itself assert on the raw output (vml-href-marker.test.cjs).
function decodeEntities(v) {
  return v.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&#x27;/g, "'")
    .replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
}
function safeEncode(raw) {
  return raw.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
    .replace(/&/g, '\\x26').replace(/</g, '\\x3c').replace(/>/g, '\\x3e')
    .replace(/ /g, '\\x20').replace(/\t/g, '\\x09').replace(/\n/g, '\\x0a').replace(/\r/g, '\\x0d');
}
const MARKER = /" \}\}<span data-lm-vml-href="([^"]*)"><\/span>\{\{ Safe "/g;
function foldVmlMarkers(html) {
  return html.replace(MARKER, (_, v) => safeEncode(decodeEntities(v)));
}
module.exports = { foldVmlMarkers, decodeEntities, safeEncode };
