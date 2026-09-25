// Fork (official footer) -- OFFICIAL-FOOTER-SPEC D11: the re-save sweep that runs in the browser
// after an `Official_` template is saved. An official template is what every campaign's
// OfficialFooter block renders, but a campaign's stored body is compiled at Save -- so a footer
// edit reaches nothing until each carrier is re-saved. This file is the pure planner + the
// runner (every effect injected); TemplateForm.vue owns the confirm, the toasts and the wiring.
//
// Selection is by RESOLUTION, not by "has a footer": an item is re-saved only when one of its
// OfficialFooter blocks resolves (kind + lang + brand, the builder's official/resolve.ts rule)
// to the saved template -- so one brand's footer edit never rewrites another brand's drafts.
//
// Payloads are the full key sets integrations' lib/listmonk-resave.ts pins
// (CAMPAIGN_UPDATE_KEYS / TEMPLATE_UPDATE_KEYS): `PUT /api/campaigns/:id` is a full replace
// (absent lists/media CLEAR the associations) and the template update SQL writes `brand`
// unconditionally (absent => the column is zeroed). Both sides carry a key-set pin test.
//
// Disposable at listmonk v7, like the rest of the Vue: the Go refusals are the durable half.

export const OFFICIAL_PREFIX = 'Official_';

// MUST MATCH integrations lib/listmonk-resave.ts CAMPAIGN_UPDATE_KEYS (Campaign.vue's
// updateCampaign() key set, in its order).
export const CAMPAIGN_UPDATE_KEYS = Object.freeze([
  'archive_slug', 'name', 'subject', 'lists', 'from_email', 'messenger', 'type', 'tags', 'send_at',
  'headers', 'attribs', 'template_id', 'content_type', 'body', 'body_source', 'altbody', 'archive',
  'archive_template_id', 'archive_meta', 'media', 'evergreen', 'send_delay_secs',
]);

// MUST MATCH integrations lib/listmonk-resave.ts TEMPLATE_UPDATE_KEYS (TemplateForm.vue's
// updateTemplate() key set, brand + lang included).
export const TEMPLATE_UPDATE_KEYS = Object.freeze(['id', 'name', 'type', 'subject', 'body', 'body_source', 'brand', 'lang']);

// The statuses a campaign may be edited in (Campaign.vue canEdit); a running EVERGREEN is edited
// by pausing it first.
export const SWEEP_STATUSES = Object.freeze(['draft', 'scheduled', 'paused']);

const BRAND_TAG_PREFIX = 'brand:';
const CURATED = 'curated';

export const isOfficialName = (name) => typeof name === 'string' && name.startsWith(OFFICIAL_PREFIX);

const slug = (s) => String(s == null ? '' : s).toLowerCase().replace(/[^a-z0-9]/g, '');
const upperLang = (l) => (String(l == null ? '' : l).trim() || 'en').toUpperCase();

// Mirrors the builder's parseOfficialName (official/resolve.ts).
export function parseOfficialName(name) {
  const corp = /^Official_Footer_([A-Za-z]+)$/.exec(String(name || ''));
  if (corp) {
    return { kind: 'corporate', brand: '', lang: corp[1] };
  }
  const brand = /^Official_(.+)_Footer_([A-Za-z]+)$/.exec(String(name || ''));
  if (brand) {
    return { kind: 'brand', brand: slug(brand[1]), lang: brand[2] };
  }
  return null;
}

// A campaign's resolution context (I9's Vue twin): lang = attribs.lang (empty -> en); brand =
// the ONE `brand:` tag across its lists (lower-folded; none -> curated; two -> error, never
// guessed). `lists` is the lists store (tags included); a campaign's own lists are {id, name}.
export function deriveContext(campaign, lists) {
  const attribs = (campaign && campaign.attribs) || {};
  const lang = attribs.lang || 'en';
  const all = lists || [];
  const brands = new Set();
  ((campaign && campaign.lists) || []).forEach((l) => {
    const full = all.find((x) => x.id === (l && l.id));
    const tag = ((full && full.tags) || []).find((t) => t.startsWith(BRAND_TAG_PREFIX));
    if (tag) {
      brands.add(tag.slice(BRAND_TAG_PREFIX.length).toLowerCase());
    }
  });
  if (brands.size > 1) {
    return { lang, brand: null, error: `lists carry ${brands.size} brands (${[...brands].sort().join(', ')})` };
  }
  return { lang, brand: brands.size === 1 ? [...brands][0] : CURATED, error: null };
}

