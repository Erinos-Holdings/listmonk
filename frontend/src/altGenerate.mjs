// Fork (campaign review) -- integrations ALT-AUTOGEN-SPEC D1/D4/D5/D9: alt text is generated inside
// Inspect, before the inspection. Pure, dependencies injected:
//
//   generateAndApply  the campaign page's sequence between its save and startCampaignReview: ask
//                     the review Lambda for alts (POST /api/campaigns/:id/alts), apply them through
//                     the existing alt.set path (reviewFixes.mjs applyFixes -> recompile ->
//                     reviewPayload PUT), record them in attribs.alt_generated. EVERY return path
//                     lets the caller start the inspection: generation never stops one.
//   generatedMap      keep + one entry per APPLIED fix (a skipped fix records nothing); each new
//                     entry carries `from` (omitted when undecided) and the Lambda's `at` (A1.4).
//   acceptAlts /      the Inspect window's half: which `lm-review:alts` messages it takes (same
//   altsRowState      origin, from its opener, for its campaign) and the live state, de-duplicated
//                     by token. What the window shows is the checklist's first step
//                     (reviewChecklist.mjs altsStep, ALT-AUTOGEN-SPEC A1.1).
//
// Message (same origin only, posted on reviewNavigate.mjs OPENER_POST_SCHEDULE_MS):
//   campaign page -> Inspect window  {type: 'lm-review:alts', campaignId, phase, alts?, skipped?, rejected?, token}
// `token` is `<run>#<seq>`: one run per Inspect click, seq 1, 2, ... per phase in order, so a late
// re-post of an earlier phase never overwrites a later one, and the next click's run replaces the row.

import { applyFixes, reviewPayload } from './reviewFixes.mjs'; // eslint-disable-line import/extensions

export const MSG_ALTS = 'lm-review:alts';
export const ALT_PHASES = Object.freeze(['generating', 'done', 'failed']);

const keyOf = (f) => (f.block ? `block:${f.block}` : `index:${f.index}`);

// D4 / A1.4: the map the generation save writes -- the Lambda's `keep` plus one
// {src, alt, from?, at?} per applied fix. `from` is the replaced alt (omitted when the image had none,
// the fix's `undecided`); `at` is the Lambda's answer time, the same value on every new entry -- never
// this browser's clock (A1-S2-3). An answer without `at` (an older Lambda) writes no `at`.
export function generatedMap(keep, applied, at) {
  const out = { ...((keep && typeof keep === 'object' && !Array.isArray(keep)) ? keep : {}) };
  (applied || []).forEach((f) => {
    if (f && f.kind === 'alt.set' && (f.block || Number.isInteger(f.index))) {
      out[keyOf(f)] = {
        src: String(f.src || ''),
        alt: f.to,
        ...(!f.undecided && typeof f.from === 'string' ? { from: f.from } : {}),
        ...(typeof at === 'string' && at !== '' ? { at } : {}),
      };
    }
  });
  return out;
}

