<template>
  <!-- Fork (launch filter, integrations LAUNCH-FILTER-SPEC D4). The navbar's All | Launched |
       Unlaunched filter, right of the brand selector. A presentation filter on the selector's
       options and the Brands / SES health tables only -- never a list scope (D7, S1). Counts are
       over the FULL roster. Hidden when the selector is hidden or locked, or without brands:get
       (no documents are fetched); disabled while the selection is pending. -->
  <div v-if="shown" class="launch-filter" data-cy="launch-filter" role="group" :aria-label="$t('brand.launch.label')">
    <!-- .segmented: the shared one-control treatment (style.scss), also on the Campaigns page's
         Broadcasts | Automations toggle. -->
    <b-field class="segmented">
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
import { launchCounts } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'LaunchFilter',

  computed: {
    ...mapGetters(['brandScope', 'brandRoster', 'brandHealth', 'launch']),

    // D4: nothing to narrow when the selector is hidden or locked; without brands:get the
    // documents are never fetched and every brand would read as launched.
    shown() {
      return this.$can('brands:get') && !this.brandScope.hidden && !this.brandScope.locked;
    },

    counts() {
      return launchCounts(this.brandRoster, this.brandHealth);
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
