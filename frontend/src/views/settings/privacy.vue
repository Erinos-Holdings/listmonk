<template>
  <div class="items">
    <div class="columns">
      <div class="column is-6">
        <b-field :message="$t('settings.privacy.disableTrackingHelp')">
          <b-switch v-model="data['privacy.disable_tracking']" name="privacy.disable_tracking">
            {{ $t('settings.privacy.disableTracking') }}
          </b-switch>
        </b-field>
      </div>
      <div class="column is-6" :class="{ 'is-disabled': data['privacy.disable_tracking'] }">
        <b-field :message="$t('settings.privacy.individualSubTrackingHelp')">
          <b-switch v-model="data['privacy.individual_tracking']" :disabled="data['privacy.disable_tracking']"
            name="privacy.individual_tracking">
            {{ $t('settings.privacy.individualSubTracking') }}
          </b-switch>
        </b-field>
      </div>
    </div>

    <!-- Fork (click tracking, CLICK-TRACKING-SPEC §3.4) -->
    <b-field :label="$t('settings.privacy.linkFallbackURL')" label-position="on-border"
      :message="$t('settings.privacy.linkFallbackURLHelp')">
      <b-input v-model="data['app.link_fallback_url']" name="app.link_fallback_url"
        placeholder="https://curatedfor.you" data-cy="link-fallback-url" />
    </b-field>
    <b-field :message="$t('settings.privacy.utmEnableHelp')">
      <b-switch v-model="data['app.utm_enable']" name="app.utm_enable" data-cy="utm-enable">
        {{ $t('settings.privacy.utmEnable') }}
      </b-switch>
    </b-field>
    <div class="columns" :class="{ 'is-disabled': !data['app.utm_enable'] }">
      <div class="column is-6">
        <b-field :label="$t('settings.privacy.utmHosts')" label-position="on-border"
          :message="$t('settings.privacy.utmHostsHelp')">
          <b-taginput v-model="data['app.utm_hosts']" name="app.utm_hosts" data-cy="utm-hosts" />
        </b-field>
      </div>
      <div class="column is-6">
        <b-field :label="$t('settings.privacy.utmParams')" label-position="on-border"
          :message="$t('settings.privacy.utmParamsHelp')">
          <b-input type="textarea" v-model="data['app.utm_params']" name="app.utm_params" data-cy="utm-params" />
        </b-field>
      </div>
    </div>

    <hr />

    <b-field :message="$t('settings.privacy.listUnsubHeaderHelp')">
      <b-switch v-model="data['privacy.unsubscribe_header']" name="privacy.unsubscribe_header">
        {{ $t('settings.privacy.listUnsubHeader') }}
      </b-switch>
    </b-field>

    <b-field :message="$t('settings.privacy.allowBlocklistHelp')">
      <b-switch v-model="data['privacy.allow_blocklist']" name="privacy.allow_blocklist">
        {{ $t('settings.privacy.allowBlocklist') }}
      </b-switch>
    </b-field>

    <b-field :message="$t('settings.privacy.allowPrefsHelp')">
      <b-switch v-model="data['privacy.allow_preferences']" name="privacy.allow_blocklist">
        {{ $t('settings.privacy.allowPrefs') }}
      </b-switch>
    </b-field>

    <b-field :message="$t('settings.privacy.allowExportHelp')">
      <b-switch v-model="data['privacy.allow_export']" name="privacy.allow_export">
        {{ $t('settings.privacy.allowExport') }}
      </b-switch>
    </b-field>

    <b-field :message="$t('settings.privacy.allowWipeHelp')">
      <b-switch v-model="data['privacy.allow_wipe']" name="privacy.allow_wipe">
        {{ $t('settings.privacy.allowWipe') }}
      </b-switch>
    </b-field>

    <b-field :message="$t('settings.privacy.recordOptinIPHelp')">
      <b-switch v-model="data['privacy.record_optin_ip']" name="privacy.record_optin_ip">
        {{ $t('settings.privacy.recordOptinIP') }}
      </b-switch>
    </b-field>

    <hr />

    <b-tabs v-model="tab" type="is-boxed" :animated="false">
      <b-tab-item :label="`${$t('settings.privacy.domainBlocklist')} (${numBlocked})`">
        <b-field :message="$t('settings.privacy.domainBlocklistHelp')">
          <b-input type="textarea" v-model="data['privacy.domain_blocklist']" name="privacy.domain_blocklist" />
        </b-field>
      </b-tab-item>
      <b-tab-item :label="`${$t('settings.privacy.domainAllowlist')} (${numAllowed})`">
        <b-field :message="$t('settings.privacy.domainAllowlistHelp')">
          <b-input type="textarea" v-model="data['privacy.domain_allowlist']" name="privacy.domain_allowlist" />
        </b-field>
      </b-tab-item>
    </b-tabs>
  </div>
</template>

<script>
import Vue from 'vue';

export default Vue.extend({
  props: {
    form: {
      type: Object, default: () => { },
    },
  },

  data() {
    return {
      data: this.form,
      tab: 0,
    };
  },

  methods: {
    countItems(str) {
      return str.split('\n').filter((line) => line.trim()).length;
    },
  },

  mounted() {
    this.tab = this.$utils.getPref('settings.privacyDomainTab') || 0;
  },

  computed: {
    numBlocked() {
      return this.countItems(this.form['privacy.domain_blocklist']);
    },
    numAllowed() {
      return this.countItems(this.form['privacy.domain_allowlist']);
    },
  },

  watch: {
    tab(t) {
      this.$utils.setPref('settings.privacyDomainTab', t);
    },
  },
});
</script>