// D5: the sequence. deps: { id, api: { generateAlts(id), getCampaignRaw(id), updateCampaign(id, payload) },
// compile(campaign) -> Promise<body> (reviewFixes.mjs compileCampaignBody with the Official_ refs and
// deriveContext), notify(message) }. Resolves { saved, phase }; never rejects.
export async function generateAndApply({
  id, api, compile, notify,
}) {
  const say = (m) => {
    try {
      const r = notify && notify(m);
      if (r && typeof r.catch === 'function') r.catch(() => {});
    } catch (e) {
      // The window is closed or the post threw: the display is informational.
    }
  };
  const failed = () => {
    say({ phase: 'failed' });
    return { saved: false, phase: 'failed' };
  };

  say({ phase: 'generating' });
  let res;
  try {
    res = await api.generateAlts(id);
  } catch (e) {
    return failed();
  }
  if (!res || (res.status !== 'ok' && res.status !== 'none')) {
    return failed();
  }
  const fixes = Array.isArray(res.fixes) ? res.fixes : [];
  const rejected = Number.isInteger(res.rejected) && res.rejected > 0 ? res.rejected : 0;
  if (!fixes.length) {
    say({ phase: 'done', alts: [], rejected });
    return { saved: false, phase: 'done' };
  }

  let raw;
  try {
    raw = await api.getCampaignRaw(id);
  } catch (e) {
    return failed();
  }
  const {
    campaign, applied, skipped, recompile,
  } = applyFixes(raw, fixes);
  if (!applied.length) {
    say({
      phase: 'done', alts: [], skipped: skipped.length, rejected,
    });
    return { saved: false, phase: 'done' };
  }

  let { body } = campaign;
  if (recompile) {
    try {
      body = await compile(campaign);
    } catch (e) {
      return failed();
    }
    if (typeof body !== 'string' || body === '') {
      return failed();
    }
  }
  campaign.attribs = { ...(campaign.attribs || {}), alt_generated: generatedMap(res.keep, applied, res.at) };
  try {
    await api.updateCampaign(id, reviewPayload(campaign, body));
  } catch (e) {
    // A rejected PUT (reviewGuardOnEdit on a scheduled campaign, say): the inspection runs on the first save.
    return failed();
  }
  say({
    phase: 'done',
    alts: applied.map(({ to, block, index }) => ({ to, ...(block ? { block } : { index }) })),
    skipped: skipped.length,
    rejected,
  });
  return { saved: true, phase: 'done' };
}

// A message to the window. `run` is the click's token, `seq` the phase's order within it.
export function altsMessage(campaignId, run, seq, m) {
  return {
    type: MSG_ALTS,
    campaignId,
    phase: m.phase,
    ...(Array.isArray(m.alts) ? { alts: m.alts } : {}),
    ...(Number.isInteger(m.skipped) ? { skipped: m.skipped } : {}),
    ...(Number.isInteger(m.rejected) ? { rejected: m.rejected } : {}),
    token: `${run}#${seq}`,
  };
}

// D9: the window takes an `lm-review:alts` only from our origin, from the window that is its opener
// (REVIEW-NAVIGATION §4.5: the adopted opener, else window.opener), for its own campaign.
export function acceptAlts({
  origin, ourOrigin, source, opener, data, campaignId,
}) {
  if (!ourOrigin || origin !== ourOrigin) return false;
  if (!source || !opener || source !== opener) return false;
  if (!data || typeof data !== 'object' || data.type !== MSG_ALTS) return false;
  if (data.campaignId !== campaignId) return false;
  if (!ALT_PHASES.includes(data.phase)) return false;
  return typeof data.token === 'string' && /^[^#]+#\d+$/.test(data.token);
}

const runOf = (token) => token.slice(0, token.lastIndexOf('#'));
const seqOf = (token) => Number(token.slice(token.lastIndexOf('#') + 1));

// The live state after an accepted message: a token already seen, or an earlier phase of the same
// run, changes nothing; a new run replaces the state. `baseJob` (A1.1 rule 1) is the job id of the
// report the window shows when the run's first message arrives (null with no report; undefined while
// the window has not loaded its review state yet -- the window fills it on its first load). The run
// drives the checklist's alt step until a report of a later job is shown (reviewChecklist.mjs altsStep).
export function altsRowState(state, data, baseJob) {
  if (state && state.token === data.token) return state;
  const run = runOf(data.token);
  if (state && state.run === run && seqOf(data.token) <= state.seq) return state;
  const strs = (Array.isArray(data.alts) ? data.alts : [])
    .filter((a) => a && typeof a.to === 'string')
    .map((a) => a.to);
  return {
    token: data.token,
    run,
    seq: seqOf(data.token),
    phase: data.phase,
    alts: strs,
    skipped: Number.isInteger(data.skipped) ? data.skipped : 0,
    rejected: Number.isInteger(data.rejected) ? data.rejected : 0,
    baseJob: state && state.run === run ? state.baseJob : baseJob,
  };
}
