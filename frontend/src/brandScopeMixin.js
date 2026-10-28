// Fork (global brand, integrations GLOBAL-BRAND-SPEC D3/D4). The Vue half every Phase 1 page
// shares: the resolved selection and its effective list set from the store, and afterListsLoaded,
// which runs a page's FIRST scoped fetch only once the selection has resolved (I13): the lists
// store has loaded and, under a narrowing launch filter, the health documents too -- never an
// unscoped read followed by a scoped one. The rules themselves are brandScope.mjs.
import { mapGetters, mapState } from 'vuex';

export default {
  computed: {
    ...mapState(['listsLoaded', 'brandRows']),
    ...mapGetters(['brandScope', 'brandSelection', 'scopeListIds', 'brandKey']),
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
  },
};
