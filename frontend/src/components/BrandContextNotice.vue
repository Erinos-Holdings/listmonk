<template>
  <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D11, S4). A record outside the selected
       brand is opened, never hidden or redirected: this one line says so. The text is computed by
       brandScope.mjs contextNotice (null under All brands, for a matching record, and for a record
       whose brand cannot be derived). -->
  <div v-if="notice" class="notification is-warning is-light brand-context-notice" data-cy="brand-context-notice">
    {{ $t('brand.context.notice', { record: notice.record, kind, selected: notice.selected }) }}
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import { contextNotice } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'BrandContextNotice',

  props: {
    // The record's brand slug ('' = no brand); undefined when it cannot be derived.
    recordBrand: { type: String, default: undefined },
    // The record kind in the text ("campaign", "list", "template").
    kind: { type: String, required: true },
  },

  computed: {
    ...mapState(['brandRows']),
    ...mapGetters(['brandScope', 'brandSelection']),

    notice() {
      if (this.brandScope.pending) {
        return null;
      }
      const opts = { noneLabel: this.$t('brand.selector.none') };
      return contextNotice(this.recordBrand, this.brandSelection, this.brandRows, opts);
    },
  },
});
</script>
