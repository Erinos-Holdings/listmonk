<template>
  <section class="user-profile section-mini">
    <b-loading v-if="loading.users" :active="loading.users" :is-full-page="false" />

    <h1 class="title">
      @{{ data.username }}
    </h1>
    <b-tag v-if="data.userRole">{{ data.userRole.name }}</b-tag>

    <br /><br /><br />
    <form @submit.prevent="onSubmit">
      <b-field v-if="data.type !== 'api'" :label="$t('subscribers.email')" label-position="on-border">
        <b-input :maxlength="200" v-model="form.email" name="email" :placeholder="$t('subscribers.email')"
          :disabled="!data.passwordLogin" required autofocus />
      </b-field>

      <b-field :label="$t('globals.fields.name')" label-position="on-border">
        <b-input :maxlength="200" v-model="form.name" name="name" :placeholder="$t('globals.fields.name')" />
      </b-field>

      <!-- Fork (password policy, integrations PASSWORD-POLICY-SPEC D5) -- the server rule's bounds and
        text as a convenience; the server (validatePassword) is the enforcement. The step-up
        dialog's password is a verify path and stays minlength="8" (PASSKEY-2FA-SPEC D9). -->
      <div v-if="data.passwordLogin" class="columns">
        <div class="column is-6">
          <b-field :label="$t('users.password')" label-position="on-border" :message="$t('users.passwordPolicy')">
            <b-input minlength="16" :maxlength="72" v-model="form.password" type="password" name="password"
              :placeholder="$t('users.password')" />
          </b-field>
        </div>
        <div class="column is-6">
          <b-field :label="$t('users.passwordRepeat')" label-position="on-border">
            <b-input minlength="16" :maxlength="72" v-model="form.password2" type="password" name="password2" />
          </b-field>
        </div>
      </div>

      <b-field expanded>
        <b-button type="is-primary" icon-left="content-save-outline" native-type="submit" data-cy="btn-save">
          {{ $t('globals.buttons.save') }}
        </b-button>
      </b-field>
    </form>

    <br /><br />

    <!-- 2FA. Fork (two-factor, integrations PASSKEY-2FA-SPEC D11): a list of factors -- the TOTP row
      and one row per passkey. Every change asks for a step-up first (stepUp.js). -->
    <section v-if="data.passwordLogin" class="twofa-section">
      <div class="box">
        <h3 class="title is-size-5">
          {{ $t('users.twoFA') }}
          <b-tag v-if="twofa.required" type="is-warning">{{ $t('users.twoFARequiredTag') }}</b-tag>
        </h3>
        <p v-if="twofa.required" class="has-text-grey">{{ $t('users.twoFARequired') }}</p>
        <br />

        <!-- TOTP -->
        <div class="columns is-vcentered">
          <div class="column">
            <strong>TOTP</strong>
            <b-icon v-if="twofa.totp" icon="check-circle-outline" type="is-success" size="is-small" />
            <p class="has-text-grey is-size-7">
              {{ twofa.totp ? $t('users.twoFAEnabledDesc', { type: 'TOTP' }) : $t('users.twoFANotEnabled') }}
            </p>
          </div>
          <div class="column is-narrow">
            <b-button v-if="twofa.totp" type="is-danger" size="is-small" @click="onDisableTOTP" data-cy="btn-totp-off">
              {{ $t('globals.buttons.disable') }}
            </b-button>
            <b-button v-else-if="!isTotpVisible" size="is-small" @click="onEnableTOTP" data-cy="btn-totp-on">
              {{ $t('globals.buttons.enable') }}
            </b-button>
          </div>
        </div>

        <!-- TOTP setup -->
        <div v-if="isTotpVisible && totpQR" class="totp-setup qr-section">
          <p class="has-text-grey">{{ $t('users.totpScanQR') }}</p><br />

          <img :src="'data:image/png;base64,' + totpQR" alt="QR Code" />

          <br /><br />
          <p>
            <strong>{{ $t('users.totpSecret') }}</strong><br />
            <code><copy-text :text="`${totpSecret}`" /></code>
          </p>

          <br /><br />
          <form @submit.prevent="confirmTOTP">
            <b-field :label="$t('users.totpCode')" label-position="on-border">
              <b-input ref="totpCodeInput" v-model="totpCode" maxlength="6" pattern="[0-9]{6}" placeholder="000000"
                required />
            </b-field>
            <div class="buttons">
              <b-button type="is-primary" native-type="submit">
                {{ $t('globals.buttons.enable') }}
              </b-button>
              <b-button type="button" @click="onCancelTOTPSetup">
                {{ $t('globals.buttons.cancel') }}
              </b-button>
            </div>
          </form>
        </div>

        <hr />

        <!-- Passkeys -->
        <p><strong>{{ $t('users.passkeys') }}</strong></p>
        <div v-for="pk in twofa.passkeys" :key="pk.id" class="columns is-vcentered passkey">
          <div class="column">
            {{ pk.name }}
            <p class="has-text-grey is-size-7">
              {{ $t('globals.fields.createdAt') }} {{ $utils.niceDate(pk.createdAt) }} &middot;
              {{ $t('users.passkeyLastUsed') }}
              {{ pk.lastUsedAt ? $utils.niceDate(pk.lastUsedAt, true) : $t('users.passkeyNeverUsed') }}
            </p>
          </div>
          <div class="column is-narrow">
            <b-button type="is-danger" size="is-small" @click="onDeletePasskey(pk)" data-cy="btn-passkey-remove">
              {{ $t('globals.buttons.remove') }}
            </b-button>
          </div>
        </div>

        <form class="columns mt-3" @submit.prevent="onAddPasskey">
          <div class="column">
            <b-field :label="$t('users.passkeyNameLabel')" label-position="on-border">
              <b-input v-model="passkeyName" :maxlength="64" :placeholder="$t('users.passkeyNamePlaceholder')"
                required />
            </b-field>
          </div>
          <div class="column is-narrow">
            <b-button native-type="submit" type="is-primary" icon-left="plus" :loading="addingPasskey"
              data-cy="btn-passkey-add">
              {{ $t('users.passkeyAdd') }}
            </b-button>
          </div>
        </form>
      </div>
    </section>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import CopyText from '../components/CopyText.vue';
