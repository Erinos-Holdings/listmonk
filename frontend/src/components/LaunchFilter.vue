<template>
  <!-- Fork (launch filter, integrations LAUNCH-FILTER-SPEC D4). The navbar's All | Launched |
       Unlaunched filter, right of the brand selector. A presentation filter on the selector's
       options and the Brands / SES health tables only -- never a list scope (D7, S1). Counts are
       over the FULL roster. Hidden when the selector is hidden or locked; disabled while the
       selection is pending. Fork (brand roles, integrations BRAND-ROLES-SPEC D8) -- no brands:get
       condition: without it the launch state is GET /api/brands/launch's one bit (launchDocs). -->
  <div v-if="shown" class="launch-filter" data-cy="launch-filter" role="group" :aria-label="$t('brand.launch.label')">
    <!-- .segmented: the shared one-control treatment (style.scss), also on the Campaigns page's
         Broadcasts | Automations toggle. is-global: the blue ring of a selector whose state follows
         the user across pages (integrations JOURNEY-UX-SPEC A1.16/A1.17). -->
    <b-field class="segmented is-global">
      <b-radio-button v-model="value" native-value="all" size="is-small" type="is-primary"
        :disabled="brandScope.pending" data-cy="launch-all">
        {{ $t('brand.launch.all') }}
      </b-radio-button>
      <b-radio-button v-model="value" native-value="launched" size="is-small" type="is-primary"
        :disabled="brandScope.pending" data-cy="launch-launched">
        {{ $t('brand.launch.launched', { n: counts.launched }) }}
      </b-radio-button>
      <b-radio-button v-model="value" native-value="unlaunched" size="is-small" type="is-primary"
        :disabled="brandScope.pending" data-cy="launch-unlaunched">
        {{ $t('brand.launch.unlaunched', { n: counts.unlaunched }) }}
      </b-radio-button>
    </b-field>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { launchCounts, launchFilterShown } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'LaunchFilter',

  computed: {
    ...mapGetters(['brandScope', 'brandRoster', 'launchDocs', 'launch']),

    // D4: nothing to narrow when the selector is hidden or locked (BRAND-ROLES-SPEC D8: shown for
    // every user otherwise).
    shown() {
      return launchFilterShown(this.brandScope);
    },

    counts() {
      return launchCounts(this.brandRoster, this.launchDocs);
    },

    // D1/I7: the control is the selectLaunch mutation's only caller.
    value: {
      get() {
        return this.launch;
      },
      set(v) {
        this.$store.commit('selectLaunch', v);
      },
    },
  },
});
</script>

<style lang="scss">
.launch-filter {
  .field {
    margin-bottom: 0;
  }
}
</style>
