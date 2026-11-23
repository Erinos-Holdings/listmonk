<template>
  <!-- Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D1/D13). The navbar "Campaign mode" switch
       (the Lists page's "View system lists" style), right of the launch filter, for a user who can
       read campaigns or their analytics. On, the Campaigns page, the Analytics picker and the
       Campaign Matrix collapse campaigns into sets and promo series. A presentation choice, never a
       permission input; the pref `campaigns.mode` ('off' | 'groups'), written only by the store's
       selectCampaignMode. On a route where it does not apply
       (campaignGroups.mjs::MODE_APPLICABILITY none) the switch is muted; on, a grey line beside it
       (campaignGroups.mjs::modeNote), the same text as the title. -->
  <div v-if="shown" class="campaign-mode-toggle" :class="`mode-app-${applicability}`" data-cy="campaign-mode-toggle"
    :title="noteText || $t('naming.campaignMode')">
    <b-switch v-model="value" size="is-small" data-cy="campaign-mode-switch">
      {{ $t('naming.campaignMode') }}
    </b-switch>
    <span v-if="noteText" class="mode-note is-size-7 has-text-grey" data-cy="campaign-mode-note">{{ noteText }}</span>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { modeApplicability, modeNote } from '../campaignGroups.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'CampaignModeToggle',

  computed: {
    ...mapGetters(['campaignMode', 'viewPane', 'langEffective']),

    // D1: shown to a user holding any of campaigns:get, campaigns:get_all, campaigns:get_analytics.
    shown() {
      return this.$can('campaigns:get', 'campaigns:get_all', 'campaigns:get_analytics');
    },

    applicability() {
      return modeApplicability((this.$route || {}).name, this.viewPane);
    },

    // D13: the grey line, in Groups only.
    noteText() {
      const key = modeNote(this.campaignMode, (this.$route || {}).name, {
        pane: this.viewPane, langEffective: this.langEffective,
      });
      return key ? this.$t(key) : '';
    },

    // D1: the switch commits selectCampaignMode (the pref's one writer) -- on 'groups', off 'off';
    // the views watch the getter and re-fetch.
    value: {
      get() {
        return this.campaignMode === 'groups';
      },
      set(on) {
        this.$store.commit('selectCampaignMode', on ? 'groups' : 'off');
      },
    },
  },
});
</script>

<style lang="scss">
.campaign-mode-toggle {
  display: inline-flex;
  align-items: center;
  margin-left: 0.75rem;
  .switch {
    margin-right: 0;
    white-space: nowrap;
  }
  // D13 -- not applicable on this route: muted, still usable.
  &.mode-app-none .switch {
    opacity: 0.55;
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
