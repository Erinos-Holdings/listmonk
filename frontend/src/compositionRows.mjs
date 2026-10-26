// Fork (blocklist composition, integrations BLOCKLIST-COMPOSITION-SPEC D5/I5). Pure shaping for
// the "Blocklist composition" card on the SES pane (ses document) and the Brands detail (brand
// document). The integrations BrandHealth Lambda computes every bucket, count, domain and code
// (lib/blocklist-composition.ts owns the rules and the vocabulary); this module only labels the
// stored rows and formats the share of the stored total -- the one arithmetic the card does.
// Tested by compositionRows.test.mjs (node --test).

const defaultFormat = new Intl.NumberFormat('en-US').format;

// The stored bucket key -> its i18n id under `composition.bucket.` (D3 order). A key outside this
// map renders as its raw key (fail-visible, never dropped -- I5).
export const BUCKET_IDS = {
  kickbox: 'kickbox',
  complaint: 'complaint',
  'hard:block': 'hardBlock',
  'hard:ses-suppressed': 'hardSesSuppressed',
  'hard:address': 'hardAddress',
  'hard:other': 'hardOther',
  'soft-only': 'softOnly',
  manual: 'manual',
};

const isObj = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);
const num = (v) => (Number.isFinite(v) ? v : 0);

// compositionState returns 'absent' (no field, or not an object), 'skipped' (the volume guard --
// the card shows the note and the stored total, no rows) or 'ok'.
export function compositionState(composition) {
  if (!isObj(composition)) {
    return 'absent';
  }
  return composition.skipped ? 'skipped' : 'ok';
}

// share returns count / total as a percentage with one decimal, or null when total is not a
// finite number above 0.
export function share(count, total) {
  if (!(Number.isFinite(total) && total > 0)) {
    return null;
  }
  return `${((num(count) / total) * 100).toFixed(1)}%`;
}

// compositionRows returns one row per stored bucket, in stored order:
// { key, labelKey, helpKey, label, count, countText, share, domains, codes, hasDetail }.
// labelKey/helpKey are null for an unknown key, whose `label` is the raw key. No rows for an
// absent or skipped composition.
export function compositionRows(composition, fmt = defaultFormat) {
  if (compositionState(composition) !== 'ok' || !Array.isArray(composition.buckets)) {
    return [];
  }
  const total = num(composition.total);
  return composition.buckets.filter(isObj).map((b) => {
    const key = String(b.key ?? '');
    const id = Object.prototype.hasOwnProperty.call(BUCKET_IDS, key) ? BUCKET_IDS[key] : null;
    const count = num(b.count);
    const domains = (Array.isArray(b.domains) ? b.domains : []).filter(isObj)
      .map((d) => ({ domain: String(d.domain ?? ''), n: num(d.n), nText: fmt(num(d.n)) }));
    const codes = (Array.isArray(b.codes) ? b.codes : []).filter(isObj)
      .map((c) => ({ code: String(c.code ?? ''), n: num(c.n), nText: fmt(num(c.n)) }));
    return {
      key,
      labelKey: id ? `composition.bucket.${id}` : null,
      helpKey: id ? `composition.bucket.${id}Help` : null,
      label: key,
      count,
      countText: fmt(count),
      share: share(count, total),
      domains,
      codes,
      hasDetail: domains.length > 0 || codes.length > 0,
    };
  });
}
