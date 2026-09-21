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

// Fork (the "+" convention). English as a SEND language is wider than English as a STORED
// language: an English campaign also reaches subscribers with no language set (COALESCE-EN), and
// the Lists grid's EN line counts them. Every label that means the send audience carries a "+"
// -- English+ / EN+ -- by DEFINITION, whether or not a given list has any no-language rows, so
// the same word never means two audiences. Labels that mean the stored/content language stay
// bare: a template's language, and the Subscribers picker's "English" (lang=en_only). Each "+"
// label carries the langs.sendPlusHelp title.
export const SEND_PLUS_LANG = 'en';
export const isSendPlus = (code) => (code || '').toLowerCase() === SEND_PLUS_LANG;

export const campaignLangLabel = (code) => (CAMPAIGN_LANGS.find((l) => l.code === code) || {}).label
  || (code || '').toUpperCase();

// The send-audience forms: "English+" for a select option, "EN+" for a chip or grid label.
export const sendLangLabel = (code) => `${campaignLangLabel(code)}${isSendPlus(code) ? '+' : ''}`;
export const sendLangCode = (code) => `${(code || '').toUpperCase()}${isSendPlus(code) ? '+' : ''}`;
