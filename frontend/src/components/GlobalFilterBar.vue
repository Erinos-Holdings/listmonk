<template>
  <!-- Fork (polish 2026-11-14, the user). The blue bar across the top of a matrix or table while a
       global filter narrows it: the active filters, then the columns they hide (filterBar.mjs).
       Rendered only when something applies. Presentation only -- it changes nothing. -->
  <div v-if="state" class="global-filter-bar" role="status" data-cy="global-filter-bar">
    <b-icon icon="filter-outline" size="is-small" />
    <span class="gfb-label">{{ $t('filterBar.label') }}</span>
    <span v-for="(it, i) in state.items" :key="it.key" class="gfb-item">
      <span v-if="i > 0" class="gfb-sep">·</span>{{ $t(it.key, it.params) }}
    </span>
    <span v-if="state.hidden.length" class="gfb-hidden" data-cy="global-filter-bar-hidden">
      — {{ $t('filterBar.hidden', { cols: state.hidden.map((k) => $t(k)).join(', ') }) }}
    </span>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { filterBarState } from '../filterBar.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'GlobalFilterBar',

  props: {
    // Which global filters narrow this table (filterBar.mjs).
    brand: { type: Boolean, default: true },
    launch: { type: Boolean, default: false },
    lang: { type: Boolean, default: true },
    // The columns this table hides under a filter.
    brandColumn: { type: Boolean, default: false },
    langColumn: { type: Boolean, default: false },
  },

  computed: {
    ...mapGetters(['brandSelection', 'langEffective']),
    ...mapGetters({ launchValue: 'launch' }),

    state() {
      return filterBarState({
        selection: this.brandSelection,
        launch: this.launchValue,
        lang: this.langEffective,
        applies: { brand: this.brand, launch: this.launch, lang: this.lang },
        columns: { brand: this.brandColumn, lang: this.langColumn },
      });
    },
  },
});
</script>
