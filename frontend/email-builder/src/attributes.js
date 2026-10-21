// Fork (attribute picker) -- integrations ATTRIBUTE-PICKER-SPEC D1-D4.
//
// The ONE roster of subscriber attributes the "Insert attribute" picker offers: the campaign
// subject and preheader (frontend/src/views/Campaign.vue), the richtext toolbar
// (RichtextEditor.vue) and the builder's Text ribbon, Heading, Button Url and Image link fields
// (App/.../helpers/inputs/AttributeMenuButton.tsx). A pick inserts `expr` exactly, through
// emojiInsert.js::insertEmoji (TinyMCE: its own insertContent).
//
// - `text` surfaces get the text group, URL surfaces the link group -- never both (D2).
// - The shop link's brand is the EDITING CONTEXT's brand (the campaign's derived brand, or a
//   template's brand column), never the global brand selector (D3).
// - The shop link is bare: no `or`, no fallback, no `index` (D4). An empty value falls back to
//   the brand site at click time (hazard 52); a template fallback cannot say "signed up before
//   launch".
// - The product link (integrations PRODUCT-LINK-SPEC, link group only) is the one item that
//   REPLACES the field: it rewrites the URL already typed (productRefUrl) to carry the
//   subscriber's referral slug.
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

function productLinkItem(brand, value) {
  const res = productRefUrl(value, brand);
  if (res.url === undefined) {
    return {
      id: 'product_link', label: 'Personalize product link', expr: '', disabled: true, reason: res.reason, replace: true,
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

// attributeMenu(kind, context) -> { items: [{ id, label, expr, disabled, reason, replace? }] }.
// kind is 'text' | 'link'; context is { brand, value? } (brand a slug, null or undefined; value
// the field's current text, for the product link). A disabled item is shown, never hidden, and
// has expr '' and a non-empty reason. `expr` is the exact string to insert, or -- when
// `replace` is true (only product_link) -- the exact string the field becomes; an item without
// `replace` inserts at the caret. An unknown kind returns no items.
export function attributeMenu(kind, context) {
  const brand = context ? context.brand : undefined;
  const value = context ? context.value : undefined;
  if (kind === 'text') {
    return { items: TEXT_ITEMS.map((it) => ({ ...it, disabled: false, reason: '' })) };
  }
  if (kind === 'link') {
    return {
      items: [
        shopLinkItem(brand),
        productLinkItem(brand, value),
        {
          id: 'unsubscribe', label: 'Unsubscribe link', expr: '{{ UnsubscribeURL }}', disabled: false, reason: '',
        },
        {
          id: 'message', label: 'View in browser', expr: '{{ MessageURL }}', disabled: false, reason: '',
        },
      ],
    };
  }
  return { items: [] };
}
