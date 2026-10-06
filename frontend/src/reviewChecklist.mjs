// Fork (review navigation, integrations REVIEW-NAVIGATION-SPEC §4.6) -- the Inspect window's live
// checklist, pure half: the five rows from the review Lambda's cumulative progress (§3.2:
// `checks`, then the D `items`, then `screenshots`, then `ai`) and the report; the section rule the
// window's lists and the rows' counts share; and the read-only early findings.
//
// The progress contract is additive (I14): an older Lambda's `{stage, at}` renders the rows with
// their pending labels and the final counts -- never an error. Nothing here can stage a
// disposition: the early list is built from `progress.items` and every entry is `decidable: false`
// (I5); the window's `entries` reads the report only.
import { isStructureKey } from './structureBrief.mjs'; // eslint-disable-line import/extensions

export const STAGES = ['reading', 'deterministic', 'screenshots', 'ai', 'writing'];

// The window's section rule for one finding of one item: a D fail is a blocker, any other D
// verdict advisory; an A critical a blocker, an A high needs acknowledgement, the rest advisory.
export function sectionOf(item, finding) {
  if (item.tier === 'D') {
    return item.verdict === 'fail' ? 'blockers' : 'advisory';
  }
  if (finding.severity === 'critical') return 'blockers';
  if (finding.severity === 'high') return 'acknowledge';
  return 'advisory';
}

// Every open finding of a report's (or a progress's) items with its section: pass and n/a items
// are skipped, and the structure key (R#…, R) is excluded exactly as the window's `entries`
// excludes it -- it is decided in the Structure section only.
export function openEntries(items) {
  const out = [];
  (Array.isArray(items) ? items : []).forEach((item) => {
    if (!item || item.verdict === 'pass' || item.verdict === 'n/a') return;
    (item.findings || []).filter((f) => f && !isStructureKey(f.key)).forEach((finding) => {
      out.push({
        key: finding.key, item, finding, section: sectionOf(item, finding),
      });
    });
  });
  return out;
}

// { blockers, acknowledge, advisory } counts over `items`.
export function sectionCounts(items) {
  const n = { blockers: 0, acknowledge: 0, advisory: 0 };
  openEntries(items).forEach((e) => { n[e.section] += 1; });
  return n;
}

const dItems = (items) => (Array.isArray(items) ? items.filter((i) => i && i.tier === 'D') : []);

// The rows. `phase` is `running` (the current row spins), `complete` (every row done) or `halted`
// (failed/stale: the rows stay as at the last PATCH, nothing spins). `t(key, params)` is i18n.
// `sectionsOf` (default sectionCounts) is the window's own section rule.
export function checklist({
  progress, report, phase = 'running', sectionsOf = sectionCounts, t,
}) {
  const p = (progress && typeof progress === 'object') ? progress : {};
  const rep = phase === 'complete' && report && Array.isArray(report.items) ? report : null;
  const at = STAGES.indexOf(p.stage);
  const current = phase === 'complete' ? STAGES.length : Math.max(at, 0);
  const tr = (k, params) => t(`campaigns.review.checklist.${k}`, params);

  // The deterministic row: its counts from progress.items (the window's rule), else the report's D
  // items (a completed review of an older Lambda -- the same keys, I4).
  let ditems = null;
  if (Array.isArray(p.items)) {
    ditems = dItems(p.items);
  } else if (rep) {
    ditems = dItems(rep.items);
  }
  let checks = null;
  if (Number.isInteger(p.checks)) {
    checks = p.checks;
  } else if (rep) {
    checks = dItems(rep.items).length;
  }
  let shots = null;
  if (Number.isInteger(p.screenshots)) {
    shots = p.screenshots;
  } else if (rep && rep.provenance && Number.isInteger(rep.provenance.screenshots)) {
    shots = rep.provenance.screenshots;
  }
  let ai = null;
  if (p.ai && typeof p.ai === 'object' && Number.isInteger(p.ai.calls)) {
    ai = p.ai;
  } else if (rep && rep.ai && Array.isArray(rep.ai.calls)) {
    ai = { calls: rep.ai.calls.length, status: rep.ai.status };
  }

  const pending = {
    reading: tr('reading'),
    deterministic: checks !== null ? tr('deterministic', { checks }) : tr('checksUnknown'),
    screenshots: tr('screenshots'),
    ai: tr('ai'),
    writing: tr('writing'),
  };
  const done = {
    reading: tr('readingDone'),
    deterministic: null,
    screenshots: null,
    ai: null,
    writing: null,
  };
  if (ditems && checks !== null) {
    const c = sectionsOf(ditems);
    done.deterministic = tr('deterministicDone', { checks, b: c.blockers, a: c.advisory });
  }
  if (shots !== null) {
    done.screenshots = shots === 0 ? tr('screenshotsNone') : tr('screenshotsDone', { n: shots });
  }
  if (ai) {
    done.ai = ai.status === 'ok' ? tr('aiDone', { calls: ai.calls }) : tr('aiStatus', { status: ai.status });
  }
  if (rep) {
    const c = sectionsOf(rep.items);
    done.writing = tr('writingDone', { b: c.blockers, ack: c.acknowledge, adv: c.advisory });
  }

  return STAGES.map((key, i) => {
    let state = 'pending';
    if (i < current) state = 'done';
    else if (i === current) state = 'current';
    const label = state === 'done' ? (done[key] || pending[key]) : pending[key];
    return {
      key, state, checked: state === 'done', spinner: state === 'current' && phase === 'running', label,
    };
  });
}

