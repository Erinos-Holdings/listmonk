<template>
  <!-- Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D1/D13). The navbar Campaign mode toggle,
       Off | Groups, immediately right of the language toggle, for a user who can read campaigns or
       their analytics. In Groups the Campaigns page, the Analytics picker and the Campaign Matrix
       collapse campaigns into sets (the language variants of one send) and promo series. A
       presentation choice, never a permission input; the pref `campaigns.mode`, written only by the
       store's selectCampaignMode. The selected button wears the route's applicability
       (campaignGroups.mjs::MODE_APPLICABILITY: blue applies, grey not applicable), with a grey line
       beside it in Groups (campaignGroups.mjs::modeNote) and the same text as the title. -->
  <div v-if="shown" class="campaign-mode-toggle" :class="`mode-app-${applicability}`" data-cy="campaign-mode-toggle"
    role="group" :aria-label="$t('naming.campaignMode')" :title="noteText || $t('naming.campaignMode')">
    <b-field class="segmented is-global">
      <b-radio-button v-for="v in values" :key="v" v-model="value" :native-value="v" size="is-small" :type="buttonType"
        :data-cy="`campaign-mode-${v}`">
        {{ $t(`naming.campaignMode_${v}`) }}
      </b-radio-button>
    </b-field>
    <span v-if="noteText" class="mode-note is-size-7 has-text-grey" data-cy="campaign-mode-note">{{ noteText }}</span>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { MODES, modeApplicability, modeNote } from '../campaignGroups.mjs'; // eslint-disable-line import/extensions

// The Buefy button type per applicability state (the language toggle's classes, style.scss).
const BUTTON_TYPES = Object.freeze({ all: 'is-primary', none: 'is-lang-none' });

export default Vue.extend({
  name: 'CampaignModeToggle',

  data() {
    return { values: MODES };
  },

  computed: {
    ...mapGetters(['campaignMode', 'viewPane', 'langEffective']),

    // D1: shown to a user holding any of campaigns:get, campaigns:get_all, campaigns:get_analytics.
    shown() {
      return this.$can('campaigns:get', 'campaigns:get_all', 'campaigns:get_analytics');
    },

    applicability() {
      return modeApplicability((this.$route || {}).name, this.viewPane);
    },

    buttonType() {
      return BUTTON_TYPES[this.applicability] || BUTTON_TYPES.all;
    },

    // D13: the grey line, in Groups only.
    noteText() {
      const key = modeNote(this.campaignMode, (this.$route || {}).name, {
        pane: this.viewPane, langEffective: this.langEffective,
      });
      return key ? this.$t(key) : '';
    },

    // D1: the control commits selectCampaignMode (the pref's one writer); the views watch the getter
    // and re-fetch.
    value: {
      get() {
        return this.campaignMode;
      },
      set(v) {
        this.$store.commit('selectCampaignMode', v);
      },
    },
  },
});
</script>

<style lang="scss">
.campaign-mode-toggle {
  display: inline-flex;
  align-items: center;
  margin-left: 0.5rem;
  .field,
  > .field.segmented:not(:last-child) {
    margin-bottom: 0;
  }
  .mode-note {
    margin-left: 0.5rem;
    max-width: 22rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
