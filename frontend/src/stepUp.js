// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6/D11). stepUp(vm) resolves once the session
// carries a fresh step-up stamp: it opens the shared StepUpDialog (passkey / TOTP code / password
// when the user has no factor) and rejects if the dialog is cancelled. The server keeps a stamp
// for 5 minutes; within 4 of the last success the dialog is skipped (the server is still the judge
// -- a 403 there just means the user steps up again).
import StepUpDialog from './components/StepUpDialog.vue';

const REUSE_MS = 4 * 60 * 1000;
let stampedAt = 0;

export default function stepUp(vm) {
  if (Date.now() - stampedAt < REUSE_MS) {
    return Promise.resolve();
  }
  return vm.$api.getProfileTwofa().then((tf) => new Promise((resolve, reject) => {
    vm.$buefy.modal.open({
      parent: vm,
      component: StepUpDialog,
      props: { totp: tf.totp, passkeys: (tf.passkeys || []).length },
      hasModalCard: true,
      trapFocus: true,
      canCancel: ['escape', 'x'],
      onCancel: () => reject(new Error('cancelled')),
      events: {
        done: () => {
          stampedAt = Date.now();
          resolve();
        },
        cancel: () => reject(new Error('cancelled')),
      },
    });
  }));
}
