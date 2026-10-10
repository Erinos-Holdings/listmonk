// Fork (persona From, integrations PERSONA-FROM-SPEC I10). Run: yarn test:persona-from
// (node --test src/personaFrom.test.mjs). The Campaign page's sender picker: the brand From is
// first, a persona From is `<persona> <address>`, and a stored persona survives every arrival
// order of the lists, the server config and the brands rows.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  BRANDS_PENDING,
  BRANDS_LOADED,
  BRANDS_FAILED,
  personaFrom,
  findBrandRow,
  senderOptions,
  selectedPersona,
  displayOptions,
  acceptedFrom,
  pickerBrandRow,
  fromToApply,
  isSyncRepoint,
  campaignRefs,
  blockingCampaigns,
} from './personaFrom.mjs'; // eslint-disable-line import/extensions

const TG = {
  slug: 'thirstygirl',
  from_email: 'Thirsty Girl <hello@tg.example.test>',
  display_name: 'Thirsty Girl',
  address: 'hello@tg.example.test',
  personas: ['Natasha at Thirsty Girl', 'Jo at Thirsty Girl'],
};
const RUZE = {
  slug: 'ruze',
  from_email: 'Ruze <hello@ruze.example.test>',
  display_name: 'Ruze',
  address: 'hello@ruze.example.test',
  personas: ['Elizabeth at Ruze'],
};
const BARE = {
  slug: 'bare', from_email: 'hello@bare.example.test', display_name: 'hello@bare.example.test', address: 'hello@bare.example.test', personas: [],
};
const NAT = 'Natasha at Thirsty Girl <hello@tg.example.test>';
const JO = 'Jo at Thirsty Girl <hello@tg.example.test>';
const LIZ = 'Elizabeth at Ruze <hello@ruze.example.test>';

const mapped = (row) => ({
  error: null, brand: row.slug, fromEmail: row.from_email, unmapped: false,
});
const UNMAPPED = {
  error: null, brand: 'curated', fromEmail: 'Curated <hello@curated.example.test>', unmapped: true,
};
const ERRORED = {
  error: 'two brands', brand: null, fromEmail: null, unmapped: false,
};
// The server config has not arrived: an unmapped derivation with no From yet.
const UNSETTLED = {
  error: null, brand: 'curated', fromEmail: undefined, unmapped: true,
};

test('personaFrom composes <persona> <address>', () => {
  assert.equal(personaFrom('Natasha at Thirsty Girl', TG), NAT);
  assert.equal(personaFrom('Elizabeth at Ruze', RUZE), LIZ);
});

test('senderOptions puts the brand From first, then the personas in stored order', () => {
  assert.deepEqual(senderOptions(TG), [
    { value: TG.from_email, label: 'Thirsty Girl', persona: null },
    { value: NAT, label: 'Natasha at Thirsty Girl', persona: 'Natasha at Thirsty Girl' },
    { value: JO, label: 'Jo at Thirsty Girl', persona: 'Jo at Thirsty Girl' },
  ]);
  assert.deepEqual(senderOptions(BARE), [{ value: BARE.from_email, label: BARE.from_email, persona: null }]);
  // A row from an older server (no personas key) and no row at all.
  assert.deepEqual(senderOptions({ ...TG, personas: undefined }).map((o) => o.value), [TG.from_email]);
  assert.deepEqual(senderOptions(null), []);
});

test('selectedPersona names the persona a From carries, or null', () => {
  assert.equal(selectedPersona(NAT, TG), 'Natasha at Thirsty Girl');
  assert.equal(selectedPersona(TG.from_email, TG), null);
  assert.equal(selectedPersona(LIZ, TG), null);
  assert.equal(selectedPersona(NAT, null), null);
});

test('displayOptions keeps a From that is no longer an option visible', () => {
  assert.deepEqual(displayOptions(NAT, TG), senderOptions(TG));
  const gone = 'Kim at Thirsty Girl <hello@tg.example.test>';
  const opts = displayOptions(gone, TG);
  assert.equal(opts.length, 4);
  assert.deepEqual(opts[3], { value: gone, label: gone, persona: null });
  assert.deepEqual(displayOptions('', TG), senderOptions(TG));
});

test('acceptedFrom keeps a valid persona From and the brand From', () => {
  assert.equal(acceptedFrom(NAT, TG), NAT);
  assert.equal(acceptedFrom(JO, TG), JO);
  assert.equal(acceptedFrom(TG.from_email, TG), TG.from_email);
});

test('acceptedFrom repoints everything else to the brand From', () => {
  const cases = {
    'a bare address': 'hello@tg.example.test',
    'a foreign-brand persona': LIZ,
    'a removed persona': 'Kim at Thirsty Girl <hello@tg.example.test>',
    'a persona on another address': 'Natasha at Thirsty Girl <hello@other.example.test>',
    'a quoted variant': '"Natasha at Thirsty Girl" <hello@tg.example.test>',
    'a double space': 'Natasha at Thirsty Girl  <hello@tg.example.test>',
    'the default sender': UNMAPPED.fromEmail,
    'an empty From': '',
    'no From': undefined,
  };
  Object.entries(cases).forEach(([name, from]) => {
    assert.equal(acceptedFrom(from, TG), TG.from_email, name);
  });
});

