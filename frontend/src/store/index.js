import Vue from 'vue';
import Vuex from 'vuex';
import { models } from '../constants';
import Utils from '../utils';
import {
  ALL, resolveSelection, rosterFrom, scopeListIds,
} from '../brandScope.mjs'; // eslint-disable-line import/extensions

Vue.use(Vuex);

// Fork (global brand, integrations GLOBAL-BRAND-SPEC S7/D1-D3). The selection lives in the pref
// `brand.global` and here, never in the URL. The pref's ONE writer is the selectBrand mutation
// (I9), which only the BrandSelector calls.
const prefs = new Utils();

export default new Vuex.Store({
  state: {
    // Data from API responses for different models, eg: lists, campaigns.
    // The API responses are stored in this map as-is. This is invoked by
    // API requests in `http`. This initialises lists: {}, campaigns: {}
    // etc. on state.
    ...Object.keys(models).reduce((obj, cur) => ({ ...obj, [cur]: [] }), {}),

    // Map of loading status (true, false) indicators for different model keys
    // like lists, campaigns etc. loading: {lists: true, campaigns: true ...}.
    // The Axios API global request interceptor marks a model as loading=true
    // and the response interceptor marks it as false. The model keys are being
    // pre-initialised here to fix "reactivity" issues on first loads.
    loading: Object.keys(models).reduce((obj, cur) => ({ ...obj, [cur]: false }), {}),

    // Fork (global brand, D3) -- set by the first minimal lists response App.vue requests. Until
    // then the selection is pending and no Phase 1 page issues its first scoped fetch.
    listsLoaded: false,
    // Fork (global brand, D1) -- the GET /api/brands rows, for the roster's labels and order only.
    brandRows: [],
    // Fork (global brand, S7) -- the user's stored choice; brandScope (below) resolves it.
    brandStored: prefs.getPref('brand.global') || ALL,
  },

  mutations: {
    // Set data from API responses. `model` is 'lists', 'campaigns' etc.
    setModelResponse(state, { model, data }) {
      state[model] = data;
    },

    // Set the loading status for a model globally. When a request starts,
    // status is set to true which is used by the UI to show loaders and block
    // forms. When a response is received, the status is set to false. This is
    // invoked by API requests in `http`.
    setLoading(state, { model, status }) {
      state.loading[model] = status;
    },

    // Fork (global brand, D3).
    setListsLoaded(state) {
      state.listsLoaded = true;
    },

    setBrandRows(state, rows) {
      state.brandRows = Array.isArray(rows) ? rows : [];
    },

    // Fork (global brand, D4, I9) -- the selector's change: the store and the pref, no reload.
    // The pref's one writer.
    selectBrand(state, selection) {
      state.brandStored = selection;
      prefs.setPref('brand.global', selection);
    },
  },

  getters: {
    [models.lists]: (state) => state[models.lists],
    [models.subscribers]: (state) => state[models.subscribers],
    [models.campaigns]: (state) => state[models.campaigns],
    [models.media]: (state) => state[models.media],
    [models.templates]: (state) => state[models.templates],
    [models.users]: (state) => state[models.users],
    [models.profile]: (state) => state[models.profile],
    [models.userRoles]: (state) => state[models.userRoles],
    [models.listRoles]: (state) => state[models.listRoles],
    [models.settings]: (state) => state[models.settings],
    [models.serverConfig]: (state) => state[models.serverConfig],
    [models.logs]: (state) => state[models.logs],

    // Fork (global brand, D1/D2). The roster of the permitted active lists and the resolved
    // selection ({ pending } until listsLoaded). Recomputed when the lists store changes, so a
    // stored brand whose lists all left the store falls back to All brands.
    brandRoster: (state) => rosterFrom(state[models.lists], state.brandRows),
    brandScope: (state, getters) => resolveSelection(state.brandStored, getters.brandRoster, state.listsLoaded),

    // The resolved selection (All brands while pending) and its effective list set (D3): null =
    // send no parameter.
    brandSelection: (state, getters) => (getters.brandScope.pending ? ALL : getters.brandScope.selection),
    scopeListIds: (state, getters) => scopeListIds(state[models.lists], getters.brandSelection),
    // A primitive the pages watch: it changes only when the resolved selection does (a lists
    // refresh that leaves the selection as it was does not re-fire a page's fetch).
    brandKey: (state, getters) => (getters.brandScope.pending ? 'pending' : JSON.stringify(getters.brandSelection)),
  },

  modules: {
  },
});
