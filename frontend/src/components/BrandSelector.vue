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
             name (brandScope.mjs::selectorOptions); No brand stays last. -->
        <!-- Fork (brand groups, integrations BRAND-GROUPS-SPEC D7) -- the offered groups (whole or
             absent, brandScope.mjs::offeredGroups), then the parent labelled "<name> (cust.)". -->
        <option v-for="g in options.groups" :key="g.group" :value="`group:${g.group}`">
          {{ $t(`brand.selector.${g.group}`) }}
        </option>
        <option v-if="options.parent" :value="`brand:${options.parent.slug}`">
          {{ $t('brand.selector.parent', { name: options.parent.label }) }}
        </option>
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
import { selectorOptions } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'BrandSelector',

  computed: {
    ...mapGetters(['brandScope', 'offeredRoster', 'brandGroups']),

    showNone() {
      return this.offeredRoster.none && !this.brandScope.locked;
    },

    // Fork (brand groups, BRAND-GROUPS-SPEC D7) -- groups, parent, rest and the divider rule.
    options() {
      return selectorOptions(this.offeredRoster.brands, this.brandGroups, this.showNone);
    },

    divider() {
      return this.options.divider;
    },

    value() {
      if (this.brandScope.pending) {
        return '';
      }
      const s = this.brandScope.selection;
      if (s.kind === 'group') {
        return `group:${s.group}`;
      }
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
      } else if (typeof v === 'string' && v.startsWith('group:')) {
        // No slugs: resolution adds them (BRAND-GROUPS-SPEC D7).
        selection = { kind: 'group', group: v.slice('group:'.length) };
      }
      this.$store.commit('selectBrand', selection);
    },
  },
});
</script>