test('findBrandRow and pickerBrandRow', () => {
  assert.equal(findBrandRow([RUZE, TG], 'thirstygirl'), TG);
  assert.equal(findBrandRow([RUZE], 'thirstygirl'), null);
  assert.equal(findBrandRow(null, 'thirstygirl'), null);
  assert.equal(findBrandRow([TG], null), null);

  assert.equal(pickerBrandRow(mapped(TG), TG, BRANDS_LOADED), TG);
  assert.equal(pickerBrandRow(mapped(TG), TG, BRANDS_PENDING), null);
  assert.equal(pickerBrandRow(mapped(TG), TG, BRANDS_FAILED), null);
  assert.equal(pickerBrandRow(mapped(TG), null, BRANDS_LOADED), null, 'no row for the brand');
  assert.equal(pickerBrandRow(UNMAPPED, TG, BRANDS_LOADED), null, 'unmapped: nothing to pick');
  assert.equal(pickerBrandRow(ERRORED, TG, BRANDS_LOADED), null);
  // A SQL-written from: tag that differs from the row: the exact match, no picker.
  const drifted = { ...mapped(TG), fromEmail: 'TG <hello@tg.example.test>' };
  assert.equal(pickerBrandRow(drifted, TG, BRANDS_LOADED), null);
});

test('fromToApply returns null while the brand row is pending -- whatever is stored', () => {
  [NAT, TG.from_email, 'hello@tg.example.test', LIZ, ''].forEach((stored) => {
    assert.equal(fromToApply(stored, stored, mapped(TG), null, BRANDS_PENDING), null, `load pass, stored ${stored}`);
    assert.equal(fromToApply(null, stored, mapped(TG), null, BRANDS_PENDING), null, `later pass, form ${stored}`);
  });
});

test('fromToApply returns null for an unsettled or errored derivation', () => {
  assert.equal(fromToApply(NAT, NAT, UNSETTLED, null, BRANDS_PENDING), null);
  assert.equal(fromToApply(NAT, NAT, UNSETTLED, TG, BRANDS_LOADED), null);
  assert.equal(fromToApply(NAT, NAT, ERRORED, TG, BRANDS_LOADED), null);
  assert.equal(fromToApply(NAT, NAT, null, TG, BRANDS_LOADED), null);
});

test('fromToApply returns the brand From when the brands fetch failed', () => {
  assert.equal(fromToApply(NAT, NAT, mapped(TG), null, BRANDS_FAILED), TG.from_email);
  assert.equal(fromToApply(null, NAT, mapped(TG), null, BRANDS_FAILED), TG.from_email);
  // ...and when the fetch succeeded but carries no row for the brand, or a row that disagrees
  // with the lists' from: tag.
  assert.equal(fromToApply(NAT, NAT, mapped(TG), null, BRANDS_LOADED), TG.from_email);
  const drifted = { ...mapped(TG), fromEmail: 'TG <hello@tg.example.test>' };
  assert.equal(fromToApply(NAT, NAT, drifted, TG, BRANDS_LOADED), 'TG <hello@tg.example.test>');
});

test('fromToApply needs no brands rows for an unmapped derivation', () => {
  [BRANDS_PENDING, BRANDS_LOADED, BRANDS_FAILED].forEach((state) => {
    assert.equal(fromToApply(NAT, NAT, UNMAPPED, null, state), UNMAPPED.fromEmail, state);
  });
});

test('fromToApply keeps the stored persona From on load when it is valid', () => {
  assert.equal(fromToApply(NAT, NAT, mapped(TG), TG, BRANDS_LOADED), NAT);
  assert.equal(fromToApply(JO, JO, mapped(TG), TG, BRANDS_LOADED), JO);
  assert.equal(fromToApply(TG.from_email, TG.from_email, mapped(TG), TG, BRANDS_LOADED), TG.from_email);
  // On the load pass the STORED value is judged, not whatever the form momentarily holds.
  assert.equal(fromToApply(NAT, UNMAPPED.fromEmail, mapped(TG), TG, BRANDS_LOADED), NAT);
  // A stored From that is not valid repoints.
  assert.equal(fromToApply('Kim at Thirsty Girl <hello@tg.example.test>', '', mapped(TG), TG, BRANDS_LOADED), TG.from_email);
  assert.equal(fromToApply('hello@tg.example.test', '', mapped(TG), TG, BRANDS_LOADED), TG.from_email);
});

