// Fork (promo series, integrations listmonk/PROMO-SERIES-SPEC D1, D5, D6, D7). Every decision of the
// Campaign page's Promo series group, pure so it is testable (node --test src/promoSeries.test.mjs):
// the form's view of the three served fields, the tag parser (D1's rule mirrored FOR DISPLAY -- the
// server's models/promo.go is the rule), the owned-key fold (the only normaliser), the 60-day engaged
// default on a Step change, the preview's query and which purchased count the audience box uses, the
// notes, and the analytics picker label. Whether a campaign skips anyone is never decided here: that
// is the server's SQL (promo_applicable_tags / promo_purchasers).

export const PROMO_CODE_MAX = 30;
export const PROMO_TAGS_MAX = 20;
export const PROMO_TAG_MAX = 64;
export const PROMO_PREFIX_MIN = 3;
export const PROMO_STEPS = Object.freeze([1, 2, 3]);

const OWNED = ['promo_code', 'promo_step', 'promo_tags', 'promoCode', 'promoStep', 'promoTags'];

// eslint-disable-next-line no-control-regex
const CONTROL = /[\u0000-\u001f\u007f-\u009f]/;

const chars = (s) => Array.from(String(s || '')).length;

export const stepOf = (v) => {
  const n = typeof v === 'string' && v.trim() !== '' ? Number(v) : v;
  return PROMO_STEPS.includes(n) ? n : null;
};

// The Code's problem (an i18n key, '' when valid). A blank Code is valid -- it means "no promo
// series". (Its `n/30` counter was dropped from the hint 2026-11-22, user.)

export const codeProblem = (code) => {
  const c = String(code || '').trim();
  if (c === '') {
    return '';
  }
  if (CONTROL.test(c)) {
    return 'naming.nameControl';
  }
  return chars(c) > PROMO_CODE_MAX ? 'naming.nameTooLong' : '';
};

// One tag (already trimmed): '' when valid, else the reason (length, control, comma, star, prefix) --
// models.PromoTagProblem, mirrored.
export const tagProblem = (tag) => {
  const t = String(tag);
  const n = chars(t);
  if (n < 1 || n > PROMO_TAG_MAX) {
    return 'length';
  }
  if (CONTROL.test(t)) {
    return 'control';
  }
  if (t.includes(',')) {
    return 'comma';
  }
  const i = t.indexOf('*');
  if (i >= 0) {
    if (i !== t.length - 1) {
      return 'star';
    }
    if (chars(t.slice(0, i)) < PROMO_PREFIX_MIN) {
      return 'prefix';
    }
  }
  return '';
};

// The comma-separated text as typed -> { tags, problem }: each piece trimmed, empty pieces dropped,
// case-insensitive duplicates dropped (first kept); problem is the first bad tag's reason, or 'count'
// past 20, or ''.
export const parseTags = (text) => {
  const tags = [];
  const seen = new Set();
  let problem = '';
  String(text || '').split(',').forEach((raw) => {
    const t = raw.trim();
    if (t === '') {
      return;
    }
    const p = tagProblem(t);
    if (p && !problem) {
      problem = p;
    }
    const k = t.toLowerCase();
    if (seen.has(k)) {
      return;
    }
    seen.add(k);
    tags.push(t);
  });
  if (!problem && tags.length > PROMO_TAGS_MAX) {
    problem = 'count';
  }
  return { tags, problem };
};

// The form's three fields from the SERVED top-level fields (never attribs).
export const promoFormOf = (c) => ({
  promoCode: (c && typeof c.promoCode === 'string') ? c.promoCode : '',
  promoStep: stepOf(c && c.promoStep) || '',
  promoTagsText: (c && Array.isArray(c.promoTags) ? c.promoTags : []).join(', '),
});

// The normal form of the form's values: trimmed Code; Step and tags only with a Code.
export const promoValues = (form) => {
  const code = String((form && form.promoCode) || '').trim();
  if (!code) {
    return { code: '', step: null, tags: [] };
  }
  return { code, step: stepOf(form.promoStep), tags: parseTags(form.promoTagsText).tags };
};

// The owned-key fold (D1): writes the three snake_case keys when set, removes them when blank, always
// removes the camelCased junk keys, and removes all three when the form is not a non-evergreen
// regular campaign (`applies` false -- evergreen or opt-in). A new object; attribs may be null.
export const foldPromo = (attribs, form, applies = true) => {
  const out = { ...(attribs || {}) };
  OWNED.forEach((k) => delete out[k]);
  if (!applies) {
    return out;
  }
  const v = promoValues(form);
  if (v.code) {
    out.promo_code = v.code;
    if (v.step) {
      out.promo_step = v.step;
    }
    if (v.tags.length) {
      out.promo_tags = v.tags;
    }
  }
  return out;
};

// Whether the form's promo values equal the stored (served) ones -- then the box uses the served
// purchased share, else the live preview's.
export const samePromo = (form, c) => {
  const a = promoValues(form);
  const b = promoValues(promoFormOf(c));
  return a.code === b.code && a.step === b.step && a.tags.join('\n') === b.tags.join('\n');
};

