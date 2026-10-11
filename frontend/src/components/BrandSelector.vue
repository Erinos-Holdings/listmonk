<template>
  <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D4). The navbar's brand context: All
       brands, the roster's brands by display name, No brand. A presentation filter only (S1).
       Disabled placeholder until the lists load (D3); disabled with the brand shown when locked
       (S5); hidden when the roster is empty (D2). -->
  <div v-if="!brandScope.hidden" class="brand-selector" data-cy="brand-selector">
    <b-select :value="value" :disabled="brandScope.pending || brandScope.locked" size="is-small"
      :aria-label="$t('brand.selector.label')" @input="onInput">
      <option v-if="brandScope.pending" value="">…</option>
      <template v-else>
        <option v-if="!brandScope.locked" value="all">{{ $t('brand.selector.all') }}</option>
        <option v-for="b in brandRoster.brands" :key="b.slug" :value="`brand:${b.slug}`">{{ b.label }}</option>
        <option v-if="brandRoster.none && !brandScope.locked" value="none">{{ $t('brand.selector.none') }}</option>
      </template>
    </b-select>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';

export default Vue.extend({
  name: 'BrandSelector',

  computed: {
    ...mapGetters(['brandScope', 'brandRoster']),

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