// A template's context: its own columns (TemplateForm.vue's context, D9).
export function templateContext(template) {
  return { lang: (template && template.lang) || 'en', brand: (template && template.brand) || CURATED, error: null };
}

// The OfficialFooter kinds a stored body_source holds.
export function officialKinds(bodySource) {
  let doc = null;
  try {
    doc = typeof bodySource === 'string' ? JSON.parse(bodySource) : bodySource;
  } catch (e) {
    return [];
  }
  if (!doc || typeof doc !== 'object') {
    return [];
  }
  const kinds = new Set();
  Object.values(doc).forEach((b) => {
    if (b && b.type === 'OfficialFooter' && b.data && b.data.props && typeof b.data.props.kind === 'string') {
      kinds.add(b.data.props.kind);
    }
  });
  return [...kinds];
}

// Does a block of `kind` under `ctx` resolve to the template named `savedName`?
export function resolvesTo(kind, ctx, savedName) {
  const p = parseOfficialName(savedName);
  if (!p || !ctx || p.kind !== kind || upperLang(ctx.lang) !== p.lang) {
    return false;
  }
  if (kind === 'corporate') {
    return true;
  }
  const b = slug(ctx.brand);
  return b !== '' && b !== CURATED && b === p.brand;
}

function carries(bodySource, ctx, savedName) {
  return officialKinds(bodySource).some((k) => resolvesTo(k, ctx, savedName));
}

// The plan. `campaigns`/`templates` are the raw (snake_case) API rows; `savedTemplate` is the
// official template just saved ({id, name}); `lists` the lists store.
//   campaigns   visual, draft/scheduled/paused, carrying a block that resolves to it
//   evergreens  running evergreens carrying one (pause -> save -> resume)
//   templates   non-official campaign_visual templates carrying one (own lang/brand columns)
//   unresolved  carriers whose brand cannot be derived (two brands): listed, never guessed
export function selectSweepItems(campaigns, templates, savedTemplate, lists) {
  const out = {
    campaigns: [], evergreens: [], templates: [], unresolved: [],
  };
  const name = savedTemplate && savedTemplate.name;
  if (!isOfficialName(name)) {
    return out;
  }

  (campaigns || []).forEach((c) => {
    if (!c || c.content_type !== 'visual' || officialKinds(c.body_source).length === 0) {
      return;
    }
    const editable = SWEEP_STATUSES.includes(c.status);
    const evergreen = c.status === 'running' && !!c.evergreen;
    if (!editable && !evergreen) {
      return;
    }
    const ctx = deriveContext(c, lists);
    if (ctx.error) {
      // Its brand cannot be derived, so neither can its compile: listed, never written -- when
      // it holds a block of the saved template's kind that could resolve to it.
      const saved = parseOfficialName(name);
      const couldResolve = saved && officialKinds(c.body_source).includes(saved.kind)
        && (saved.kind === 'brand' || upperLang(ctx.lang) === saved.lang);
      if (couldResolve) {
        out.unresolved.push({
          kind: 'campaign', id: c.id, name: c.name, reason: ctx.error,
        });
      }
      return;
    }
    if (!carries(c.body_source, ctx, name)) {
      return;
    }
    const item = {
      kind: 'campaign', id: c.id, name: c.name, status: c.status,
    };
    (evergreen ? out.evergreens : out.campaigns).push(item);
  });

  (templates || []).forEach((t) => {
    if (!t || t.type !== 'campaign_visual' || isOfficialName(t.name) || t.id === savedTemplate.id) {
      return;
    }
    if (carries(t.body_source, templateContext(t), name)) {
      out.templates.push({ kind: 'template', id: t.id, name: t.name });
    }
  });
  return out;
}

const idsOf = (rows) => (rows || []).flatMap((r) => (r && typeof r.id === 'number' ? [r.id] : []));

// The campaign PUT body: every value from the fetched (raw) campaign, `body` replaced. Mirrors
// integrations buildCampaignUpdate.
export function campaignPayload(campaign, body) {
  return {
    archive_slug: campaign.archive_slug == null ? null : campaign.archive_slug,
    name: campaign.name,
    subject: campaign.subject,
    lists: idsOf(campaign.lists),
    from_email: campaign.from_email,
    messenger: campaign.messenger,
    type: 'regular',
    tags: campaign.tags,
    send_at: campaign.send_at == null ? null : campaign.send_at,
    headers: campaign.headers,
    attribs: campaign.attribs,
    template_id: campaign.template_id,
    content_type: campaign.content_type,
    body,
    body_source: campaign.body_source,
    altbody: campaign.altbody == null ? null : campaign.altbody,
    archive: campaign.archive,
    archive_template_id: campaign.archive_template_id == null ? null : campaign.archive_template_id,
    archive_meta: campaign.archive_meta,
    media: idsOf(campaign.media),
    evergreen: campaign.evergreen,
    send_delay_secs: campaign.send_delay_secs,
  };
}