// D5 -- the purchased count the audience box takes (audience-box.mjs skipSplitOf's `purchased`): null
// (the served share) while the form equals the stored values, else the preview's `skipped` (0 until it
// answers).
export const purchasedArg = (form, c, preview) => {
  if (samePromo(form, c)) {
    return null;
  }
  return preview ? Math.max(0, Number(preview.skipped) || 0) : 0;
};

// The preview's query (GET /api/campaigns/:id/promo-audience): the form's values standing in for the
// stored ones. Tags comma-joined as parsed.
export const previewQuery = (form) => {
  const v = promoValues(form);
  return { code: v.code, step: v.step ? String(v.step) : '', tags: v.tags.join(',') };
};

// The preview's request key: a saved, not-started broadcast whose form carries a Code and tags with no
// problem (null otherwise -- no request). Changes with the query and the stored update time.
export const previewKey = (c, form, show) => {
  if (!show || !c || !c.id || c.evergreen || (c.status !== 'draft' && c.status !== 'scheduled' && c.status !== 'paused')) {
    return null;
  }
  const v = promoValues(form);
  if (!v.code || !v.tags.length || parseTags(form.promoTagsText).problem || codeProblem(v.code)) {
    return null;
  }
  const q = previewQuery(form);
  return `${c.id}:${c.updatedAt || ''}:${q.code}:${q.step}:${q.tags}`;
};

// The group's notes: `noTags` -- step 2 or 3 with a Code and no tags ("nobody is skipped for having
// bought"); `stale` -- the preview answered stale on a step 2 or 3 with a Code and tags.
export const promoNotes = (form, preview) => {
  const v = promoValues(form);
  const later = v.step === 2 || v.step === 3;
  return {
    noTags: !!v.code && later && v.tags.length === 0,
    stale: !!v.code && later && v.tags.length > 0 && !!(preview && preview.stale === true),
  };
};

// Editable while draft or paused (a new campaign is a draft); shown disabled on a scheduled one.
export const promoEditable = (status, canEdit, isNew = false) => !!canEdit && (isNew || status === 'draft' || status === 'paused');

// D6 -- the 60-day engaged default a Step change applies: on for step 2, off for 1, 3 and blank.
export const silentDefaultFor = (step) => stepOf(step) === 2;

// D6 -- on a Step CHANGE of a draft (new or saved), the new step's default, only when the user has not
// clicked the switch this session AND the switch equals the default of the step being left. Returns
// the new switch value, or null to leave it alone.
export const silentDefaultOnStep = ({
  draft, prevStep, nextStep, silentOn, touched,
}) => {
  if (!draft || touched || stepOf(prevStep) === stepOf(nextStep)) {
    return null;
  }
  if ((silentOn === true) !== silentDefaultFor(prevStep)) {
    return null;
  }
  return silentDefaultFor(nextStep);
};

// D6 -- a Step change applied to the form: { promoStep, silentSkip } after the user picks `nextStep`
// (blank as ''), the 60-day engaged default re-applied by silentDefaultOnStep. `ctx` is
// { draft, touched }. A new object; the form is not mutated.
export const applyStepChange = (form, nextStep, ctx) => {
  const promoStep = nextStep === null || nextStep === undefined ? '' : nextStep;
  const next = silentDefaultOnStep({
    draft: !!(ctx && ctx.draft),
    prevStep: form.promoStep,
    nextStep: promoStep,
    silentOn: form.silentSkip === true,
    touched: !!(ctx && ctx.touched),
  });
  return { promoStep, silentSkip: next === null ? form.silentSkip === true : next };
};

// Stage 4 V3 / 4b W1 -- what the SAVE folds when the Code is empty while a Step is set: the Step
// counts as blank exactly as if the user had picked blank (applyStepChange(form, '')), so a 60-day
// default the Step set never outlives its series. Decided at the fold, never while typing: a Code
// field emptied for a moment and retyped keeps its Step (a watcher that reset it lost the Step for
// good). Returns { promoStep, silentSkip } as the fold should read them; the form is not mutated.
export const settleForFold = (form, ctx) => {
  const hasCode = String(form.promoCode || '').trim() !== '';
  const hasStep = !(form.promoStep === '' || form.promoStep === null || form.promoStep === undefined);
  if (hasCode || !hasStep) {
    return { promoStep: form.promoStep, silentSkip: form.silentSkip === true };
  }
  return applyStepChange(form, '', ctx);
};

// D6, Stage 2 S1 -- which value the fold writes for silent_skip: the form's whenever the Smart Sending
// row applies (showSmartSending -- a new campaign included), else the stored one.
export const silentSkipForFold = ({ showSmartSending, formSilent, storedSilent }) => (showSmartSending
  ? formSilent === true : storedSilent === true);

// D7 -- the analytics picker's option label: ` · <Code> S<step>` when a Code is set.
export const pickerLabel = (name, code, step) => {
  const c = typeof code === 'string' ? code.trim() : '';
  if (!c) {
    return name;
  }
  const s = stepOf(step);
  return `${name} · ${c}${s ? ` S${s}` : ''}`;
};

// D7 -- the Campaigns list's Code and Step cells: blank for a journey.
export const promoCells = (c) => {
  if (!c || c.evergreen) {
    return { code: '', step: '' };
  }
  const s = stepOf(c.promoStep);
  return { code: typeof c.promoCode === 'string' ? c.promoCode : '', step: s ? String(s) : '' };
};
