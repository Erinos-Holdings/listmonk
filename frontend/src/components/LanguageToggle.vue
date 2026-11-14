<template>
  <!-- Fork (naming standard, integrations NAMING-STANDARD-SPEC D2; global language, GLOBAL-LANG-SPEC
       D1/D2). The navbar language toggle All | EN | ES | FR | DE | IT, beside the brand selector, on
       every route with the navbar (hidden only while app.lang_enable is off). A presentation filter,
       never a scope or a permission. The selected button wears the view's applicability
       (langScope.mjs::LANG_APPLICABILITY): blue applies, orange partly applies, grey not
       applicable; off All a grey line beside it names what is filtered (the "Filtered" marker), and
       the title carries the same text -- colour is never the only signal. Never disabled: on a grey
       view it still changes the global selection. -->
  <div v-if="shown" class="language-toggle" :class="`lang-app-${applicability.state}`" data-cy="language-toggle"
    role="group" :aria-label="$t('naming.langToggle')" :title="noteText || descText">
    <b-field class="segmented is-global">
      <b-radio-button v-for="v in values" :key="v" v-model="value" :native-value="v" size="is-small" :type="buttonType"
        :data-cy="`lang-${v}`">
        {{ labelOf(v) }}
      </b-radio-button>
    </b-field>
    <span v-if="noteText" class="lang-note is-size-7 has-text-grey" data-cy="lang-note">{{ noteText }}</span>
  </div>
</template>

<script>
import Vue from 'vue';
import { mapGetters, mapState } from 'vuex';
import {
  LANG_VALUES, langApplicability, langLabel, langNote, langToggleVisible, routeLangLabel,
} from '../langScope.mjs'; // eslint-disable-line import/extensions
import { campaignLangLabel, sendLangLabel } from '../langs';

// The Buefy button type per applicability state (the classes are style.scss's).
const BUTTON_TYPES = Object.freeze({ all: 'is-primary', partial: 'is-lang-partial', none: 'is-lang-none' });

export default Vue.extend({
  name: 'LanguageToggle',

  data() {
    return { values: LANG_VALUES };
  },

  computed: {
    ...mapGetters(['langSelection', 'viewPane']),
    ...mapState(['serverConfig']),

    // D1 (S2-14): hidden everywhere while app.lang_enable is off.
    shown() {
      return langToggleVisible(this.serverConfig);
    },

    applicability() {
      const r = this.$route || {};
      return langApplicability(r.name, { pane: this.viewPane, query: r.query, selection: this.langSelection });
    },

    buttonType() {
      return BUTTON_TYPES[this.applicability.state] || BUTTON_TYPES.all;
    },

    // D2/I3: the grey line, only off All.
    noteText() {
      const n = langNote(this.langSelection, this.applicability);
      return n ? this.translate(n.key, n.lang, n.pageLang) : '';
    },

    // The title under All: the view's description, with the language left out.
    descText() {
      return this.translate(this.applicability.desc, this.langSelection, this.applicability.pageLang);
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

    // A Subscribers route value as the page's own picker names it (langScope.mjs::routeLangLabel,
    // the one mapper; S4-7).
    pageLangLabel(v) {
      return routeLangLabel(v, {
        all: this.$t('subscribers.langAll'),
        none: this.$t('lists.grid.noLangChip'),
        other: this.$t('lists.grid.other'),
        name: campaignLangLabel,
        sendName: sendLangLabel,
      });
    },

    translate(key, lang, pageLang) {
      return this.$t(key, { lang: this.labelOf(lang), page: this.pageLangLabel(pageLang) });
    },
  },
});
</script>

<style lang="scss">
.language-toggle {
  display: inline-flex;
  align-items: center;
  .field {
    margin-bottom: 0;
  }
  .lang-note {
    margin-left: 0.5rem;
    max-width: 22rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
