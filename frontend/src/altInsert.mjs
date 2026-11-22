// Fork (campaign review) -- integrations ALT-ON-INSERT-SPEC D5: alt text is generated IN THE EDITOR
// when an image arrives, and regenerated when the campaign's language changes. Pure, dependencies
// injected (the altGenerate.mjs pattern); wired in Campaign.vue.
//
//   imagePairs   the set of `<block id>\u0000<trimmed url>` over a document's Image blocks with a
//                non-empty url (the Lambda's `src` is the trimmed url) -- Trigger 1's comparison.
//   normLang     the page's copy of the Lambda's `campaignLang` normalisation ('' -> 'en'). Its
//                table is tested here AND in integrations tests/lib/campaign-review/alt-generate.test.ts
//                (I8) -- the two must agree.
//   stampLang    every generated map entry with no `lang` gets the language being left (Trigger 2).
//   createAltInsert  the page's sequence: the two triggers, the 1,500 ms debounce, single flight,
//                the snapshot request (POST /api/campaigns/alts), and D5's ONE rule for using an
//                answer, read when it arrives. It never saves: the only API it holds is
//                generateAltsDraft, and the only writes it makes are to the open editor (applyAlts)
//                and to the form's attribs text.

import { generatedMap } from './altGenerate.mjs'; // eslint-disable-line import/extensions

export const ALT_INSERT_DEBOUNCE_MS = 1500;
// How often a dropped (b)/(d)/(e) answer looks for the page to settle before its one queued run.
export const ALT_INSERT_SETTLE_POLL_MS = 500;

const SEP = '\u0000';

// D5 Trigger 1. null when the source is not a builder document (such a change is no trigger and is
// not the new baseline).
export function imagePairs(sourceStr) {
  let doc;
  try {
    doc = typeof sourceStr === 'string' ? JSON.parse(sourceStr) : sourceStr;
  } catch (e) {
    return null;
  }
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) {
    return null;
  }
  const out = new Set();
  Object.keys(doc).forEach((id) => {
    const b = doc[id];
    if (!b || b.type !== 'Image') {
      return;
    }
    const url = b.data && b.data.props && typeof b.data.props.url === 'string' ? b.data.props.url.trim() : '';
    if (url !== '') {
      out.add(`${id}${SEP}${url}`);
    }
  });
  return out;
}

// I8: the Lambda's campaignLang -- String(lang ?? '') || 'en'.
export function normLang(lang) {
  return String(lang === undefined || lang === null ? '' : lang) || 'en';
}

const isObject = (v) => !!v && typeof v === 'object' && !Array.isArray(v);

// D5 Trigger 2: a copy of `map` in which every entry with no (string) `lang` carries `lang`.
export function stampLang(map, lang) {
  const out = {};
  if (!isObject(map)) {
    return out;
  }
  Object.keys(map).forEach((k) => {
    const e = map[k];
    out[k] = isObject(e) && typeof e.lang !== 'string' ? { ...e, lang } : e;
  });
  return out;
}

// The form's attribs text as an object, or null when it does not parse to one (an empty text is {}).
export function parseAttribs(str) {
  if (typeof str !== 'string' || str.trim() === '') {
    return {};
  }
  try {
    const v = JSON.parse(str);
    return isObject(v) ? v : null;
  } catch (e) {
    return null;
  }
}

const mapOf = (attribs) => (attribs && isObject(attribs.alt_generated) ? attribs.alt_generated : {});

// Deep equality over JSON values, key order ignored.
export function sameJSON(a, b) {
  if (a === b) {
    return true;
  }
  if (Array.isArray(a) || Array.isArray(b)) {
    return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((x, i) => sameJSON(x, b[i]));
  }
  if (isObject(a) && isObject(b)) {
    const ka = Object.keys(a).sort();
    const kb = Object.keys(b).sort();
    return ka.length === kb.length && ka.every((k, i) => k === kb[i] && sameJSON(a[k], b[k]));
  }
  return false;
}

const clone = (v) => JSON.parse(JSON.stringify(v));