import stepUp from '../stepUp';
import loadWebAuthn from '../webauthnLoader';

export default Vue.extend({
  name: 'UserProfile',

  components: {
    CopyText,
  },

  data() {
    return {
      form: {},
      data: {},
      twofa: {
        totp: false, passkeys: [], required: false, enforced: false,
      },
      isTotpVisible: false,
      totpQR: null,
      totpSecret: null,
      totpCode: '',
      passkeyName: '',
      addingPasskey: false,
    };
  },

  methods: {
    onSubmit() {
      const params = {
        name: this.form.name,
        email: this.form.email,
      };

      if (this.data.passwordLogin && this.form.password) {
        if (this.form.password !== this.form.password2) {
          this.$utils.toast(this.$t('users.passwordMismatch'), 'is-danger');
          return;
        }

        params.password = this.form.password;
        params.password2 = this.form.password2;
      }

      // Fork (two-factor, PASSKEY-2FA-SPEC D6) -- replacing the password or the email needs a
      // step-up; a name-only save does not.
      const credentials = this.data.passwordLogin && (params.password || params.email !== this.data.email);
      (credentials ? stepUp(this) : Promise.resolve())
        .then(() => this.$api.updateUserProfile(params))
        .then(() => {
          this.form.password = '';
          this.form.password2 = '';
          this.$utils.toast(this.$t('globals.messages.updated', { name: this.data.username }));
          return this.reload();
        })
        .catch(() => {});
    },

    // reload refreshes the profile and its factor state.
    reload() {
      return Promise.all([this.$api.getUserProfile(), this.$api.getProfileTwofa()]).then(([data, twofa]) => {
        this.data = { ...data };
        this.twofa = { ...this.twofa, ...twofa };
      });
    },

    onEnableTOTP() {
      stepUp(this)
        .then(() => this.$api.getTOTPQR(this.data.id))
        .then((data) => {
          this.totpQR = data.qr;
          this.totpSecret = data.secret;
          this.isTotpVisible = true;

          this.$nextTick(() => {
            if (this.$refs.totpCodeInput) {
              this.$refs.totpCodeInput.focus();
            }
          });
        })
        .catch(() => {});
    },

    onCancelTOTPSetup() {
      this.isTotpVisible = false;
      this.totpQR = null;
      this.totpSecret = null;
      this.totpCode = '';
    },

    confirmTOTP() {
      if (!this.totpCode || this.totpCode.length !== 6) {
        this.$utils.toast(this.$t('globals.messages.invalidValue'), 'is-danger');
        return;
      }

      const d = new FormData();
      d.append('secret', this.totpSecret);
      d.append('code', this.totpCode);

      stepUp(this)
        .then(() => this.$api.enableTOTP(this.data.id, d))
        .then(() => {
          this.$utils.toast(this.$t('users.twoFAEnabled'));
          this.onCancelTOTPSetup();
          return this.reload();
        })
        .catch(() => {});
    },

    // TOTP is turned off with a step-up, not the password (PASSKEY-2FA-SPEC U7).
    onDisableTOTP() {
      this.$utils.confirm(this.$t('globals.messages.confirm'), () => {
        stepUp(this)
          .then(() => this.$api.disableTOTP(this.data.id))
          .then(() => {
            this.$utils.toast(this.$t('globals.messages.done'));
            return this.reload();
          })
          .catch(() => {});
      });
    },

    onAddPasskey() {
      const name = this.passkeyName.trim();
      if (!name) {
        return;
      }

      this.addingPasskey = true;
      stepUp(this)
        .then(() => loadWebAuthn())
        .then((wa) => {
          if (!wa.supported()) {
            throw new Error(this.$t('users.passkeyNotSupported'));
          }
          return this.$api.addPasskeyBegin().then((opts) => wa.create(opts));
        })
        .then((cred) => this.$api.addPasskeyFinish(name, cred))
        .then(() => {
          this.passkeyName = '';
          this.$utils.toast(this.$t('globals.messages.created', { name }));
          return this.reload();
        })
        .catch((e) => {
          // API errors are toasted by the API layer; a browser/ceremony error is not.
          if (e && !e.response && e.message !== 'cancelled') {
            this.$utils.toast(e.message || String(e), 'is-danger');
          }
        })
        .finally(() => {
          this.addingPasskey = false;
        });
    },

    onDeletePasskey(pk) {
      this.$utils.confirm(this.$t('globals.messages.confirm'), () => {
        stepUp(this)
          .then(() => this.$api.deletePasskey(pk.id))
          .then(() => {
            this.$utils.toast(this.$t('globals.messages.deleted', { name: pk.name }));
            return this.reload();
          })
          .catch(() => {});
      });
    },
  },

  mounted() {
    this.reload().then(() => {
      this.form = { name: this.data.name, email: this.data.email };
    });
  },

  computed: {
    ...mapState(['loading']),
  },

});
</script>