// The read-only early findings: from the `screenshots` stage on, while the review runs, the D items
// in progress.items by section. Every entry is `decidable: false` -- no button renders and nothing
// can be staged against it. The report replaces them when it arrives (phase `complete` -> none).
export function earlyEntries({ progress, phase = 'running' }) {
  if (phase !== 'running' || !progress || !Array.isArray(progress.items)) return [];
  if (STAGES.indexOf(progress.stage) < STAGES.indexOf('screenshots')) return [];
  return openEntries(dItems(progress.items)).map((e) => ({ ...e, decidable: false }));
}

// ------------------------------------------------------------------ the auxiliary sections
// (polish pass, user 2026-10-06) Context, Structure and Provenance as collapsible sections with a
// status pill: the window's eye-catcher only -- nothing here decides, gates or stages anything.

const STATUS_RANK = { fail: 0, warn: 1, ok: 2 };
const PILL_TYPE = { ok: 'is-success', warn: 'is-warning', fail: 'is-danger' };

// A context line is a concern when its value is not `ok`, not `none` and not a number ("issues",
// "warn", "unknown", a list of running broadcasts …).
export function isContextConcern(value) {
  const v = String(value ?? '').trim().toLowerCase();
  if (v === 'ok' || v === 'none') return false;
  return !/^-?\d+(\.\d+)?$/.test(v);
}

// `structure` is structureBrief.mjs `structureOf(report)` (D4.2's coverage) or null. Returns the
// sections present in the report, ordered fail, then warn, then ok (Context, Structure, Provenance
// within a status): `{key, status, pill, pillType, open, highlight: Set<index>}`. `open` is
// `status !== 'ok'`. Context highlights its concern lines; Provenance the AI call lines whose
// status is not ok/cached.
export function auxSections({ report, structure, t }) {
  const rep = report && typeof report === 'object' ? report : null;
  if (!rep) return [];
  const tr = (k, p) => t(`campaigns.review.pill.${k}`, p);
  const out = [];
  const add = (key, status, pill, highlight = new Set()) => out.push({
    key, status, pill, pillType: PILL_TYPE[status], open: status !== 'ok', highlight,
  });

  const context = Array.isArray(rep.context) ? rep.context : [];
  if (context.length) {
    const hl = new Set();
    context.forEach((c, i) => { if (c && isContextConcern(c.value)) hl.add(i); });
    if (hl.size) add('context', 'warn', tr('attention', { n: hl.size }), hl);
    else add('context', 'ok', tr('passed'));
  }

  const cov = structure && structure.coverage;
  if (cov) {
    if (cov.verified) add('structure', 'ok', tr('passed'));
    else if (cov.plan && cov.plan.available === false) add('structure', 'warn', tr('unavailable'));
    else add('structure', 'fail', tr('unverified'));
  } else if (rep.structure) {
    add('structure', rep.structure.verified ? 'ok' : 'fail', rep.structure.verified ? tr('passed') : tr('unverified'));
  }

  const ai = rep.ai && typeof rep.ai === 'object' ? rep.ai : null;
  const calls = ai && Array.isArray(ai.calls) ? ai.calls : [];
  const hl = new Set();
  calls.forEach((c, i) => { if (!c || (c.status !== 'ok' && c.status !== 'cached')) hl.add(i); });
  if (ai && ai.status === 'unavailable') add('provenance', 'fail', tr('aiUnavailable'), hl);
  else if (ai && ai.status === 'partial') add('provenance', 'warn', tr('aiPartial'), hl);
  else add('provenance', 'ok', tr('passed'), hl);

  return out
    .map((s, i) => ({ s, i }))
    .sort((a, b) => STATUS_RANK[a.s.status] - STATUS_RANK[b.s.status] || a.i - b.i)
    .map(({ s }) => s);
}
