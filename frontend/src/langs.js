// Fork (multi-language campaigns; LIST-GRID-SPEC D12). The languages a campaign or a template
// can carry, labelled in their own language. The closed set is models.CampaignLangs on the
// server, which validates every write -- this list only renders the selects.
export const CAMPAIGN_LANGS = [
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Español' },
  { code: 'fr', label: 'Français' },
  { code: 'de', label: 'Deutsch' },
  { code: 'it', label: 'Italiano' },
];

export const campaignLangLabel = (code) => (CAMPAIGN_LANGS.find((l) => l.code === code) || {}).label
  || (code || '').toUpperCase();
