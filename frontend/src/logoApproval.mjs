// Fork (approved brand logos, integrations LOGO-APPROVAL-SPEC D10). The Inspect window's D3.5
// approval card, pure half: which state a D3.5 item shows given the brand's row from GET
// /api/brands -- `approved` (one of the review's candidate files is already on the approved list:
// the item leaves Advisory and is listed under Passed), `card` (the first candidate on neither list:
// the Yes/No question in place of the todo), or `warn` (today's D3.5 card, unchanged). The review
// Lambda writes `logoCandidates` (each {url, location, n?}, `n` the editor's block number) and
// `brand` on the D3.5 item; the window prints `n`, never computes it. A missing row (a failed read)
// reads as empty lists. Cards decide nothing about Done: D3.5 is advisory in every state.
// No Vue, no store; tested by logoApproval.test.mjs (node --test).

export const LOGO_ITEM = 'D3.5';
export const LOGO_PERMISSION = 'brands:logos';

const strings = (v) => (Array.isArray(v) ? v.filter((s) => typeof s === 'string') : []);

// A brands row's two lists; null / undefined / an older fork's row without the columns -> empty.
export function brandLists(brandRow) {
  const row = brandRow && typeof brandRow === 'object' ? brandRow : {};
  return { approved: strings(row.logos_approved), rejected: strings(row.logos_rejected) };
}

// The brands row of an item's brand from the window's rows (the GET /api/brands array; null = none).
export function brandRowFor(item, rows) {
  const slug = item && typeof item.brand === 'string' ? item.brand : '';
  if (!slug || !Array.isArray(rows)) return null;
  return rows.find((r) => r && r.slug === slug) || null;
}

const candidatesOf = (item) => (item && Array.isArray(item.logoCandidates) ? item.logoCandidates : [])
  .filter((c) => c && typeof c.url === 'string' && c.url !== '');

// D10: the state of a D3.5 item against its brand row -- approved beats card; card is the first
// candidate on neither list; else warn. Any other item, or a D3.5 without candidates, is `warn`.
export function logoCardState(item, brandRow) {
  if (!item || item.id !== LOGO_ITEM || item.verdict !== 'warn') return { state: 'warn' };
  const cands = candidatesOf(item);
  const { approved, rejected } = brandLists(brandRow);
  const ok = cands.find((c) => approved.includes(c.url));
  if (ok) return { state: 'approved', url: ok.url };
  const next = cands.find((c) => !approved.includes(c.url) && !rejected.includes(c.url));
  if (next) return { state: 'card', candidate: next };
  return { state: 'warn' };
}

// The report's items as the window shows them: a D3.5 in the `approved` state becomes a pass carrying
// `approvedLogo` (no findings), so it leaves Advisory, the checklist's advisory count and the entries,
// and is listed under Passed. Every other item is returned as it is (the same object).
export function withApprovedLogo(items, rows) {
  return (Array.isArray(items) ? items : []).map((item) => {
    if (!item || item.id !== LOGO_ITEM) return item;
    const s = logoCardState(item, brandRowFor(item, rows));
    if (s.state !== 'approved') return item;
    return {
      ...item, verdict: 'pass', examined: 1, findings: [], approvedLogo: s.url,
    };
  });
}

// Whether an entry ({key, item, finding}) shows the approval card.
export function isLogoCard(entry, rows) {
  return !!(entry && entry.item) && logoCardState(entry.item, brandRowFor(entry.item, rows)).state === 'card';
}

// The window's Accepted split with the D10 exception (stage 2 F10): a D3.5 entry showing the card
// stays in the open sections whatever disposition its key carries (an earlier Accept risk or I'll fix
// it stays in the log). `split` is reviewChecklist.mjs splitAccepted. Order is preserved.
export function logoAwareSplit(entries, latest, split, rows) {
  const list = Array.isArray(entries) ? entries : [];
  const { open, accepted } = split(list.filter((e) => !isLogoCard(e, rows)), latest);
  const openSet = new Set(open);
  return { open: list.filter((e) => isLogoCard(e, rows) || openSet.has(e)), accepted };
}

// The card's question (an i18n call): "<url> — This section N image appears…"; a candidate without
// `n` drops "section N " (the NoBlock key). `t(key, params)` is the window's $t.
export function logoQuestion(candidate, t) {
  const c = candidate || {};
  return Number.isInteger(c.n)
    ? t('campaigns.review.logoQuestion', { url: c.url, n: c.n })
    : t('campaigns.review.logoQuestionNoBlock', { url: c.url });
}

// The action a card button posts: Yes approves, No rejects.
export const logoAction = (yes) => (yes ? 'approve' : 'reject');
