// Fork (on-demand brand-health refresh, integrations BRANDHEALTH-REFRESH-SPEC D3/I5). The pure
// half of the SES pane's "Recompute now": who sees the button, when a recompute counts as done,
// and the start -> poll -> done | capped state machine. No Vue, no DOM; tested by
// healthRefresh.test.mjs (yarn test). The fork computes nothing: the run is the integrations
// BrandHealth Lambda's, and its only signal here is the stored ses document's `computedAt`.
//
// `computedAt` advances only when the run's SES leg fully succeeded (Stage 2 S3) -- a partial or
// failed run leaves it unmoved, so the 3-minute cap's message says "finished with errors or still
// running", never "failed".

export const POLL_MS = 10000;
export const CAP_MS = 3 * 60 * 1000;

// D3: the button renders only for a brands:manage holder AND while the server says the
// integration is configured (serverConfig.health_refresh_enabled -- a boolean; the URL and the
// secret never reach the SPA).
export function canRefresh(serverConfig, can) {
  return !!(serverConfig && serverConfig.health_refresh_enabled === true)
    && typeof can === 'function' && !!can('brands:manage');
}

const ms = (v) => (typeof v === 'string' && v ? Date.parse(v) : NaN);

// The baseline a click captures: the current ses doc's computedAt, or null when there is none.
export function baselineOf(doc) {
  return (doc && typeof doc.computedAt === 'string' && doc.computedAt) ? doc.computedAt : null;
}

// Has the stored document moved past the baseline? With no prior document (or one without a
// computedAt), the first document that carries a computedAt counts as the advance (Stage 2 S3).
export function advanced(baseline, doc) {
  const now = ms(doc && doc.computedAt);
  if (Number.isNaN(now)) {
    return false;
  }
  const was = ms(baseline);
  return Number.isNaN(was) ? true : now > was;
}

// The i18n key of each state's line; idle has none.
export const STATE_MESSAGE = Object.freeze({
  idle: '',
  starting: 'dashboard.ses.refreshing',
  running: 'dashboard.ses.refreshing',
  busy: 'dashboard.ses.refreshBusy',
  done: 'dashboard.ses.refreshDone',
  capped: 'dashboard.ses.refreshCapped',
  failed: 'dashboard.ses.refreshFailed',
});

// The button is disabled while a recompute is being started or polled.
export function isActive(state) {
  return state === 'starting' || state === 'running' || state === 'busy';
}

// createRefresh drives one recompute at a time.
//   start()  -> Promise<{started, busy}>   POST /api/system/health/refresh
//   read()   -> Promise<doc|null>          the newest ses document (a quiet read)
//   onState(state, doc)                    every transition; `doc` on done
//   now(), setTimer(fn, ms), clearTimer(id) injectable for tests.
// begin(baseline) starts; stop() abandons the poll (component destroyed). A start error is
// `failed`; a read error is retried at the next tick (the cap still applies).
export function createRefresh(opts) {
  const now = opts.now || (() => Date.now());
  const setTimer = opts.setTimer || ((fn, t) => setTimeout(fn, t));
  const clearTimer = opts.clearTimer || ((id) => clearTimeout(id));
  let state = 'idle';
  let timer = null;
  let gen = 0;

  const set = (s, doc) => {
    state = s;
    if (opts.onState) {
      opts.onState(s, doc);
    }
  };

  function stop() {
    gen += 1;
    if (timer !== null) {
      clearTimer(timer);
      timer = null;
    }
  }

  function poll(my, baseline, startedAt) {
    timer = setTimer(async () => {
      timer = null;
      if (my !== gen) {
        return;
      }
      let doc = null;
      try {
        doc = await opts.read();
      } catch (e) {
        doc = null;
      }
      if (my !== gen) {
        return;
      }
      if (advanced(baseline, doc)) {
        set('done', doc);
        return;
      }
      if (now() - startedAt >= CAP_MS) {
        set('capped');
        return;
      }
      poll(my, baseline, startedAt);
    }, POLL_MS);
  }

  async function begin(baseline) {
    if (isActive(state)) {
      return;
    }
    stop();
    const my = gen;
    set('starting');
    let res;
    try {
      res = await opts.start();
    } catch (e) {
      if (my === gen) {
        set('failed');
      }
      return;
    }
    if (my !== gen) {
      return;
    }
    const started = !!(res && res.started);
    const busy = !!(res && res.busy);
    if (!started && !busy) {
      set('failed');
      return;
    }
    set(busy ? 'busy' : 'running');
    poll(my, baseline, now());
  }

  return {
    begin,
    stop,
    get state() { return state; },
  };
}
