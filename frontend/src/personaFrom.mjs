// Fork (persona From, integrations PERSONA-FROM-SPEC D5). The Campaign page's "Sender display
// name" picker, pure half: which From values a campaign may carry for its brand, and which one the
// page should apply when the lists, the server config and the brands rows arrive in any order.
// No Vue, no store; tested by personaFrom.test.mjs (node --test).
//
// The server is the control (models.IsPersonaFrom): these only shape the form. Nothing here
// validates a persona NAME -- the Add control posts the name and shows the server's message.
//
// A brand row is the GET /api/brands shape, verbatim:
//   { slug, from_email, display_name, address, personas: [] }
// and a derivation is Campaign.vue's brandDerivation: { error, brand, fromEmail, unmapped }.

// The three states of the page's one GET /api/brands.
export const BRANDS_PENDING = 'pending';
export const BRANDS_LOADED = 'loaded';
export const BRANDS_FAILED = 'failed';

function personasOf(brandRow) {
  return brandRow && Array.isArray(brandRow.personas) ? brandRow.personas : [];
}

// personaFrom composes the canonical campaign From of a persona: `<persona> <address>`
// (models.PersonaFrom in the Go backend -- a From composed any other way is refused with a 400).
export function personaFrom(name, brandRow) {
  return `${name} <${brandRow.address}>`;
}

// findBrandRow is the row for a slug, or null.
export function findBrandRow(brands, slug) {
  if (!Array.isArray(brands) || !slug) {
    return null;
  }
  return brands.find((b) => b && b.slug === slug) || null;
}

// senderOptions are the picker's options: the brand From first, then the brand's personas in
// stored order. `persona` is null on the brand's own entry.
export function senderOptions(brandRow) {
  if (!brandRow) {
    return [];
  }
  return [
    { value: brandRow.from_email, label: brandRow.display_name, persona: null },
    ...personasOf(brandRow).map((p) => ({ value: personaFrom(p, brandRow), label: p, persona: p })),
  ];
}

// selectedPersona is the persona a From names for this brand, or null (the brand From, or
// anything that is not one of the brand's options).
export function selectedPersona(from, brandRow) {
  const hit = senderOptions(brandRow).find((o) => o.value === from);
  return hit ? hit.persona : null;
}

// displayOptions is senderOptions plus, when the current From is none of them, that From as a
// last entry -- so a campaign that can no longer be edited (finished, with a persona since
// removed) still shows what it sent as.
export function displayOptions(current, brandRow) {
  const opts = senderOptions(brandRow);
  if (current && !opts.some((o) => o.value === current)) {
    opts.push({ value: current, label: current, persona: null });
  }
  return opts;
}

// acceptedFrom keeps `current` when it is one of the brand's options, else returns the brand
// From: a bare address, another brand's persona and a removed persona all repoint.
export function acceptedFrom(current, brandRow) {
  return senderOptions(brandRow).some((o) => o.value === current) ? current : brandRow.from_email;
}

// pickerBrandRow is the row the picker may be built from, or null: the derivation is mapped and
// clean, the brands rows are loaded, a row exists for the brand, and its From is the one the
// lists' tags carry. A SQL-written from: tag that differs from the row resolves the old way --
// the exact match -- so no persona is offered for it.
export function pickerBrandRow(derivation, brandRow, brandsState) {
  if (!derivation || derivation.error || derivation.unmapped || !derivation.fromEmail) {
    return null;
  }
  if (brandsState !== BRANDS_LOADED || !brandRow || brandRow.from_email !== derivation.fromEmail) {
    return null;
  }
  return brandRow;
}

// fromToApply is the From the page should hold after a derivation pass, or null for "do nothing
// yet". `stored` is the campaign's stored From on the LOAD pass (the first settled pass after an
// existing campaign loads) and null/'' on every later pass, when the form's own value is judged.
//
//   - an unsettled derivation (an error, or no From yet -- the server config has not arrived)
//     -> null;
//   - an unmapped derivation -> its From (the default sender; there is nothing to pick);
//   - mapped while the brands rows are PENDING -> null: no repoint, no toast, and the caller
//     leaves its one-shot load flag alone, exactly as it does for an unloaded lists store.
//     Acting here would repoint a stored persona to the brand From before the row that makes it
//     valid has arrived, and a routine save would then ship the campaign as the brand;
//   - mapped and the fetch FAILED, or no usable row -> the brand From (the forcing that predates
//     personas);
//   - otherwise acceptedFrom of the stored value (load pass) or the form's value (later).
export function fromToApply(stored, form, derivation, brandRow, brandsState) {
  if (!derivation || derivation.error || !derivation.fromEmail) {
    return null;
  }
  if (derivation.unmapped) {
    return derivation.fromEmail;
  }
  if (brandsState === BRANDS_PENDING) {
    return null;
  }
  const row = pickerBrandRow(derivation, brandRow, brandsState);
  if (!row) {
    return derivation.fromEmail;
  }
  return acceptedFrom(stored || form, row);
}

// isSyncRepoint is the repoint-notice predicate: true only while the form holds the brand From
// the derivation forced and the stored From differs -- an unsaved automatic repoint. A deliberate
// persona pick (the form holds a persona From) is never announced as a repoint.
export function isSyncRepoint(stored, form, derivation) {
  return !!stored && !!derivation && !derivation.error
    && form === derivation.fromEmail && stored !== form;
}

// campaignRefs renders the campaigns blocking a persona's removal for the hover text, short:
// ids only, grouped by status in first-seen order -- `draft campaigns: c194, c195` (and
// `; running campaigns: c67` when statuses are mixed). Names are left out on purpose: several
// long campaign names make the tooltip unreadable.
export function campaignRefs(campaigns) {
  const groups = new Map();
  (Array.isArray(campaigns) ? campaigns : []).forEach((c) => {
    if (!groups.has(c.status)) {
      groups.set(c.status, []);
    }
    groups.get(c.status).push(`c${c.id}`);
  });
  return [...groups].map(([status, ids]) => `${status} campaigns: ${ids.join(', ')}`).join('; ');
}

// blockingCampaigns is the GET /api/brands/:slug/personas entry's campaigns for one persona
// ([] when the persona is unused or the list has not loaded).
export function blockingCampaigns(uses, persona) {
  const hit = (Array.isArray(uses) ? uses : []).find((u) => u && u.name === persona);
  return hit && Array.isArray(hit.campaigns) ? hit.campaigns : [];
}
