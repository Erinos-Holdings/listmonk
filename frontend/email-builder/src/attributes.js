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
//
// Adding an attribute = a row here + its pin in test/attributes.test.cjs (I1) (+ a text
// attribute's `attributes.<camelCasedId>` key in i18n/en.json -- the SPA's surfaces offer the
// text group only; a missing key falls back to `label`). No UI change.
//
// Plain ESM, dependency-free (I9): test/attributes.test.cjs evaluates it standalone in the
// builder's CI test step, and the SPA imports it by relative path.

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

// attributeMenu(kind, context) -> { items: [{ id, label, expr, disabled, reason }] }. kind is
// 'text' | 'link'; context is { brand } (a slug, null or undefined). A disabled item is shown,
// never hidden, and has expr '' and a non-empty reason. An unknown kind returns no items.
export function attributeMenu(kind, context) {
  const brand = context ? context.brand : undefined;
  if (kind === 'text') {
    return { items: TEXT_ITEMS.map((it) => ({ ...it, disabled: false, reason: '' })) };
  }
  if (kind === 'link') {
    return {
      items: [
        shopLinkItem(brand),
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