// deps:
//   api            { generateAltsDraft(snapshot) } -- the ONLY API (I12)
//   getState()     { mount, contentType, editable, status, name, attribsStr, lang, source, body,
//                    saving, syncing, inspecting }: `mount` identifies the editor mount (editorKey),
//                    `editable` is canEdit && draft/paused, `lang` the form's language field
//   setAttribsStr(text)   write the form's attribs text
//   applyAlts(fixes)      the builder's EmailBuilder.applyAlts -> {applied, skipped}, or null when
//                         the loaded bundle predates it (then nothing is done with the answer)
//   toast(count)          "N alt tags generated"
//   setTimeout / clearTimeout   timers (injected for the tests)
export function createAltInsert(deps) {
  const st = {
    mount: undefined,
    pairs: null,
    timer: null,
    inFlight: false,
    queued: false,
    settleTimer: null,
    // Set by destroy(): no run, no use of an answer and no poll after the page is gone.
    destroyed: false,
  };
  const setT = deps.setTimeout || setTimeout;
  const clearT = deps.clearTimeout || clearTimeout;
  const debounceMs = deps.debounceMs === undefined ? ALT_INSERT_DEBOUNCE_MS : deps.debounceMs;
  const settleMs = deps.settlePollMs === undefined ? ALT_INSERT_SETTLE_POLL_MS : deps.settlePollMs;

  const state = () => deps.getState() || {};
  // D5 "When": visual, editable, attribs text parses to an object.
  const whenHolds = (s) => s.contentType === 'visual' && !!s.editable && parseAttribs(s.attribsStr) !== null;
  const settled = (s) => !s.saving && !s.syncing && !s.inspecting;

  const writeMap = (map) => {
    const attribs = parseAttribs(state().attribsStr);
    if (attribs === null) {
      return;
    }
    deps.setAttribsStr(JSON.stringify({ ...attribs, alt_generated: map }, null, 4));
  };

  let run;

  // A dropped (b)/(d)/(e) answer: one run once the page is not saving, syncing or inspecting.
  const queueSettled = () => {
    // A pending debounce already runs on the settled page: one follow-up run, never two.
    if (st.destroyed || st.settleTimer !== null || st.timer !== null) {
      return;
    }
    const poll = () => {
      st.settleTimer = null;
      if (st.destroyed) {
        return;
      }
      if (settled(state())) {
        run();
        return;
      }
      st.settleTimer = setT(poll, settleMs);
    };
    st.settleTimer = setT(poll, settleMs);
  };

  const use = (res, snap) => {
    if (st.destroyed) {
      return;
    }
    const s = state();
    // (a) the same editor mount; (c) the When conditions; (f) an ok/none answer -- else dropped, nothing queued.
    if (s.mount !== snap.mount || !whenHolds(s) || !res || (res.status !== 'ok' && res.status !== 'none')) {
      return;
    }
    // (b) settled; (d) the form's map is the one the snapshot carried; (e) the answer's language is
    // the form's -- else dropped, one run queued for when the page has settled.
    const attribs = parseAttribs(s.attribsStr);
    if (!settled(s) || !sameJSON(mapOf(attribs), snap.map) || res.lang !== normLang(s.lang)) {
      queueSettled();
      return;
    }
    const fixes = Array.isArray(res.fixes) ? res.fixes : [];
    const keep = isObject(res.keep) ? res.keep : {};
    if (fixes.length) {
      const r = deps.applyAlts(fixes);
      if (!r) {
        // A stale cached bundle without applyAlts: nothing (D6).
        return;
      }
      const applied = Array.isArray(r.applied) ? r.applied : [];
      if (applied.length) {
        writeMap(generatedMap(keep, applied, res.at, res.lang));
        deps.toast(applied.length);
        return;
      }
    }
    // No applied fix: the map is written only when `keep` differs (dropped or refreshed entries).
    if (!sameJSON(keep, mapOf(attribs))) {
      writeMap(generatedMap(keep, [], res.at, res.lang));
    }
  };

  run = async () => {
    if (st.destroyed) {
      return;
    }
    if (st.inFlight) {
      st.queued = true;
      return;
    }
    const s = state();
    if (!whenHolds(s)) {
      return;
    }
    const attribs = parseAttribs(s.attribsStr);
    const map = clone(mapOf(attribs));
    const snap = { mount: s.mount, map };
    const snapshot = {
      content_type: s.contentType,
      body_source: s.source,
      body: s.body,
      attribs: { lang: s.lang || '', alt_generated: map },
      status: s.status,
      name: s.name,
    };
    st.inFlight = true;
    let res = null;
    try {
      res = await deps.api.generateAltsDraft(snapshot);
    } catch (e) {
      res = null; // (f): a failure of any kind shows nothing.
    }
    st.inFlight = false;
    try {
      use(res, snap);
    } catch (e) {
      // A failure applying shows nothing; Inspect generates as before.
    }
    if (st.queued) {
      st.queued = false;
      run();
    }
  };

  const trigger = () => {
    if (!whenHolds(state())) {
      return;
    }
    if (st.timer !== null) {
      clearT(st.timer);
    }
    st.timer = setT(() => {
      st.timer = null;
      run();
    }, debounceMs);
  };

  return {
    // D5 Trigger 1: a builder change. The first change after a mount is the baseline, never a trigger.
    onBuilderChange({ mount, source }) {
      const pairs = imagePairs(source);
      if (pairs === null) {
        return;
      }
      if (mount !== st.mount || st.pairs === null) {
        st.mount = mount;
        st.pairs = pairs;
        return;
      }
      const prev = st.pairs;
      st.pairs = pairs;
      let arrived = false;
      pairs.forEach((p) => {
        if (!prev.has(p)) {
          arrived = true;
        }
      });
      if (arrived) {
        trigger();
      }
    },

    // D5 Trigger 2: the form's language changed from `from` to `to`. The stamp is written at once,
    // whether or not a request follows; unparseable attribs text: no stamp and no run.
    onLangChange(from, to) {
      const a = normLang(from);
      if (a === normLang(to)) {
        return;
      }
      const attribs = parseAttribs(state().attribsStr);
      if (attribs === null) {
        return;
      }
      const map = mapOf(attribs);
      const stamped = stampLang(map, a);
      if (!sameJSON(stamped, map)) {
        deps.setAttribsStr(JSON.stringify({ ...attribs, alt_generated: stamped }, null, 4));
      }
      trigger();
    },

    // An Inspect click cancels a pending debounce (Inspect generates itself).
    onInspect() {
      if (st.timer !== null) {
        clearT(st.timer);
        st.timer = null;
      }
    },

    // The page is going away: no timer outlives it.
    destroy() {
      st.destroyed = true;
      st.queued = false;
      this.onInspect();
      if (st.settleTimer !== null) {
        clearT(st.settleTimer);
        st.settleTimer = null;
      }
    },
  };
}
