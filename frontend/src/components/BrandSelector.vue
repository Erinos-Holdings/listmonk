<template>
  <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D4). The navbar's brand context: All
       brands, the roster's brands by display name, No brand. A presentation filter only (S1).
       Disabled placeholder until the lists load (D3); disabled with the brand shown when locked
       (S5); hidden when the roster is empty (D2). -->
  <!-- The hidden guard is this component's own contract; App.vue's brand-context wrapper repeats
       it so the wrapper (and the launch filter) leave the navbar with it (LAUNCH-FILTER-SPEC D4). -->
  <div v-if="!brandScope.hidden" class="brand-selector" data-cy="brand-selector">
    <b-select :value="value" :disabled="brandScope.pending || brandScope.locked" size="is-small"
      :aria-label="$t('brand.selector.label')" @input="onInput">
      <option v-if="brandScope.pending" value="">…</option>
      <template v-else>
        <option v-if="!brandScope.locked" value="all">{{ $t('brand.selector.all') }}</option>
        <!-- Fork (launch filter, integrations LAUNCH-FILTER-SPEC D5) -- the roster the launch filter
             leaves offered; locked/hidden stay the full roster's (brandScope). -->
        <!-- 2026-11-02 (user): the parent brand first, then a grey divider, then the rest by display
             name (brandScope.mjs::selectorBrands); No brand stays last. -->
        <option v-if="options.parent" :value="`brand:${options.parent.slug}`">{{ options.parent.label }}</option>
        <option v-if="divider" value="-" disabled class="brand-divider" aria-hidden="true">──────────</option>
        <option v-for="b in options.rest" :key="b.slug" :value="`brand:${b.slug}`">{{ b.label }}</option>
        <option v-if="showNone" value="none">{{ $t('brand.selector.none') }}</option>
      </template>
    </b-select>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { selectorBrands } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'BrandSelector',

  computed: {
    ...mapGetters(['brandScope', 'offeredRoster']),

    options() {
      return selectorBrands(this.offeredRoster.brands);
    },

    showNone() {
      return this.offeredRoster.none && !this.brandScope.locked;
    },

    // The divider only between the parent and something below it.
    divider() {
      return !!this.options.parent && (this.options.rest.length > 0 || this.showNone);
    },

    value() {
      if (this.brandScope.pending) {
        return '';
      }
      const s = this.brandScope.selection;
      return s.kind === 'brand' ? `brand:${s.slug}` : s.kind;
    },
  },

  methods: {
    onInput(v) {
      let selection = { kind: 'all' };
      if (v === 'none') {
        selection = { kind: 'none' };
      } else if (typeof v === 'string' && v.startsWith('brand:')) {
        selection = { kind: 'brand', slug: v.slice('brand:'.length) };
      }
      this.$store.commit('selectBrand', selection);
    },
  },
});
</script>
