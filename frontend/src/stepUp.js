// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6/D11; integrations STEPUP-ADMIN-SPEC D4).
// stepUp(vm) resolves once the session carries a fresh step-up stamp: it opens the shared
// StepUpDialog (passkey / TOTP code / password when the user has no factor) and rejects with
// Error('cancelled') if the dialog is cancelled. Whether the dialog can be skipped is the
// server's call: GET /api/profile/twofa reports the seconds left on the session's stamp
// (stepup_ttl), and the dialog is skipped only while at least a minute is left. The server still
// checks every request -- a 403 there just means the user steps up again.
import StepUpDialog from './components/StepUpDialog.vue';

const MIN_TTL = 60;

export default function stepUp(vm) {
  return vm.$api.getProfileTwofa().then((tf) => {
    if ((tf.stepupTtl || 0) >= MIN_TTL) {
      return undefined;
    }
    return new Promise((resolve, reject) => {
      vm.$buefy.modal.open({
        parent: vm,
        component: StepUpDialog,
        props: { totp: tf.totp, passkeys: (tf.passkeys || []).length },
        hasModalCard: true,
        trapFocus: true,
        canCancel: ['escape', 'x'],
        onCancel: () => reject(new Error('cancelled')),
        events: {
          done: () => resolve(),
          cancel: () => reject(new Error('cancelled')),
        },
      });
    });
  });
}

// isCancelled reports whether e is the rejection of a cancelled step-up dialog.
export function isCancelled(e) {
  return !!e && e.message === 'cancelled';
}

// stepUpError is the .catch handler for a call that steps up first: a cancelled dialog is silent
// (nothing was sent), an API error has already been toasted by the API layer, and any other error
// is toasted here -- so no rejection goes unhandled and only the cancel is swallowed.
export function stepUpError(vm) {
  return (e) => {
    if (!e || isCancelled(e) || e.config) {
      return;
    }
    vm.$utils.toast(e.message || String(e), 'is-danger');
  };
}