// The template PUT body. `brand` and `lang` are REQUIRED: the update SQL writes brand=$6
// unconditionally, so a payload without it zeroes the column.
export function templatePayload(template, body) {
  return {
    id: template.id,
    name: template.name,
    type: template.type,
    subject: template.subject,
    body,
    body_source: template.body_source,
    brand: template.brand || '',
    lang: template.lang || 'en',
  };
}

const errText = (e) => String((e && e.message) || e);

// The runner. Sequential; one failure never stops the rest and never un-saves the template.
//   deps.api: getCampaign(id) / getTemplate(id) (raw rows), updateCampaign(id, payload),
//             updateTemplate(payload), changeStatus(id, status)
//   deps.compile(document, context) -> body (EmailBuilder.compileDocument with fresh refs)
//   deps.canSend: $can('campaigns:send') -- required for the evergreens' status PUTs
//   deps.lists: the lists store; deps.onProgress(done, total, item)
// Running evergreens: paused and resumed ONE AT A TIME, the resume in a `finally` (a failed body
// PUT leaves the old body, so the resume passes the footer guard); one left paused is named
// first in the summary. Without the grant they are listed, not touched.
export async function runSweep(plan, deps) {
  const summary = {
    saved: [], failed: [], leftPaused: [], notResaved: [], unresolved: plan.unresolved || [],
  };
  const evergreens = deps.canSend ? plan.evergreens : [];
  if (!deps.canSend) {
    summary.notResaved = plan.evergreens.map((e) => e.id);
  }
  const total = plan.campaigns.length + evergreens.length + plan.templates.length;
  let done = 0;
  const progress = (item) => {
    done += 1;
    if (deps.onProgress) {
      deps.onProgress(done, total, item);
    }
  };

  const saveCampaign = async (id) => {
    const c = await deps.api.getCampaign(id);
    const ctx = deriveContext(c, deps.lists);
    if (ctx.error) {
      throw new Error(ctx.error);
    }
    const body = deps.compile(JSON.parse(c.body_source), { lang: ctx.lang, brand: ctx.brand });
    if (!body) {
      throw new Error('the builder produced no output');
    }
    await deps.api.updateCampaign(id, campaignPayload(c, body));
  };

  // Sequential by design (one write at a time, evergreens paused one at a time): a promise
  // chain rather than await-in-a-loop.
  const sequential = (items, step) => items.reduce((p, item) => p.then(() => step(item)), Promise.resolve());

  await sequential(plan.campaigns, async (item) => {
    try {
      await saveCampaign(item.id);
      summary.saved.push(`c${item.id}`);
    } catch (e) {
      summary.failed.push({ ref: `c${item.id}`, error: errText(e) });
    }
    progress(item);
  });

  await sequential(evergreens, async (item) => {
    let paused = false;
    try {
      await deps.api.changeStatus(item.id, 'paused');
      paused = true;
      await saveCampaign(item.id);
      summary.saved.push(`c${item.id}`);
    } catch (e) {
      summary.failed.push({ ref: `c${item.id}`, error: errText(e) });
    } finally {
      if (paused) {
        try {
          await deps.api.changeStatus(item.id, 'running');
        } catch (e) {
          summary.leftPaused.push({ ref: `c${item.id}`, error: errText(e) });
        }
      }
    }
    progress(item);
  });

  await sequential(plan.templates, async (item) => {
    try {
      const t = await deps.api.getTemplate(item.id);
      const ctx = templateContext(t);
      const body = deps.compile(JSON.parse(t.body_source), { lang: ctx.lang, brand: ctx.brand });
      if (!body) {
        throw new Error('the builder produced no output');
      }
      await deps.api.updateTemplate(templatePayload(t, body));
      summary.saved.push(`t${item.id}`);
    } catch (e) {
      summary.failed.push({ ref: `t${item.id}`, error: errText(e) });
    }
    progress(item);
  });
  return summary;
}
