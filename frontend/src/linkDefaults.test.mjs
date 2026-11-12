// Fork (integrations EDITOR-DEFAULTS-SPEC I6). Run: node --test src/linkDefaults.test.mjs.
// campaignLinkDefault: null brand / no doc / non-identifier slug -> null; attribution true -> the
// bare shop-link expression of THAT brand (curated included); else From on curatedfor.you ->
// https://curatedfor.you, From on curatedby.you -> https://curatedby.you, otherwise
// https://curatedfor.you/<brand>; never the selector's brand (the function takes the campaign's).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { campaignLinkDefault } from './linkDefaults.mjs'; // eslint-disable-line import/extensions

const DOCS = [
  { brand: 'thirstygirl', usage: { attribution: true } },
  { brand: 'ruze', usage: { attribution: true } },
  { brand: 'shala', usage: { attribution: false } },
  { brand: 'younique', usage: { attribution: false } },
  { brand: 'curated', usage: { attribution: false } },
  { brand: 'nousage' },
  { brand: 'bad-slug', usage: { attribution: true } },
];

test('null brand, no document, documents not loaded -> null', () => {
  assert.equal(campaignLinkDefault(null, 'Shala <hello@shala.com>', DOCS), null);
  assert.equal(campaignLinkDefault('', 'Shala <hello@shala.com>', DOCS), null);
  assert.equal(campaignLinkDefault(undefined, '', DOCS), null);
  assert.equal(campaignLinkDefault('bemijade', 'BEMIJADE <hello@bemijade.co>', DOCS), null);
  assert.equal(campaignLinkDefault('shala', 'Shala <hello@shala.com>', []), null);
  assert.equal(campaignLinkDefault('shala', 'Shala <hello@shala.com>', undefined), null);
});

test('attribution true -> the bare shop link of THAT brand, no or fallback', () => {
  assert.equal(campaignLinkDefault('thirstygirl', 'Thirsty Girl <hello@thirstygirlhydration.com>', DOCS), '{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}');
  assert.equal(campaignLinkDefault('ruze', 'Ruze Pouches <hello@ruzepouches.com>', DOCS), '{{ .Subscriber.Attribs.shop_links.ruze.url }}');
  // Case-folded to the stored slug.
  assert.equal(campaignLinkDefault('Ruze', 'x <a@b.com>', DOCS), '{{ .Subscriber.Attribs.shop_links.ruze.url }}');
});

test('attribution true wins for a corporate brand too (curated once its shop links are configured)', () => {
  const docs = [{ brand: 'curated', usage: { attribution: true } }];
  assert.equal(campaignLinkDefault('curated', 'Curated <hello@curatedfor.you>', docs), '{{ .Subscriber.Attribs.shop_links.curated.url }}');
});

test('a shop-link brand whose slug is not a template identifier -> null', () => {
  assert.equal(campaignLinkDefault('bad-slug', 'x <hello@bad.com>', DOCS), null);
});

test('otherwise keyed on the From domain', () => {
  assert.equal(campaignLinkDefault('curated', 'Curated <hello@curatedfor.you>', DOCS), 'https://curatedfor.you');
  assert.equal(campaignLinkDefault('curated', 'hello@curatedfor.you', DOCS), 'https://curatedfor.you');
  assert.equal(campaignLinkDefault('shala', 'Creators <hello@curatedby.you>', DOCS), 'https://curatedby.you');
  assert.equal(campaignLinkDefault('shala', 'Shala <hello@shala.com>', DOCS), 'https://curatedfor.you/shala');
  assert.equal(campaignLinkDefault('younique', 'Younique <hello@younique.com>', DOCS), 'https://curatedfor.you/younique');
  // A document with no usage object reads as not configured.
  assert.equal(campaignLinkDefault('nousage', 'N <n@nousage.com>', DOCS), 'https://curatedfor.you/nousage');
  // No From (an unsettled derivation) -> the per-brand path.
  assert.equal(campaignLinkDefault('shala', null, DOCS), 'https://curatedfor.you/shala');
});

test('never another brand: the result names only the brand passed in', () => {
  DOCS.forEach((d) => {
    const out = campaignLinkDefault(d.brand, 'x <x@example.com>', DOCS);
    DOCS.filter((other) => out !== null && other.brand !== d.brand).forEach((other) => {
      assert.ok(!out.includes(`shop_links.${other.brand}.`) && !out.endsWith(`/${other.brand}`), `${d.brand} -> ${out}`);
    });
  });
});
