// Fork (global brand, integrations GLOBAL-BRAND-SPEC D3/D4). The Vue half every Phase 1 page
// shares: the resolved selection and its effective list set from the store, and afterListsLoaded,
// which runs a page's FIRST scoped fetch only once the selection has resolved (I13): the lists
// store has loaded and, under a narrowing launch filter, the health documents too -- never an
// unscoped read followed by a scoped one. brandChanged is every page's brandKey watcher guard.
// The rules themselves are brandScope.mjs.
import { mapGetters, mapState } from 'vuex';
import { brandKeyStep } from './brandScope.mjs'; // eslint-disable-line import/extensions

export default {
  computed: {
    ...mapState(['listsLoaded', 'brandRows']),
    ...mapGetters(['brandScope', 'brandSelection', 'scopeListIds', 'brandKey']),
  },

  created() {
    // The key the page's data was last fetched under; non-reactive. Mounted after the scope has
    // resolved, that is the current key (afterListsLoaded fetches under it at once).
    const k = this.$store.getters.brandKey;
    this.seenBrandKey = k === 'pending' ? null : k;
  },

  methods: {
    afterListsLoaded(fn) {
      if (!this.$store.getters.brandScope.pending) {
        fn();
        return;
      }
      const unwatch = this.$watch(() => this.$store.getters.brandScope.pending, (pending) => {
        if (!pending) {
          unwatch();
          fn();
        }
      });
    },

    // True when the page must refetch for the new brandKey (brandScope.mjs::brandKeyStep).
    brandChanged(now) {
      const step = brandKeyStep(this.seenBrandKey, now);
      this.seenBrandKey = step.seen;
      return step.changed;
    },
  },
};
