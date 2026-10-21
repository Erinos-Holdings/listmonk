// Fork (attribute picker) -- integrations ATTRIBUTE-PICKER-SPEC D1-D4.
//
// The ONE roster of subscriber attributes the "Insert attribute" picker offers: the campaign
// subject and preheader (frontend/src/views/Campaign.vue), the richtext toolbar
// (RichtextEditor.vue) and the builder's Text ribbon, Heading, Button Url and Image link fields
// (App/.../helpers/inputs/AttributeMenuButton.tsx). A pick inserts `expr` exactly, through
// emojiInsert.js::insertEmoji (TinyMCE: its own insertContent).
//
// - `text` surfaces get the text group, URL surfaces the link group (D2); since 2026-10-21 the
//   Text block's ribbon is the one 'markdown' surface, text group + the two shop items.
// - The shop link's brand is the EDITING CONTEXT's brand (the campaign's derived brand, or a
//   template's brand column), never the global brand selector (D3).
// - The shop link is bare: no `or`, no fallback, no `index` (D4). An empty value falls back to
//   the brand site at click time (hazard 52); a template fallback cannot say "signed up before
//   launch".
// - The product link (integrations PRODUCT-LINK-SPEC; link and markdown groups) is the one item
//   that REPLACES the field (or, on 'markdown', the selection): it rewrites the URL already
//   typed (productRefUrl) to carry the subscriber's referral slug. On 'markdown' a selected
//   bare URL becomes plain text (never tracked); a selected `[text](url)` link becomes a
//   tracked markdown link (TEXT-LINK-TRACKING-SPEC D2, parseMarkdownLink).
//
// Adding an attribute = a row here + its pin in test/attributes.test.cjs (I1) (+ a text
// attribute's `attributes.<camelCasedId>` key in i18n/en.json -- the SPA's surfaces offer the
// text group only; a missing key falls back to `label`). No UI change.
//
// Plain ESM, dependency-free (I9): test/attributes.test.cjs evaluates it standalone in the
// builder's CI test step, and the SPA imports it by relative path.

// attributeI18nKey maps a roster id to the SPA's i18n key (`first_name` -> `attributes.firstName`);
// the SPA renders `$te(key) ? $t(key) : item.label` (D8). One mapping for every SPA surface.
export function attributeI18nKey(id) {
  return `attributes.${String(id).replace(/_([a-z])/g, (m, c) => c.toUpperCase())}`;
}

// isTemplateIdent is the one rule for the dotted `.Attribs.shop_links.<slug>.url` grammar.
export function isTemplateIdent(slug) {
  return typeof slug === 'string' && /^[A-Za-z0-9_]+$/.test(slug);
}

// shopLinkExpr returns the bare shop-link expression for a valid slug, null otherwise.
export function shopLinkExpr(brand) {
  if (!isTemplateIdent(brand)) {
    return null;
  }
  return `{{ .Subscriber.Attribs.shop_links.${brand}.url }}`;
}

// Fork (product link) -- integrations PRODUCT-LINK-SPEC D1/D2. productRefExpr is the referral
// slug with an EMPTY-STRING fallback: the `or` is load-bearing (a missing `slug` key would
// otherwise render `<no value>`, the resolver's KindEmpty, and the click would fall back to the
// brand homepage, losing the product page). The fallback is an EMPTY BACKQUOTED raw string
// (two backticks), NEVER a double-quoted "" -- runbook hazard 102: the builder's quote hack
// un-escapes &quot; inside {{ }} into a raw `"` mid-attribute, so TrackLink never wraps the
// href (untracked) and a Button's data-lm-vml-href marker breaks Outlook desktop. Pinned by
// models/tracking_test.go. Never a non-empty fallback -- that would be false attribution.
export function productRefExpr(brand) {
  if (!isTemplateIdent(brand)) {
    return null;
  }
  return `{{ or .Subscriber.Attribs.shop_links.${brand}.slug \`\` }}`;
}

const NO_BRAND_REASON = 'Choose a list first';
const badBrandReason = (brand) => `Brand slug ${brand} is not a template identifier`;
const NO_URL_REASON = 'Type or paste the product page URL first';

