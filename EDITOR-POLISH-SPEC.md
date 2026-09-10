# Editor Polish Spec — sticky toolbar, Templates save-in-place, blockquote

Status: blind spec review complete and adjudicated (see git history for the review).
Ready to commit and hand off for implementation.

Three independent, low-risk UI fixes to the listmonk fork, requested by the user from
screenshots, plus one small persistence change (D4) that rides on D2's Templates form
work. Bundled into one spec because two of the three touch the same file
(`MarkdownContentInput.tsx`), D4 shares D2's save path, and all are small enough that a
shared release/test pass covers them together. Each item ships and reverts independently — a rejection or
revert of one item must not block the other two.

## Table of Contents

- [D1 — Sticky rich-text toolbar](#d1--sticky-rich-text-toolbar)
- [D2 — Templates "Save changes" (in place, no close)](#d2--templates-save-changes-in-place-no-close)
- [D3 — Blockquote button](#d3--blockquote-button)
- [D4 — Templates: persist the selected Brand swatch](#d4--templates-persist-the-selected-brand-swatch)
- [Non-goals](#non-goals)
- [Release gates](#release-gates)
- [Docs](#docs)

## Background

The "rich text ribbon" in all three screenshots is the Text-block content editor in the
visual campaign/template builder. This is **not** the Vue frontend
(`frontend/src`) — it is the vendored React sub-project `frontend/email-builder/`
(forked from `@usewaypoint/email-builder`, built to
`frontend/public/static/email-builder/email-builder.umd.js`, loaded into an iframe by
`frontend/src/components/VisualEditor.vue`). All email-builder edits follow the existing
pattern in that sub-project: pure, dependency-free transform functions in
`markdownFormat.ts` (isolated so `frontend/email-builder/test/*.test.cjs` can transpile
and run them standalone — see the file's own header comment), wired into the toolbar
`Stack` in `MarkdownContentInput.tsx`.

Image 3 (the Slack-style quote block with a rounded vertical bar) is a reference for the
*visual target* of the rendered blockquote, not a literal screenshot of listmonk — no
functional requirement should be inferred from its Slack-specific chrome (avatar,
timestamp, keyboard-shortcut tooltip) beyond "indent + rounded left bar."

## D1 — Sticky rich-text toolbar

**Problem.** The toolbar `Stack`
(`MarkdownContentInput.tsx:132-136`, containing Bold/Italic/Underline/Link/Text
color/Bullet list) is a normal in-flow element above the `TextField` textarea
(`MarkdownContentInput.tsx:163`). Both live inside `ConfigurationPanel`, which is
rendered inside a scrollable container: `InspectorDrawer/index.tsx:93` —
`<Box sx={{ ..., overflow: 'auto' }}>`. For a long text block, scrolling down inside that
drawer to reach later paragraphs scrolls the toolbar out of view along with the content
above it, so formatting the middle or end of a long block requires scrolling back up
first.

**Decision.** Make the toolbar `Stack` sticky to the top of its own scroll context, not
to the drawer's scroll context, so it is visible on-screen for as long as the textarea
it controls is in view.

- The `TextField` itself does not scroll internally (it is a `multiline` autosizing
  textarea, `minRows={rows}`, that grows with content — the *drawer* scrolls, not the
  textarea). Sticky positioning must therefore be evaluated against the drawer's
  scroll container (`InspectorDrawer/index.tsx:93`), the actual scrolling ancestor,
  and verified to stay pinned while scrolling past a text block whose rendered height
  exceeds the drawer's viewport.
- Implementation: `position: sticky; top: 0` plus an opaque background (it already has
  `bgcolor: 'grey.100'`) and a `zIndex` above the `TextField` label/outline, added to
  the `sx` on `MarkdownContentInput.tsx:132-136`. No changes to `InspectorDrawer`
  itself are anticipated — sticky positioning relative to the nearest scrolling
  ancestor requires no cooperation from that ancestor beyond already being
  `overflow: auto`, which it is.
- Scope: this ribbon only. Other scrollable panels in the drawer (Style tab, other
  block types) are untouched.
- **Confirmed against the real DOM nesting**: drawer `Box` with `overflow: auto`
  (`InspectorDrawer/index.tsx:93`) → `BaseSidebarPanel`'s `Box p={2}` → `Stack
  spacing={5}` → `MarkdownContentInput`'s wrapper `Box` (`:131`) → the toolbar `Stack`.
  No intermediate sets `overflow`, `transform`, or `contain`, so the sticky containing
  block resolves to the wrapper `Box` at `MarkdownContentInput.tsx:131` as intended —
  the toolbar unpins once the textarea has fully scrolled past that `Box`, which is
  the correct behavior for I1. Two implementation notes: `top: 0` pins flush to the
  drawer edge while the panel has 16px of padding, so the pinned bar sits slightly
  differently than its in-flow position (cosmetic — call it out in the manual check);
  `zIndex: 1` is sufficient to clear the `TextField`'s floated label.

**Invariant I1.** Scrolling a long text block inside the Inspect panel keeps the
toolbar pinned to the top of the panel, never scrolling out of view while the textarea
below it is still partially visible.
— *Reason untestable by the existing suite:* `frontend/email-builder/test/*.test.cjs`
run transform functions in Node against no DOM/layout, and the project has no
browser/visual regression harness for the React sub-project (confirmed: no
Cypress/Playwright coverage of the email-builder iframe). This is a CSS/layout
property, not a pure-function one. **Verification is manual**: open the visual editor,
add a text block with enough paragraphs to overflow the Inspect panel, scroll, confirm
the toolbar stays pinned and no button is visually obscured or clipped by the sticky
bar overlapping the `TextField` outline/label on first scroll.

## D2 — Templates "Save changes" (in place, no close)

**Problem.** `TemplateForm.vue` renders a modal footer
(`TemplateForm.vue:88-95`) with a "Close" button
(`$parent.close()`, line 89) beside a "Save" button
(`$t('globals.buttons.save')`, `native-type="submit"`, lines 92-94). Both
`createTemplate()` and `updateTemplate()` (lines 189-221) call `this.$parent.close()` on
a successful save (lines 201, 218) — so Save always closes the modal, even though a
dedicated Close button already exists for that. `Campaign.vue` has the pattern the user
wants: a "Save changes" button (`globals.buttons.saveChanges`, `Campaign.vue:47/332`,
also bound to Ctrl+S) that calls `updateCampaign()`, which PUTs and replaces
`this.data` / toasts, with **no navigation and no window close** (`Campaign.vue:1247-1320`,
confirmed no `close()`/`$router.push` call in that path).

There is no shared component between the two — Templates uses a Buefy modal with
generic `globals.buttons.save`/`close` i18n keys; Campaigns is a standalone page with
its own dedicated button markup. This is two separate, independent edits, not a shared
component change.

**Decision.**
- Relabel the Templates save button from `globals.buttons.save` to the existing
  `globals.buttons.saveChanges` key (`i18n/en.json:223`, already present — no new i18n
  key needed).
- Remove the `this.$parent.close()` call from both `createTemplate()` (line 201) and
  `updateTemplate()` (line 218), so a save updates `this.data`/toasts and leaves the
  modal open, matching Campaigns' post-save behavior. The user still dismisses the
  modal with the existing Close button.
- **`createTemplate()` needs a parent-owned state flip, not a child-side field write.**
  The create-vs-update branch is the `isEditing` **prop** (`TemplateForm.vue:180-187`),
  owned by `Templates.vue` and set by its `showNewForm`/`showEditForm`
  (`Templates.vue:148-161`) — not `this.data.id`. `data` is itself a prop, so writing
  `this.data.id` in the child would mutate the parent's `curItem` object without
  flipping `isEditing`, and a second Save would still call `createTemplate()` and
  duplicate the record. The child's `form` is a mounted-time copy of the prop
  (`TemplateForm.vue:344`) and is not re-synced on prop change, so form content
  survives a prop flip safely. **Fix:** on a successful create, the child emits the
  created record (`this.$emit('created', d)`); the parent's handler sets
  `curItem = d` and `isEditing = true` (and still calls `getTemplates()`). This also
  fixes the header text and the Type `<select>`, both of which are gated on `isEditing`
  (`TemplateForm.vue:10-18, 30`) and would otherwise keep reading "New template" with
  Type still editable after the first save.
- No change to `$emit('finished')` — `Templates.vue`'s `formFinished` handler only
  calls `getTemplates()` (`Templates.vue:163-165`) and does not close the modal, so
  removing the two `$parent.close()` calls is sufficient for "stays open." The
  remaining Close button (line 89) is untouched.

**Invariant I2a.** Clicking Save on the Templates form persists the change and leaves
the modal open (verified: no `close()` call fires from either success handler).
— *Reason untestable:* `frontend/` has no component-test harness (no Jest/Vitest
config, no test script in `frontend/package.json`); the only UI coverage is Cypress
e2e (`frontend/cypress/e2e/`), which is not run by `.github/workflows/build-image.yml`
or the Makefile. **Manual verification**: save a template, confirm the modal stays
open and the toast/data update as expected.

**Invariant I2b.** A second Save on a template created earlier in the same modal
session updates the existing record rather than creating a duplicate.
— *Reason untestable:* same as I2a. **Manual verification**: create a new template,
save, edit content again, save a second time, confirm via the templates list that
only one record exists and the second save's content landed on it.

**Existing coverage this changes.** `frontend/cypress/e2e/templates.cy.js`'s "Edits
template" case (`:35-44`) clicks Save and then reads the table, assuming the modal has
closed. With the modal staying open, the table read likely still passes because
`getTemplates()` refreshes it in the background regardless — but if this suite is ever
wired into CI, that case must be updated to close the modal explicitly before its
assertion rather than relying on that side effect. The "Clones" cases use a separate
clone-prompt modal and are unaffected.

## D3 — Blockquote button

**Problem.** The toolbar has Bold/Italic/Underline/Link/Text color/Bullet list but no
blockquote. `<blockquote>` is already in the sanitizer's `ALLOWED_TAGS`
(`@usewaypoint/block-text`, vendored dep,
`node_modules/@usewaypoint/block-text/dist/index.js:78`) and `marked` (with `gfm: true`)
already parses Markdown `> ` blockquote syntax into it — so the Markdown→HTML pipeline
requires no new allowlisting. Two gaps remain: (1) no toolbar button emits `> ` syntax,
and (2) the rendered `<blockquote>` has no styling at all today — no patch, no inline
style, so it would currently render with only the browser/email-client default margin,
not the rounded-corner vertical bar the user wants (confirmed: no `blockquote` handling
anywhere in `block-text`'s `CustomRenderer` or CSS, and no override in the fork's own
`patches/@usewaypoint+block-text+0.0.6.patch`, which currently only extends font-family
options).

**Decision — editor-side (input transform).**
- Add `applyBlockquote(text, start, end): FormatResult` to `markdownFormat.ts`,
  modeled on `applyBulletList` (same file, block-selection + per-line prefix pattern):
  expand the selection to whole lines, prefix each non-blank line with `> ` unless
  already prefixed (idempotent, matching the bullet-list toggle-safe behavior), same
  trailing-newline handling as `applyBulletList`.
- Add a 7th `toolbarButton(...)` call to the `Stack` in `MarkdownContentInput.tsx`
  (after "Bullet list", `MarkdownContentInput.tsx:158-161`) using `FormatQuoteOutlined`
  from `@mui/icons-material` — confirmed present in the installed version (5.16.7),
  same "Outlined" family already imported there.
- Import `applyBlockquote` alongside the other transform imports
  (`MarkdownContentInput.tsx:20`).

**Decision — render-side (email-safe styling).**
- Patch `@usewaypoint/block-text`'s `CustomRenderer` (the `marked.Renderer` subclass at
  `dist/index.js`, same file/pattern the existing `table`/`link` overrides live in) to
  add a `blockquote(quote)` override that wraps the rendered content in inline-styled
  markup — a left border plus rounded corners plus left padding/indent, e.g.
  `border-left: 3px solid <color>; border-radius: 4px; padding-left: 12px; margin: 0;`
  — rather than relying on unstyled browser defaults. **Inline styles only** (this is
  compiled HTML mail — no external stylesheet reaches an email client; every other
  renderer override and hazard 53/54 in the listmonk runbook fork section establish
  inline-only as the working constraint for this codebase).
- Ship the change the same way the existing font-family extension is shipped: edit
  `node_modules/@usewaypoint/block-text/dist/{index.js,index.mjs}` and regenerate
  `patches/@usewaypoint+block-text+0.0.6.patch` via `npx patch-package
  @usewaypoint/block-text` (mirroring the existing patch's two-file shape — CJS and
  ESM builds must both be patched, or one module system silently keeps the unstyled
  renderer). Do not hand-edit the patch file directly without regenerating it from a
  working `node_modules` edit — patch-package's diff format is easy to get subtly
  wrong by hand.
- **The two-file patch shape is required, not just mirrored for consistency**: Vite
  resolves `@usewaypoint/block-text` through its `module`/`exports.import` entry, so
  the built UMD bundle (and therefore the browser-rendered builder) uses `index.mjs`,
  while the Node test suite and `@usewaypoint/email-builder` (which imports, not
  bundles, `block-text` — no third patch needed) use `index.js`. Patching only one
  file leaves the other resolution path unstyled. The installed `marked` is `12.0.2`,
  so the renderer override signature is `blockquote(quote: string)` (a string, not the
  token object `marked` 13+ introduced) — pin that signature so a future `marked`
  major-version bump is a visible break, not a silent one. `style` is already in
  `GENERIC_ALLOWED_ATTRIBUTES` for every allowlisted tag including `blockquote`
  (`dist/index.js:116-121`), so no sanitizer change is needed. The Makefile re-runs
  `patch-package` before every builder build and CI's `postinstall` does the same, so
  the patch reaches the built image without a separate step.
- Color: use a fixed value consistent with the rest of the ribbon's neutral chrome
  (the toolbar itself is `bgcolor: 'grey.100'`) rather than inheriting the block's text
  color or introducing a new user-facing color control — this is a formatting glyph,
  not a themeable block property. Exact hex is an implementation choice for the
  builder to make and note in the PR, not a spec-fixed value.
- **No downlevel-revealed / Outlook-safety review needed beyond the inline-style
  constraint above** — a `<blockquote>` with inline border/padding is plain HTML, not
  a VML or conditional-comment construct, so hazard 53 ("no downlevel-revealed
  conditional comments") does not apply here. Confirmed:
  `no-downlevel-revealed.test.cjs` builds its own fixtures (`inlineButton`,
  `fullWidthButton`, `wideImage`) and asserts only on conditional-comment/`mso-hide`
  structure — it does not feed generated text-block content through the pipeline, so
  it cannot false-positive on this change. Related: `outlook.ts`'s Word font-fallback
  wrapper already treats `BLOCKQUOTE` as a text-flow tag (`outlook.ts:438`), so a
  quoted block keeps its existing font-fallback behavior with no change needed there.

**Invariant I3a.** Selecting a paragraph and clicking the blockquote button prefixes
every non-blank line of the selection with `> `, idempotently (re-clicking an
already-quoted block does not double-prefix), matching the bullet-list button's
existing selection/idempotency contract.
— *Test:* `frontend/email-builder/test/markdown-format.test.cjs` — add cases for
`applyBlockquote` alongside the existing `applyBulletList` coverage
(`markdown-format.test.cjs:108-121`), matching that coverage's exact contract:
collapsed caret, whole-line expansion, idempotence, and — the non-obvious part of
`applyBulletList`'s guard — a selection ending just past a trailing newline (e.g.
triple-click, shift+down) must not drag the next line into the quoted block.

**Invariant I3b.** Markdown containing `> quoted text` compiles to a `<blockquote>`
element carrying the inline border/radius/padding style, and is not stripped or
altered by the sanitizer.
— *Test:* `renderMarkdownString` is internal to `block-text` and not exported (the
package exports only `Text`, `TextPropsDefaults`, `TextPropsSchema` —
`dist/index.js:61-66`), so it cannot be called directly from a test. Follow
`test/font-family-parity.test.cjs`'s existing pattern instead: render the exported
`Text` component via `renderToStaticMarkup(React.createElement(Text, { style, props:
{ markdown: true, text: '> quoted' } }))` and assert the output contains
`<blockquote style="…"` with the expected border/radius/padding substring. Put this in
a new `test/blockquote-render.test.cjs` — `markdown-format.test.cjs` is scoped to the
input transforms only (per its own header comment) and transpiles standalone via a
require-map that would reject the `block-text` import. `test/run.cjs` compiles
`outlook.ts` and then runs every `*.test.cjs` in the directory, so a new file needs no
runner change to be picked up.

## D4 — Templates: persist the selected Brand swatch

**Problem.** The Templates form's "Brand swatches" dropdown (`TemplateForm.vue:56-66`,
`brandSlug` in `data()`) is session state only: it drives the swatch row and the rebrand
sweep while the modal is open, and is forgotten on close. Reopening a template starts at
"— None —", so every edit session re-picks the brand before the swatches (and the
sweep's provenance seed) are available. Templates have nowhere to keep it today: the
`templates` table (`schema.sql:103-114`, `models/templates.go:21-36`) has no brand
column, and stashing it inside `body_source` is not an option — that column is the
builder document, validated as a strict record of blocks
(`frontend/email-builder/src/documents/editor/core.tsx:132`,
`EditorConfigurationSchema = z.record(z.string(), EditorBlockSchema)`), so any non-block
key fails parsing and the template would not open.

**Decision.** Add a `brand` column to templates and round-trip it through the form, so
"Save changes" persists the dropdown's current selection (including clearing it) and
opening the template restores it.

- **Schema:** fork migration `internal/migrations/v6.2.8.go` (registered in
  `cmd/upgrade.go` after `v6.2.7`, same idempotent `IF NOT EXISTS` shape as `V6_2_7`):
  `ALTER TABLE templates ADD COLUMN IF NOT EXISTS brand TEXT NOT NULL DEFAULT ''`. Mirror
  it in `schema.sql` (fresh installs) — the fork keeps schema.sql and the migration in
  parity (`campaign_send_failures` is in both). Empty string means "no brand" (the
  dropdown's None sentinel is already `''`), so no NULL handling anywhere.
- **Model / queries / handlers:** a `Brand string` field (db/json tag `brand`) on
  `models.Template`; `get-templates` selects it unconditionally (it is metadata, not
  body — the `noBody` case must still return it, because `Templates.vue:148-152`
  hands the LIST row straight to the edit form as `curItem`); `create-template` and
  `update-template` take it as a new positional parameter; `core.CreateTemplate` /
  `core.UpdateTemplate` and the two handlers in `cmd/templates.go` (`:133`, `:174`)
  thread it through. `validateTemplate` (`cmd/templates.go:214`) accepts `''` or a value
  matching `models.ReBrandSlug` (the same regex list tags and the theme proxy use —
  `cmd/campaigns_brand.go:71`), lowercase-folded, and rejects anything else — the
  frontend roster is already folded (`TemplateForm.vue` `brandRoster`), so this only
  guards direct API callers.
- **`update-template` writes `brand=$N` unconditionally — NOT behind the file's
  `CASE WHEN $N != ''` guard.** Clearing the dropdown back to None must persist as `''`.
  The guard pattern in that query (`queries/templates.sql:16-19`) is exactly the
  silent-no-op hazard recorded for subscriber names in the integrations ci file
  (CONTACT-READINESS-SPEC D16): an empty value returns 200 and keeps the old one.
- **Frontend:** `form.brand` is included in both `createTemplate()` and
  `updateTemplate()` payloads (`TemplateForm.vue:190-197`, `207-214`), sourced from
  `brandSlug`. On `mounted()`, after `this.form = { ...data }`, set
  `this.brandSlug = data.brand || ''`; the existing `brandSlug` watcher then runs
  `onBrandPick` → theme fetch → swatch row, with no new code path. This interacts with
  D2's create-path fix: if the parent replaces `curItem` after a create, the child's
  `brandSlug` is local state and is unaffected, and the created record's `brand` is
  whatever the child just sent — no re-sync needed. Only the
  `campaign_visual` type renders the dropdown; other types save `''`.
- **Rebrand-sweep consequence (design change, intentional).** `heldBrandPalette`'s
  comment (`TemplateForm.vue:156-164`) says a fresh open has no provenance, so a
  rebrand is a two-step gesture (pick the original brand, then the target). With the
  brand persisted, opening a saved template seeds provenance from the stored brand
  (`maybeOfferBrandSweep` with `bodySource` present and `heldBrandPalette` null takes
  the silent-seed branch, `:271-274` — no prompt on open), so a rebrand becomes
  one-step: pick the new brand, get the prompt. Update that comment; the behavior is
  what the persisted value is for. Templates saved before this change carry `''` and
  keep the two-step gesture until their first save with a brand.
- **Clone:** `Templates.vue::cloneTemplate` (`:170-182`) builds its own payload; include
  `brand: t.brand` so a clone keeps the source's brand.
- **Non-goal:** the campaign editor is untouched — a campaign's brand is DERIVED from its
  target lists (`Campaign.vue:1453-1514`), and a template's stored brand does not flow
  into campaigns created from it. This column exists for the template editor's swatches
  and sweep only.

**Invariant I4a.** Saving a template (create or update) stores the dropdown's current
brand, and saving with None stores `''` (a cleared brand is not silently kept).
— *Test:* a DB-backed case in `internal/migrations/` following the
`LISTMONK_TEST_PG`-gated harness (`evergreen_db_test.go` header): build the scratch DB
from `schema.sql`, run `V6_2_8` twice (idempotency), then execute the real goyesql
`create-template` / `update-template` statements — insert with `brand='liyora'`, update
to `''`, assert the row reads `''`. Also extend `evergreen_db_test.go`'s migration
ladder (`:85-104`) with `V6_2_8`, as every prior fork migration is.

**Invariant I4b.** Reopening a saved template restores the dropdown to the stored brand
and shows its swatches without user action.
— *Reason untestable:* Vue view behavior; no component-test harness in `frontend/`
(Cypress e2e is not run in CI). **Manual verification** in the dev suite: save a visual
template with a brand, close, reopen, confirm the dropdown and swatch row; then set
None, save, reopen, confirm None.

**Invariant I4c.** `brand` on the templates API accepts `''` or a `ReBrandSlug` match and
rejects anything else.
— *Test:* table-driven unit test beside `cmd/lists_brand_test.go` exercising
`validateTemplate` with `''`, `liyora`, `Liyora` (accepted, folded), `bad slug`
(rejected).

## Non-goals

- No change to which HTML tags the sanitizer allows (`blockquote` is already
  allowlisted).
- No user-facing color picker or style control for the blockquote — one fixed style,
  not a themeable block property (see D3 rationale above).
- No change to the Campaigns save button, only Templates being brought in line with
  it.
- No sticky-toolbar treatment for other input types/panels in the Inspect drawer —
  scoped to the Text-block content ribbon only.
- No visual/browser regression test harness is being introduced for the email-builder
  React sub-project as part of this work, even though D1 and I3b would benefit from
  one — flagged as a gap, not fixed here.

## Release gates

- `frontend/email-builder`: `npm test` (or the sub-project's equivalent — confirm
  exact script, `package.json` shows `"test": "node test/run.cjs"`) passes with the
  new `markdown-format.test.cjs` cases green.
- Manual verification of I1 (sticky toolbar) and the Templates modal-stays-open
  behavior (I2a/I2b if no automated harness exists) in a local dev build before
  release — see the runbook's Local development section for the dev docker suite.
- `frontend/src` typecheck/lint (Vue) passes for the `TemplateForm.vue` change.
- `go test ./internal/...` passes (CI runs it before the image build); the I4a DB case is
  run locally against the dev suite's Postgres (`LISTMONK_TEST_PG`) before release, as
  it is opt-in and CI has no database.
- D4 adds a fork migration, so the release is NOT frontend-only: the image runs
  `v6.2.8` on upgrade. The migration is additive with a default, so rollback to the
  prior image is safe (the column is simply ignored).
- **Deploy path**: this ships through the listmonk fork's own pipeline, not any
  integrations-repo stack (Haiku, EPO, etc. are unrelated) — `.github/workflows/build-image.yml`
  builds on push to `feat/**`. Per the runbook's branch-convention row ("per-feature
  branching remains the rule for separable backend/builder features"), this is its own
  branch, **`feat/editor-polish`**, cut from the tip of the stacked release branch
  (`feat/campaign-52-hardening`, erinos.87+) rather than landing directly on it — this
  work is unrelated to campaign-52 hardening. Committing the spec lands on
  `feat/editor-polish`, not `main` and not the stacked release branch. CI running on
  that branch produces a `v6.2.0-erinos.N` image; the pin in
  `listmonk/host/docker-compose.yml` is bumped once it's ready to deploy — whether that
  merges back into the stacked release branch first or deploys directly is a decision
  for whoever lands this, not fixed here. See the runbook's fork table (Branch
  convention, Image, Tag scheme rows) for the exact procedure.
- **Existing content is unaffected by D3 at release time.** The fork's rule that "a
  builder change reaches stored *visual* bodies only on re-save" applies here: already
  compiled campaigns/templates will not gain blockquote styling until re-saved
  (`scripts/resave-listmonk-visual.ts`). Not a blocker — no existing content uses `> `
  syntax today — but worth stating so it isn't mistaken for a bug later.

## Docs

On landing, add a one-line pointer (not the implementation detail) to the listmonk
runbook's fork section noting: the blockquote button exists, its styling lives in a
`patch-package` patch on `@usewaypoint/block-text` (not in-repo source) so it must be
regenerated if that dependency is ever upgraded, and the sticky-toolbar CSS lives in
`MarkdownContentInput.tsx`; and that `templates.brand` (fork migration v6.2.8) is
editor-only metadata — it drives the Templates form's swatches and rebrand sweep and does
not feed campaign brand derivation. This is the kind of non-obvious coupling (a styling
decision that lives in a vendored-dependency patch rather than first-party source) the
Runbook Retirement Policy's "one canonical home per invariant" rule calls for
surfacing — do not duplicate the patch's diff content into the runbook, just point at
it.
