// Fork (inspect scope, integrations INSPECT-SCOPE-SPEC §2.5/§2.8) -- the Structure section's pure
// half: which buttons a user sees (by the SERVER profile's campaigns:review_structure, never a role
// name), which keys are structure keys, and the copyable structure brief both Notify buttons open.
// Client-side only: nothing here (or in the buttons) sends anything -- the brief is built from the
// review report the window already holds and the user copies it.

export const STRUCTURE_PERMISSION = 'campaigns:review_structure';

// D4.2's structure finding key (R#<hash>, S14) or the legacy pseudo-key R. The generic Blockers
// list and its Accept-risk actions exclude these; the Structure section's Acknowledge is the only
// way to accept one.
export function isStructureKey(key) {
  const k = String(key || '');
  return k === 'R' || k.startsWith('R#');
}

// Permission -> buttons (S8). With the permission: Notify Claude + Acknowledge + the CLI lines.
// Without: Notify Robbie only.
export function structureButtons(canReviewStructure) {
  if (canReviewStructure) {
    return {
      notify: 'claude', acknowledge: true, showCli: true,
    };
  }
  return {
    notify: 'robbie', acknowledge: false, showCli: false,
  };
}

// The D4.2 item, its coverage object and its one finding -- or null for a report without coverage
// (STRUCTURE_GATE off, older reports), which renders the legacy Structure section.
export function structureOf(report) {
  const item = ((report && report.items) || []).find((i) => i && i.id === 'D4.2');
  if (!item || !item.coverage || typeof item.coverage !== 'object') {
    return null;
  }
  return { item, coverage: item.coverage, finding: (item.findings || [])[0] || null };
}

const scopeLabel = (s) => (s === 'dark' ? 'dark only' : 'light + dark');
const dateOf = (s) => String(s || '').slice(0, 10);

function clientList(ids, labels) {
  return (ids || []).map((id) => `    - ${(labels && labels[id]) || id} [${id}]`);
}

// The structure brief (§2.5): plain text, deterministic for a given report. First line asks the
// question; then the campaign, the review, the fingerprint, what changed, every unverified item,
// the plan, the exact CLI and the records consulted.
export function buildStructureBrief({
  campaign, review, report, origin,
}) {
  const s = structureOf(report);
  if (!s) {
    return '';
  }
  const c = campaign || {};
  const cov = s.coverage;
  const plan = cov.plan || {};
  const labels = cov.clientLabels || {};
  const items = cov.items || [];
  const lines = [];
  lines.push(`Structure unverified for campaign ${c.id} — run the plan or adjudicate a rendering-engine fix?`);
  lines.push('');
  lines.push(`Campaign: ${c.name || ''} (id ${c.id})`);
  lines.push(`Admin URL: ${origin || ''}/admin/campaigns/${c.id}`);
  lines.push(`Review: ${(review && review.id) || '?'} · completed ${(review && review.updated_at) || '?'} · rubric ${(report && report.rubricVersion) || '?'}`);
  lines.push(`Fingerprint: ${cov.fingerprint || '?'}`);
  if (s.finding) {
    lines.push(`Structure key: ${s.finding.key}`);
  }
  lines.push(`Canary keys changed since the records: ${(cov.canaryChanged || []).length ? cov.canaryChanged.join(', ') : 'none'}`);
  if ((cov.unavailable || []).length) {
    lines.push(`Inputs unavailable: ${cov.unavailable.join(', ')}`);
  }
  lines.push('');
  lines.push(`Unverified items (${items.length} of ${cov.itemCount || items.length}):`);
  items.forEach((it) => {
    const m = it.missing || {};
    const m1 = m['1'] || [];
    const m2 = m['2'] || [];
    lines.push(`  - ${it.label} — ${it.reason} — ${scopeLabel(it.scope)}`);
    lines.push(`    item: ${it.id}`);
    lines.push(`    missing stage 1 (${m1.length}):${m1.length ? '' : ' none'}`);
    lines.push(...clientList(m1, labels).map((l) => `  ${l}`));
    lines.push(`    missing stage 2 (${m2.length}):${m2.length ? '' : ' none'}`);
    lines.push(...clientList(m2, labels).map((l) => `  ${l}`));
  });
  lines.push('');
  if (plan.available) {
    const stages = plan.stages || {};
    const next = plan.next === null || plan.next === undefined ? '-' : plan.next;
    const count = plan.count || 0;
    lines.push(`Plan: next stage ${next}, ${count} client(s) = ${count} credit(s); remaining stage ${plan.remaining || 0}; total ${plan.total || 0} credit(s) to finish`);
    lines.push(`  stage 1 (${(stages['1'] || []).length}):`);
    lines.push(...clientList(stages['1'], labels));
    lines.push(`  stage 2 (${(stages['2'] || []).length}):`);
    lines.push(...clientList(stages['2'], labels));
  } else {
    lines.push(`Plan: ${plan.reason || 'count unavailable'}`);
  }
  lines.push('');
  lines.push('CLI:');
  (cov.commands || []).forEach((cmd) => lines.push(`  ${cmd}`));
  lines.push('');
  lines.push('Records consulted:');
  if ((cov.recordsUsed || []).length) {
    cov.recordsUsed.forEach((r) => lines.push(`  - #${r.id} · ${dateOf(r.verified_at)} · test ${r.test_id} · stage ${r.stage}`));
  } else {
    lines.push('  none vouch for any item');
  }
  return lines.join('\n');
}