// productRefUrl(value, brand) -> { url } | { reason }. Strips every existing `ref` query param
// (any case, literal or templated) and appends `ref=<productRefExpr>` last, before any
// #fragment. Wholly textual (D2): parsing only locates the `#` and `?` boundaries; the path,
// the other params (in order) and the fragment are kept byte-identical -- never re-encoded
// through new URL(). Idempotent: the output is a fixed point. A pasted creator ref can never
// survive into a campaign.
export function productRefUrl(value, brand) {
  if (brand === null || brand === undefined || brand === '') {
    return { reason: NO_BRAND_REASON };
  }
  const expr = productRefExpr(brand);
  if (expr === null) {
    return { reason: badBrandReason(brand) };
  }
  const v = typeof value === 'string' ? value.trim() : '';
  // An absolute http(s) URL with a literal host: refuses empty, relative, other schemes and a
  // value that is only (or starts with) a template expression.
  if (!/^https?:\/\/[^\s/?#{}]+/i.test(v)) {
    return { reason: NO_URL_REASON };
  }

  const hashAt = v.indexOf('#');
  const fragment = hashAt >= 0 ? v.slice(hashAt) : '';
  const beforeHash = hashAt >= 0 ? v.slice(0, hashAt) : v;
  const qAt = beforeHash.indexOf('?');
  const base = qAt >= 0 ? beforeHash.slice(0, qAt) : beforeHash;
  const query = qAt >= 0 ? beforeHash.slice(qAt + 1) : '';

  const kept = query.split('&').filter((p) => {
    if (p === '') {
      return false;
    }
    const eq = p.indexOf('=');
    const name = eq >= 0 ? p.slice(0, eq) : p;
    return name.toLowerCase() !== 'ref';
  });
  const joined = kept.length > 0 ? `?${kept.join('&')}&` : '?';
  return { url: `${base}${joined}ref=${expr}${fragment}` };
}

// Fork (text link tracking) -- integrations TEXT-LINK-TRACKING-SPEC D2 (M1). The CONSERVATIVE
// markdown-link grammar the Text ribbon's product link personalizes in place: the whole
// selection (outer whitespace aside, kept verbatim) is `[label](dest)` or `[label](<dest>)`,
// `label` free of `[`, `]` and newline, `dest` (trimmed) an absolute http(s) URL free of `"`,
// `<`, `>`, newline and whitespace. Refused, deliberately: the CommonMark TITLE form
// (`[a](url "t")` -- a naive parse hands the title to the transform and the `"` makes the
// sanitizer DELETE the anchor and its label, hazard 102), nested brackets, multi-line, and an
// unbracketed dest holding a literal `(`/`)` (where CommonMark ends the destination is not ours
// to guess). The one widening, BOTH forms (amendment A1): whitespace is admitted within a
// `{{ ... }}` action carrying no `"` -- so transforming our own bracketed output is a fixed
// point, and the hand-typed UNBRACKETED form (which GFM autolinks into an unbalanced `{{` in an
// href, failing the save with `bad character U+0022`) is accepted and REPAIRED to the bracketed
// form. The action-interior class bars `"` AND `<`/`>` (A1 review M1: a `>` inside an action
// would end a CommonMark `<...>` destination early and recreate the U+0022 failure from the
// repair's own output). Returns { lead, label, dest, trail } or null.
const MD_LINK = /^(\s*)\[([^[\]\n]*)\]\(([^\n]*)\)(\s*)$/;

// matchMarkdownLink -- the link SHAPE alone (label rules, destination untouched). The markdown
// shop link (amendment A2) replaces the destination wholesale, so it needs no dest validation:
// accepting any destination is what lets a broken hand-built `[label]({{ ... }})` be repaired
// by selecting it and picking Shop link. Returns { lead, label, raw, trail } or null.
export function matchMarkdownLink(value) {
  if (typeof value !== 'string') {
    return null;
  }
  const m = MD_LINK.exec(value);
  if (m === null) {
    return null;
  }
  return {
    lead: m[1], label: m[2], raw: m[3], trail: m[4],
  };
}

export function parseMarkdownLink(value) {
  const m = matchMarkdownLink(value);
  if (m === null) {
    return null;
  }
  const raw = m.raw;
  let dest;
  if (raw.startsWith('<')) {
    if (!raw.endsWith('>') || raw.length < 2) {
      return null;
    }
    dest = raw.slice(1, -1).trim();
    if (/[\s"<>]/.test(dest.replace(/\{\{[^{}"<>]*\}\}/g, 'x'))) {
      return null;
    }
  } else {
    dest = raw.trim();
    if (/[\s"<>()]/.test(dest.replace(/\{\{[^{}"<>]*\}\}/g, 'x'))) {
      return null;
    }
  }
  if (dest === '' || !/^https?:\/\//i.test(dest)) {
    return null;
  }
  return {
    lead: m.lead, label: m.label, dest, trail: m.trail,
  };
}

const MARKDOWN_NO_URL_REASON = 'Select the product page URL, or a [text](url) link, first';