test('fromToApply across the arrival orders ends on the stored persona', () => {
  // Each step is one pass of syncBrandDerivation as the page calls it: `pending` is the one-shot
  // load flag, consumed by the first pass that returns a value.
  const run = (steps) => {
    let form = NAT; // getCampaign copies the stored From into the form
    let pending = true;
    const applied = [];
    steps.forEach(([derivation, row, state]) => {
      const out = fromToApply(pending ? NAT : null, form, derivation, row, state);
      applied.push(out);
      if (out !== null) {
        pending = false;
        form = out;
      }
    });
    return { form, pending, applied };
  };

  // Lists + config first, brands last (the race Stage 2 F3 names).
  let r = run([[mapped(TG), null, BRANDS_PENDING], [mapped(TG), TG, BRANDS_LOADED]]);
  assert.deepEqual(r.applied, [null, NAT]);
  assert.equal(r.form, NAT);
  assert.equal(r.pending, false);

  // Brands first, config last.
  r = run([[UNSETTLED, null, BRANDS_LOADED], [mapped(TG), TG, BRANDS_LOADED]]);
  assert.deepEqual(r.applied, [null, NAT]);

  // Everything pending, then brands, then a re-run.
  r = run([[mapped(TG), null, BRANDS_PENDING], [mapped(TG), null, BRANDS_PENDING], [mapped(TG), TG, BRANDS_LOADED], [mapped(TG), TG, BRANDS_LOADED]]);
  assert.deepEqual(r.applied, [null, null, NAT, NAT]);

  // The fetch fails: the forcing that predates personas, once, on the load pass.
  r = run([[mapped(TG), null, BRANDS_PENDING], [mapped(TG), null, BRANDS_FAILED]]);
  assert.deepEqual(r.applied, [null, TG.from_email]);
  assert.equal(r.pending, false);
});

test('fromToApply after a list change to another brand is that brand\'s From', () => {
  // Later passes judge the form: a Thirsty Girl persona on a campaign now targeting Ruze.
  assert.equal(fromToApply(null, NAT, mapped(RUZE), RUZE, BRANDS_LOADED), RUZE.from_email);
  assert.equal(fromToApply('', NAT, mapped(RUZE), RUZE, BRANDS_LOADED), RUZE.from_email);
  // ...and back: the form now holds the Ruze From, which is not a Thirsty Girl option.
  assert.equal(fromToApply(null, RUZE.from_email, mapped(TG), TG, BRANDS_LOADED), TG.from_email);
  // A list change within the brand keeps a deliberate pick.
  assert.equal(fromToApply(null, NAT, mapped(TG), TG, BRANDS_LOADED), NAT);
  // To an unmapped list: the default sender.
  assert.equal(fromToApply(null, NAT, UNMAPPED, null, BRANDS_LOADED), UNMAPPED.fromEmail);
});

test('isSyncRepoint is false for a deliberate persona pick, true for a sync-forced change', () => {
  // Stored brand From, the user picks a persona: not a repoint.
  assert.equal(isSyncRepoint(TG.from_email, NAT, mapped(TG)), false);
  // Stored persona, the user picks another persona: not a repoint.
  assert.equal(isSyncRepoint(NAT, JO, mapped(TG)), false);
  // Nothing changed.
  assert.equal(isSyncRepoint(NAT, NAT, mapped(TG)), false);
  assert.equal(isSyncRepoint(TG.from_email, TG.from_email, mapped(TG)), false);
  // The sync forced the brand From over a stored value that is no longer accepted.
  assert.equal(isSyncRepoint('Kim at Thirsty Girl <hello@tg.example.test>', TG.from_email, mapped(TG)), true);
  assert.equal(isSyncRepoint('hello@tg.example.test', TG.from_email, mapped(TG)), true);
  // A list change to another brand forced that brand's From.
  assert.equal(isSyncRepoint(NAT, RUZE.from_email, mapped(RUZE)), true);
  // A new campaign (nothing stored) and an errored derivation never announce one.
  assert.equal(isSyncRepoint('', TG.from_email, mapped(TG)), false);
  assert.equal(isSyncRepoint(undefined, TG.from_email, mapped(TG)), false);
  assert.equal(isSyncRepoint(NAT, TG.from_email, ERRORED), false);
});

test('campaignRefs and blockingCampaigns', () => {
  const uses = [
    { name: 'Natasha at Thirsty Girl', campaigns: [{ id: 7, name: 'Launch', status: 'draft' }, { id: 9, name: 'Welcome', status: 'running' }] },
    { name: 'Jo at Thirsty Girl', campaigns: [] },
  ];
  assert.deepEqual(blockingCampaigns(uses, 'Natasha at Thirsty Girl').map((c) => c.id), [7, 9]);
  assert.deepEqual(blockingCampaigns(uses, 'Jo at Thirsty Girl'), []);
  assert.deepEqual(blockingCampaigns(uses, 'Unknown'), []);
  assert.deepEqual(blockingCampaigns(null, 'Jo at Thirsty Girl'), []);
  assert.equal(campaignRefs(uses[0].campaigns), '7 Launch (draft), 9 Welcome (running)');
  assert.equal(campaignRefs(undefined), '');
});
