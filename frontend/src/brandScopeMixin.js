// Fork (global brand, integrations GLOBAL-BRAND-SPEC D3/D4). The Vue half every Phase 1 page
// shares: the resolved selection and its effective list set from the store, and afterListsLoaded,
// which runs a page's FIRST scoped fetch only once the lists store has loaded (I13) -- never an
// unscoped read followed by a scoped one. The rules themselves are brandScope.mjs.
import { mapGetters, mapState } from 'vuex';

export default {
  computed: {
    ...mapState(['listsLoaded', 'brandRows']),
    ...mapGetters(['brandScope', 'brandSelection', 'scopeListIds', 'brandKey']),
  },

  methods: {
    afterListsLoaded(fn) {
      if (this.$store.state.listsLoaded) {
        fn();
        return;
      }
      const unwatch = this.$watch(() => this.$store.state.listsLoaded, (loaded) => {
        if (loaded) {
          unwatch();
          fn();
        }
      });
    },
  },
};
