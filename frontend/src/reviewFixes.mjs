// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC D10: the closed list of "Fix for
// me" auto-fixes the review Lambda emits as `fix` objects, applied IN THE BROWSER as the logged-in
// user. Pure: the checklist window (views/CampaignReview.vue) fetches the raw campaign, applies the
// fixes here, recompiles a visual body through EmailBuilder.compileDocument (the sweep's compile,
// OFFICIAL-FOOTER D11), PUTs `reviewPayload` and starts a new inspection.
//
// Every fix names its target exactly -- a block id in `body_source` (visual) or the n-th <a>/<img>
// of the stored body (non-visual) -- and carries the value it expects to find (`from`). A fix whose
// `from` no longer matches (the author edited it since the inspection) is SKIPPED, never forced.
// Nothing outside the named target changes. Nothing else auto-fixes in this release.

import { campaignPayload, deriveContext, isOfficialName } from './officialSweep.mjs'; // eslint-disable-line import/extensions

// ALT-TEXT-SPEC D7: `alt.set` writes an alt (or the slice rule's blank). Since integrations
// ALT-AUTOGEN-SPEC D8 no report finding carries an alt fix: `alt.stripPrefix` is retired and
// `alt.set` is applied only by Inspect's generation (altGenerate.mjs). The agreement rule: every
// integrations lib/campaign-review/report.ts FIX_KINDS entry is in this list.
export const FIX_KINDS = Object.freeze(['subject.trim', 'preheader.trim', 'link.collapseScheme', 'alt.set']);

const clone = (v) => JSON.parse(JSON.stringify(v));

