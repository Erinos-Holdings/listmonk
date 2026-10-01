<template>
  <!-- Fork (two-factor, integrations PASSKEY-2FA-SPEC D6/D11): the one step-up dialog, shared by the
    profile page (factor, password and email changes) and every user, role and settings write. -->
  <div class="modal-card content" style="width: auto">
    <header class="modal-card-head">
      <h4>{{ $t('users.stepUpTitle') }}</h4>
    </header>
    <section class="modal-card-body">
      <p>{{ $t('users.stepUpHelp') }}</p>

      <b-button v-if="passkeys > 0" type="is-primary" icon-left="account-check-outline" :loading="busy" @click="withPasskey"
        data-cy="btn-stepup-passkey">
        {{ $t('users.passkeyUse') }}
      </b-button>

      <form v-if="totp" class="mt-5" @submit.prevent="withTotp">
        <b-field :label="$t('users.totpCode')" label-position="on-border">
          <b-input v-model="code" maxlength="6" pattern="[0-9]{6}" placeholder="000000" required />
        </b-field>
        <b-button native-type="submit" type="is-primary" :loading="busy">
          {{ $t('globals.buttons.continue') }}
        </b-button>
      </form>

      <form v-if="!totp && passkeys === 0" @submit.prevent="withPassword">
        <p class="has-text-grey">{{ $t('users.stepUpPasswordHelp') }}</p>
        <b-field :label="$t('users.password')" label-position="on-border">
          <b-input v-model="password" type="password" minlength="8" required />
        </b-field>
        <b-button native-type="submit" type="is-primary" :loading="busy">
          {{ $t('globals.buttons.continue') }}
        </b-button>
      </form>
    </section>
    <footer class="modal-card-foot has-text-right">
      <b-button @click="cancel">
        {{ $t('globals.buttons.cancel') }}
      </b-button>
    </footer>
  </div>
</template>

<script>
import Vue from 'vue';
import loadWebAuthn from '../webauthnLoader';

export default Vue.extend({
  name: 'StepUpDialog',

  props: {
    totp: { type: Boolean, default: false },
    passkeys: { type: Number, default: 0 },
  },

  data() {
    return { code: '', password: '', busy: false };
  },

  methods: {
    done() {
      this.$emit('done');
      this.$parent.close();
    },

    cancel() {
      this.$emit('cancel');
      this.$parent.close();
    },

    post(fields) {
      const d = new FormData();
      Object.keys(fields).forEach((k) => d.append(k, fields[k]));
      this.busy = true;
      return this.$api.stepUp(d).then(this.done).finally(() => {
        this.busy = false;
      });
    },

    withTotp() {
      this.post({ totp_code: this.code }).catch(() => {
        this.code = '';
      });
    },

    withPassword() {
      this.post({ password: this.password }).catch(() => {
        this.password = '';
      });
    },

    withPasskey() {
      this.busy = true;
      loadWebAuthn()
        .then((wa) => this.$api.stepUpPasskeyBegin().then((opts) => wa.get(opts)))
        .then((cred) => this.$api.stepUpPasskeyFinish(cred))
        .then(this.done)
        .catch((e) => {
          if (e && !e.response) {
            this.$utils.toast(e.message || String(e), 'is-danger');
          }
        })
        .finally(() => {
          this.busy = false;
        });
    },
  },
});
</script>