function productLinkItem(brand, value, noUrlReason) {
  // 'markdown' only (the caller passes noUrlReason): a selected markdown link is personalized
  // in place -- [label](<transformed dest>), angle-bracketed because the ref expression has
  // spaces (CommonMark refuses a spaced bare destination). Compiled, the anchor's href carries
  // the expression and TrackLink wraps it like a Button's (TEXT-LINK-TRACKING-SPEC §1, I1).
  const md = noUrlReason ? parseMarkdownLink(value) : null;
  if (md !== null) {
    const r = productRefUrl(md.dest, brand);
    if (r.url !== undefined) {
      return {
        id: 'product_link',
        label: `Personalize product link (${brand})`,
        expr: `${md.lead}[${md.label}](<${r.url}>)${md.trail}`,
        disabled: false,
        reason: '',
        replace: true,
      };
    }
  }
  const res = productRefUrl(value, brand);
  if (res.url === undefined) {
    const reason = noUrlReason && res.reason === NO_URL_REASON ? noUrlReason : res.reason;
    return {
      id: 'product_link', label: 'Personalize product link', expr: '', disabled: true, reason, replace: true,
    };
  }
  return {
    id: 'product_link',
    label: `Personalize product link (${brand})`,
    expr: res.url,
    disabled: false,
    reason: '',
    replace: true,
  };
}

const TEXT_ITEMS = [
  { id: 'first_name', label: 'First name (or "there")', expr: '{{ or .Subscriber.FirstName "there" }}' },
  { id: 'last_name', label: 'Last name', expr: '{{ .Subscriber.LastName }}' },
  { id: 'name', label: 'Full name', expr: '{{ .Subscriber.Name }}' },
  { id: 'email', label: 'Email address', expr: '{{ .Subscriber.Email }}' },
];

function shopLinkItem(brand) {
  if (brand === null || brand === undefined || brand === '') {
    return {
      id: 'shop_link', label: 'Shop link', expr: '', disabled: true, reason: 'Choose a list first',
    };
  }
  const expr = shopLinkExpr(brand);
  if (expr === null) {
    return {
      id: 'shop_link',
      label: 'Shop link',
      expr: '',
      disabled: true,
      reason: `Brand slug ${brand} is not a template identifier`,
    };
  }
  return {
    id: 'shop_link', label: `Shop link (${brand})`, expr, disabled: false, reason: '',
  };
}

// Amendment A2: on the markdown group the shop link is selection-aware, mirroring the product
// link. A selection matching the link SHAPE gets its destination REPLACED wholesale with
// `<{{ ...url }}>` -- the angle brackets carry the spaced expression (a bare one renders as
// literal text), and the compiled `href="{{ ...url }}"` is the Button shop link's
// production-proven shape (hazard 52). Any other selection keeps the caret-insert plain item.
function markdownShopLinkItem(brand, value) {
  const base = shopLinkItem(brand);
  const md = matchMarkdownLink(value);
  if (base.disabled || md === null) {
    return base;
  }
  return {
    ...base,
    expr: `${md.lead}[${md.label}](<${base.expr}>)${md.trail}`,
    replace: true,
  };
}

// attributeMenu(kind, context) -> { items: [{ id, label, expr, disabled, reason, replace? }] }.
// kind is 'text' (subject/preheader, Heading, richtext), 'link' (Button Url, Image link) or
// 'markdown' (the Text block's ribbon); context is { brand, value? } (brand a slug, null or
// undefined; value the field's current text for 'link', the SELECTION for 'markdown' -- the
// product link transforms it). A disabled item is shown, never hidden, and has expr '' and a
// non-empty reason. `expr` is the exact string to insert, or -- when `replace` is true (only
// product_link) -- the exact string the field (or the selection, on 'markdown') becomes; an
// item without `replace` inserts at the caret. An unknown kind returns no items.
//
// 2026-10-21 (user): the link group lost `{{ UnsubscribeURL }}` / `{{ MessageURL }}` (the
// official footers carry both; authors never place them), and 'markdown' adds the shop and
// product links to the Text block. Inserted bare, they render as PLAIN TEXT and are never
// tracked (a client that linkifies one produces an untracked click).
//
// 2026-10-21 (TEXT-LINK-TRACKING-SPEC D2): a selected `[text](url)` markdown link IS the
// tracked Text route -- the product link rewrites it to `[text](<url?ref=…>)` (the angle
// brackets carry the spaced expression), which compiles to an anchor TrackLink wraps exactly
// like a Button's. A hand-typed double-quoted expression in a markdown link is worse than
// untracked: the sanitizer deletes the anchor AND its label (runbook hazard 102). The shop
// link mirrors it (amendment A2): a selected `[text](anything)` becomes
// `[text](<{{ ...url }}>)`, the destination replaced wholesale.
export function attributeMenu(kind, context) {
  const brand = context ? context.brand : undefined;
  const value = context ? context.value : undefined;
  const text = TEXT_ITEMS.map((it) => ({ ...it, disabled: false, reason: '' }));
  if (kind === 'text') {
    return { items: text };
  }
  if (kind === 'link') {
    return { items: [shopLinkItem(brand), productLinkItem(brand, value)] };
  }
  if (kind === 'markdown') {
    return {
      items: [
        ...text,
        markdownShopLinkItem(brand, value),
        productLinkItem(brand, value, MARKDOWN_NO_URL_REASON),
      ],
    };
  }
  return { items: [] };
}
