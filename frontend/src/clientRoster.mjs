// Fork (client stats, integrations CLIENT-STATS-SPEC D8). Token -> the Mailgun Inspect /
// render-catalog client ids that render-verify that client, shown as a per-row detail of the
// "Email clients" tables so an accepted render gap reads directly against the row's audience share.
//
// ADVISORY display data, maintained by hand next to its test (clientRoster.test.mjs pins only
// that every key is a vocabulary token, I7). It is NOT a sync pin against the live Mailgun roster:
// the roster is remote and drifts; a stale id here degrades a tooltip, not a decision. Source of
// the ids: the integrations repo's lib/mailgun-inspect.ts (FOUNDATIONAL_LOOP and the classified
// roster ids). A token with no rendering client in the roster (thunderbird, the non-Windows
// browsers, other) has no entry.

const webmail = (provider, modes = ['lm', 'dm']) => modes.flatMap((m) => ['chr', 'edge', 'ff']
  .map((b) => `${provider}-${m}_${b}current_win10`));

export const CLIENT_ROSTER = Object.freeze({
  // Gmail's image proxy serves Gmail web and both Gmail apps.
  'gmail-proxy': Object.freeze([
    ...webmail('gmailcom'),
    'iphone16gmail_26', 'iphone16gmail_26_dm',
    'android12_gmailapp_pixel6_lm', 'android12_gmailapp_pixel6_dm',
    'android13_gmailapp_pixel7_lm', 'android13_gmailapp_pixel7_dm',
    'android14_gmailapp_pixel8_lm', 'android14_gmailapp_pixel8_dm',
    'android15_gmailapp_pixel9_lm', 'android15_gmailapp_pixel9_dm',
    'android16_gmailapp_pixel10_lm', 'android16_gmailapp_pixel10_dm',
  ]),
  // Yahoo's mail proxy also fronts AOL Mail.
  yahoo: Object.freeze([...webmail('yahoocom'), ...webmail('aolcom')]),
  'outlook-windows': Object.freeze([
    'outlook2024_win_lm_dt', 'outlook2024_win_dm_dt',
    'outlook2021_win11_lm_dt', 'outlook2021_win11_dm_dt',
    'o365_w10_lm_dt', 'o365_w10_dm_dt',
    'm365_w11_lm_dt', 'm365_w11_dm_dt',
    'outlook16_win10', 'outlook16_win10_125', 'outlook19', 'outlook19_125',
  ]),
  'outlook-mac': Object.freeze(['m365_mac13_lm_dt', 'm365_mac13_dm_dt']),
  'outlook-mobile': Object.freeze(['iphone13ol_15', 'iphone13ol_15_dm']),
  // Apple Mail on macOS and iOS Mail (the iPhone ids that are neither Gmail nor Outlook).
  'apple-mail': Object.freeze([
    'applemail16', 'applemail16_dm',
    'iphone14_16', 'iphone14_16_dm', 'iphone14pro_16', 'iphone14pro_16_dm',
    'iphone14promax_16', 'iphone14promax_16_dm', 'iphone15_17', 'iphone15_17_dm',
    'iphone15plus_17', 'iphone15plus_17_dm', 'iphone15pro_17', 'iphone15pro_17_dm',
    'iphone15promax_17', 'iphone15promax_17_dm', 'iphone16_18', 'iphone16_18_dm',
    'iphone16pro_18', 'iphone16pro_18_dm', 'iphone16promax_18', 'iphone16promax_18_dm',
    'iphone17_26', 'iphone17_26_dm', 'iphone17pro_26', 'iphone17pro_26_dm',
    'iphone17promax_26', 'iphone17promax_26_dm', 'iphone18pro_27', 'iphone18pro_27_dm',
  ]),
  // Webmail rendered in a Windows browser (the roster's webmail ids are all Windows 10).
  'browser-windows': Object.freeze([
    ...webmail('outlookcom'), ...webmail('m365com'), ...webmail('freefr'),
    ...webmail('gmxnet', ['lm']), ...webmail('webde', ['lm']), ...webmail('tonlinede', ['lm']),
    ...webmail('liberoit', ['lm']),
  ]),
});

// rosterFor returns the Inspect ids that render-verify a token ([] when none).
export function rosterFor(token) {
  return Object.prototype.hasOwnProperty.call(CLIENT_ROSTER, token) ? [...CLIENT_ROSTER[token]] : [];
}