function parseDoc(bodySource) {
  try {
    const d = typeof bodySource === 'string' ? JSON.parse(bodySource) : bodySource;
    return d && typeof d === 'object' ? d : null;
  } catch (e) {
    return null;
  }
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const escapeAttr = (s) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');

// Replace the href value `from` inside an HTML/markdown string (both the raw and the
// entity-escaped forms). Returns null when it does not occur.
function replaceHref(html, from, to) {
  const forms = [[from, to], [escapeAttr(from), escapeAttr(to)]];
  let out = html;
  let hit = false;
  forms.forEach(([f, t]) => {
    const re = new RegExp(`(href\\s*=\\s*["'])${escapeRe(f)}(["'])|(\\]\\()${escapeRe(f)}(\\))`, 'g');
    out = out.replace(re, (m, a1, a2, b1, b2) => {
      hit = true;
      return a1 !== undefined ? `${a1}${t}${a2}` : `${b1}${t}${b2}`;
    });
  });
  return hit ? out : null;
}

// The n-th tag of `name` in html, with its offsets.
function nthTag(html, name, n) {
  const re = new RegExp(`<${name}\\b[^>]*>`, 'gi');
  let m;
  let i = 0;
  // eslint-disable-next-line no-cond-assign
  while ((m = re.exec(html)) !== null) {
    if (i === n) {
      return { start: m.index, end: m.index + m[0].length, tag: m[0] };
    }
    i += 1;
  }
  return null;
}

function replaceAttrInNthTag(html, name, n, attr, from, to) {
  const t = nthTag(html, name, n);
  if (!t) {
    return null;
  }
  const re = new RegExp(`(\\s${attr}\\s*=\\s*)(["'])([^"']*)\\2`, 'i');
  const m = re.exec(t.tag);
  if (!m) {
    return null;
  }
  const decoded = m[3].replace(/&quot;/g, '"').replace(/&#0?39;/g, "'").replace(/&amp;/g, '&');
  if (decoded !== from && m[3] !== from) {
    return null;
  }
  const tag = t.tag.replace(re, `$1$2${escapeAttr(to)}$2`);
  return html.slice(0, t.start) + tag + html.slice(t.end);
}

// ALT-TEXT-SPEC D7: an undecided alt on the n-th <img> -- the tag must carry NO alt attribute;
// one is inserted (entity-escaped) before the closing `>` (or `/>`).
function insertAltInNthImg(html, n, to) {
  const t = nthTag(html, 'img', n);
  if (!t || /\salt\s*=/i.test(t.tag)) {
    return null;
  }
  const close = /\s*\/?>$/.exec(t.tag);
  const tag = `${t.tag.slice(0, close.index)} alt="${escapeAttr(to)}"${t.tag.slice(close.index)}`;
  return html.slice(0, t.start) + tag + html.slice(t.end);
}

// ALT-TEXT-SPEC D7: the same alt is attached to several findings on one image (D4); a fix with the
// same kind, target and `to` as one already applied counts as applied (dispositions record `fixed`).
const sameFix = (a, b) => a.kind === b.kind && a.block === b.block && a.index === b.index && a.to === b.to;

// Apply one fix to the working copy. Returns a skip reason, or null when applied.
function applyOne(c, docRef, fix) {
  if (!fix || !FIX_KINDS.includes(fix.kind)) {
    // A Lambda shipped before this fork emits kinds the window cannot apply (ALT-TEXT-SPEC D8).
    return 'unknown kind';
  }
  if (typeof fix.from !== 'string' || typeof fix.to !== 'string') {
    return 'malformed fix';
  }

  if (fix.kind === 'subject.trim') {
    if (c.subject !== fix.from) {
      return 'the subject changed since the inspection';
    }
    // eslint-disable-next-line no-param-reassign
    c.subject = fix.to;
    return null;
  }

  if (fix.kind === 'preheader.trim') {
    const attribs = c.attribs || {};
    if ((attribs.preheader || '') !== fix.from) {
      return 'the preheader changed since the inspection';
    }
    // eslint-disable-next-line no-param-reassign
    c.attribs = { ...attribs, preheader: fix.to };
    return null;
  }

  // Link and alt fixes: a block in the visual document, or the n-th tag of a stored body.
  if (fix.block) {
    if (!docRef.doc) {
      return 'the campaign has no visual document';
    }
    const b = docRef.doc[fix.block];
    if (!b || !b.data) {
      return 'the block no longer exists';
    }
    const p = b.data.props || {};
    if (fix.kind === 'alt.set') {
      // ALT-TEXT-SPEC D7: the `from` guard, or (undecided) no alt at all; a blank `to` is legitimate.
      const current = fix.undecided ? (p.alt === undefined || p.alt === null) : p.alt === fix.from;
      if (b.type !== 'Image' || !current) {
        return 'the alt text changed since the inspection';
      }
      p.alt = fix.to;
      b.data.props = p;
      // eslint-disable-next-line no-param-reassign
      docRef.changed = true;
      return null;
    }
    // link.collapseScheme
    if (b.type === 'Button' && p.url === fix.from) {
      p.url = fix.to;
    } else if (b.type === 'Image' && p.linkHref === fix.from) {
      p.linkHref = fix.to;
    } else if ((b.type === 'Text' || b.type === 'Heading') && typeof p.text === 'string' && replaceHref(p.text, fix.from, fix.to) !== null) {
      p.text = replaceHref(p.text, fix.from, fix.to);
    } else if (b.type === 'Html' && typeof p.contents === 'string' && replaceHref(p.contents, fix.from, fix.to) !== null) {
      p.contents = replaceHref(p.contents, fix.from, fix.to);
    } else {
      return 'the link changed since the inspection';
    }
    // eslint-disable-next-line no-param-reassign
    docRef.changed = true;
    return null;
  }

  if (Number.isInteger(fix.index)) {
    let next;
    if (fix.kind === 'alt.set' && fix.undecided) {
      next = insertAltInNthImg(c.body || '', fix.index, fix.to);
    } else if (fix.kind === 'alt.set') {
      next = replaceAttrInNthTag(c.body || '', 'img', fix.index, 'alt', fix.from, fix.to);
    } else {
      next = replaceAttrInNthTag(c.body || '', 'a', fix.index, 'href', fix.from, fix.to);
    }
    if (next === null) {
      return 'the body changed since the inspection';
    }
    // eslint-disable-next-line no-param-reassign
    c.body = next;
    return null;
  }
  return 'the fix names no target';
}

// applyFixes(campaign, fixes) -> { campaign, applied, skipped, recompile }. `campaign` is the raw
// (snake_case) API row and is never mutated. `recompile` is true when the visual document changed:
// the caller must recompile the body through the builder before saving.
export function applyFixes(campaign, fixes) {
  const c = clone(campaign);
  const docRef = { doc: c.content_type === 'visual' ? parseDoc(c.body_source) : null, changed: false };
  const applied = [];
  const skipped = [];
  (fixes || []).forEach((fix) => {
    if (fix && applied.some((a) => sameFix(a, fix))) {
      applied.push(fix);
      return;
    }
    const why = applyOne(c, docRef, fix);
    if (why) {
      skipped.push({ fix, reason: why });
    } else {
      applied.push(fix);
    }
  });
  if (docRef.changed) {
    c.body_source = JSON.stringify(docRef.doc);
  }
  return {
    campaign: c, applied, skipped, recompile: docRef.changed,
  };
}

// The PUT payload: exactly officialSweep.mjs's campaignPayload (the Campaign.vue key set) --
// imported, never copied, so the two cannot drift.
export function reviewPayload(campaign, body) {
  return campaignPayload(campaign, body);
}

// integrations ALT-AUTOGEN-SPEC D5 step 6: THE recompile of a raw campaign after a fix -- the
// window's applyStagedFixes and the campaign page's alt generation both call it (never a copy).
// `em` is the builder UMD (EmailBuilder), `templates` the raw (snake_case) template rows (the
// `Official_` ones are the footer refs), `lists` the lists store (deriveContext's brand). Throws when
// the builder has no compileDocument.
export function compileCampaignBody(em, campaign, templates, lists) {
  if (!em || typeof em.compileDocument !== 'function') {
    throw new Error('builder unavailable');
  }
  const refs = (Array.isArray(templates) ? templates : [])
    .filter((t) => t.type === 'campaign_visual' && isOfficialName(t.name))
    .map((t) => ({ id: t.id, name: t.name, body_source: t.body_source }));
  const ctx = deriveContext(campaign, lists || []);
  return em.compileDocument(JSON.parse(campaign.body_source), { lang: ctx.lang, brand: ctx.brand }, refs);
}

// The builder UMD loaded into a hidden same-origin <iframe> (its own window, so its globals never
// touch the page's): resolves EmailBuilder. Shared by the window and the campaign page.
export function loadFrameBuilder(frame, cacheBust) {
  const win = frame && frame.contentWindow;
  if (win && win.EmailBuilder) {
    return Promise.resolve(win.EmailBuilder);
  }
  return new Promise((resolve, reject) => {
    if (!frame || !frame.contentDocument) {
      reject(new Error('no builder frame'));
      return;
    }
    const script = frame.contentDocument.createElement('script');
    script.src = `/admin/static/email-builder/email-builder.umd.js?v=${cacheBust}`;
    script.onload = () => (frame.contentWindow.EmailBuilder ? resolve(frame.contentWindow.EmailBuilder) : reject(new Error('no EmailBuilder')));
    script.onerror = reject;
    frame.contentDocument.head.appendChild(script);
  });
}

// Stage 4 finding 6: the dispositions to record AFTER the fixes ran. A "Fix for me" whose fix was
// applied is `fixed`; one that could not be applied (the text changed, the builder was unavailable)
// is recorded as `fixme` -- the log never claims a fix that did not happen. Other decisions pass
// through. `applied` holds the fix objects applyFixes returned as applied (identity match).
export function dispositionsAfterFixes(staged, applied) {
  const done = new Set(applied || []);
  return (staged || []).map((s) => {
    let { action } = s;
    if (action === 'fixed' && !(s.fix && done.has(s.fix))) {
      action = 'fixme';
    }
    return {
      key: s.key, rubric_id: s.rubric_id, action, note: s.note || '',
    };
  });
}
