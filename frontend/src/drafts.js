// Fork (session expiry). localStorage stash for campaign edits that would otherwise be lost
// when the admin session expires mid-edit: the api interceptor redirects through login, and
// Campaign.vue offers the stash back on return. Campaign editor only, by design.
//
// The stash is a WHITELIST, never the form object: form carries a Date, object-shaped
// lists/media, a derived fromEmail that a watcher force-overwrites, and two template ids.

const PREFIX = 'listmonk.draft.';
export const DRAFT_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;

const key = (campaignId) => `${PREFIX}${campaignId}`;

export function readDraft(campaignId) {
  try {
    const raw = window.localStorage.getItem(key(campaignId));
    return raw ? JSON.parse(raw) : null;
  } catch (e) {
    return null;
  }
}

export function writeDraft(campaignId, draft) {
  try {
    window.localStorage.setItem(key(campaignId), JSON.stringify(draft));
  } catch (e) {
    // Quota or private mode: nothing to do, the redirect still happens.
  }
}

export function deleteDraft(campaignId) {
  try {
    window.localStorage.removeItem(key(campaignId));
  } catch (e) {
    // ignore
  }
}

export function clearAllDrafts() {
  try {
    const ks = [];
    for (let i = 0; i < window.localStorage.length; i += 1) {
      const k = window.localStorage.key(i);
      if (k && k.startsWith(PREFIX)) {
        ks.push(k);
      }
    }
    ks.forEach((k) => window.localStorage.removeItem(k));
  } catch (e) {
    // ignore
  }
}
