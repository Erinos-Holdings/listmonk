<template>
  <!-- Fork (naming standard, integrations NAMING-STANDARD-SPEC D2). The navbar language toggle
       All | EN | ES | FR | DE | IT, beside the brand selector, on Campaigns, the Campaign Matrix and
       Templates only (langScope.mjs::LANG_ROUTES). A presentation filter -- a campaign with no
       language shows under every setting (naming.mjs::langMatches); never a scope or a permission. -->
  <div v-if="shown" class="language-toggle" data-cy="language-toggle" role="group" :aria-label="$t('naming.langToggle')">
    <b-field class="segmented is-global">
      <b-radio-button v-for="v in values" :key="v" v-model="value" :native-value="v" size="is-small" type="is-primary"
        :data-cy="`lang-${v}`">
        {{ labelOf(v) }}
      </b-radio-button>
    </b-field>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters } from 'vuex';
import { LANG_VALUES, langLabel, langToggleShown } from '../langScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'LanguageToggle',

  data() {
    return { values: LANG_VALUES };
  },

  computed: {
    ...mapGetters(['langSelection']),

    shown() {
      return langToggleShown(this.$route && this.$route.name);
    },

    // D2: the control is the selectLang mutation's only caller.
    value: {
      get() {
        return this.langSelection;
      },
      set(v) {
        this.$store.commit('selectLang', v);
      },
    },
  },

  methods: {
    labelOf(v) {
      return langLabel(v, this.$t('naming.langAll'));
    },
  },
});
</script>

<style lang="scss">
.language-toggle {
  .field {
    margin-bottom: 0;
  }
}
</style>
