import Vue from 'vue';
import Buefy from 'buefy';
import VueI18n from 'vue-i18n';
import 'emoji-picker-element';

import App from './App.vue';
import { filterCaption } from './filterBar.mjs'; // eslint-disable-line import/extensions
import router from './router';
import store from './store';
import * as api from './api';
import Utils from './utils';
import { routeRedirect } from './accessPolicy.mjs'; // eslint-disable-line import/extensions
import { langToggleVisible } from './langScope.mjs'; // eslint-disable-line import/extensions
import { subscribersRouteFix } from './subscriberFilters.mjs'; // eslint-disable-line import/extensions

// Internationalisation.
Vue.use(VueI18n);
const i18n = new VueI18n();

Vue.use(Buefy, {});

// Fork (polish 2026-11-14) -- the filter caption of every narrowed matrix/table ("BNKD Wellness ·
// English"; filterBar.mjs). Read in render, so the store getters it touches keep it reactive. opts:
// { brand, lang } -- whether that global filter narrows the table (both default true); langText --
// the page's own language filter, named by the page, in place of the global language.
Vue.prototype.$filterCaption = function filterCaptionOf(opts = {}) {
  const { langText, ...applies } = opts;
  return filterCaption({
    selection: this.$store.getters.brandSelection,
    lang: this.$store.getters.langEffective,
    brands: this.$store.state.brandRows,
    noBrand: this.$t('brand.selector.none'),
    applies,
    langText,
  });
};
Vue.config.productionTip = false;

// Fork (emoji picker, integrations EMOJI-PICKER-SPEC D4): the side-effect import defines the
// <emoji-picker> custom element once for this bundle (the builder iframe imports its own copy);
// Vue must not warn on the unknown tag. No element exists until a picker is first opened (D8).
Vue.config.ignoredElements = ['emoji-picker'];

// Fork (brand analytics, integrations BRAND-ANALYTICS-SPEC D10). The logged-in profile, where the
// router guard and initConfig can both read it. It loads asynchronously AFTER the initial
// navigation starts (initConfig below), so it is null on a first load from a direct URL: the guard
// then makes no redirect decision and initConfig re-evaluates the landed route once it is known.
let currentProfile = null;

// Fork (Subscribers filters, integrations SUBSCRIBERS-FILTERS-SPEC D3). Set once the server config
// (app.lang_enable) is in the store: before that the toggle reads as hidden, and the guard would
// drop a route's lang it cannot yet judge -- so the first navigation is decided by initConfig.
let langConfigReady = false;

// D3 (I6) -- the ONE decision for every navigation to a Subscribers route, at both sites (the
// guard and initConfig): commit a route language that differs from the selection (selectLang),
// then the corrected query, if any, as a replace. Returns the corrected location or null.
function subscribersFixOf(to) {
  const fix = subscribersRouteFix(to, store.getters.langSubscribers, langToggleVisible(store.state.serverConfig));
  if (fix.commitLang) {
    store.commit('selectLang', fix.commitLang);
  }
  return fix.query ? {
    path: to.path, query: fix.query, hash: to.hash, replace: true,
  } : null;
}

// Setup the router.
router.beforeEach((to, from, next) => {
  if (to.matched.length === 0) {
    next('/404');
    return;
  }

  // D10: analytics-only users are sent away from screens that would only 403. null while the
  // profile is not loaded, and for every other user.
  const redirect = routeRedirect(currentProfile, to.name, to.params);
  if (redirect) {
    next(redirect);
    return;
  }

  // Fork (Subscribers filters, SUBSCRIBERS-FILTERS-SPEC D3; was GLOBAL-LANG-SPEC D4's entry
  // redirect) -- every navigation to a Subscribers route (entry, in-page, Back/Forward) is made to
  // agree with the toggle BEFORE the page mounts, so the page never mounts with a state the rules
  // correct and issues no refused request. Until the server config is known the first load is
  // left to initConfig, which re-applies this before mounting.
  if (langConfigReady) {
    const langRedirect = subscribersFixOf(to);
    if (langRedirect) {
      next(langRedirect);
      return;
    }
  }
  next();
});

router.afterEach((to) => {
  Vue.nextTick(() => {
    const t = to.meta.title && i18n.te(to.meta.title) ? `${i18n.tc(to.meta.title, 0)} /` : '';
    document.title = `${t} Curated Email Campaign Manager`;
  });
});

async function initConfig(app) {
  // Load logged in user profile, server side config, and the language file before mounting the app.
  const [profile, cfg] = await Promise.all([api.getUserProfile(), api.getServerConfig()]);

  const lang = await api.getLang(cfg.lang);
  i18n.locale = cfg.lang;
  i18n.setLocaleMessage(i18n.locale, lang);

  Vue.prototype.$utils = new Utils(i18n);
  Vue.prototype.$api = api;
  Vue.prototype.$events = app;

  // $can('permission:name') is used in the UI to check whether the logged in user
  // has a certain permission to toggle visibility of UI objects and UI functionality.
  Vue.prototype.$can = (...perms) => {
    if (profile.userRole.id === 1) {
      return true;
    }

    // If the perm ends with a wildcard, check whether at least one permission
    // in the group is present. Eg: campaigns:* will return true if at least
    // one of campaigns:get, campaigns:manage etc. are present.
    return perms.some((perm) => {
      if (perm.endsWith('*')) {
        const group = `${perm.split(':')[0]}:`;
        return profile.userRole.permissions.some((p) => p.startsWith(group));
      }

      return profile.userRole.permissions.includes(perm);
    });
  };

  Vue.prototype.$canList = (id, perm) => {
    if (profile.userRole.id === 1) {
      return true;
    }

    // If the user role has global list permissions, return true.
    const can = Vue.prototype.$can('lists:get_all', 'lists:manage_all');
    if (can) {
      return true;
    }

    return profile.listRole.lists.some((list) => list.id === id && list.permissions.includes(perm));
  };

  // Fork (brand analytics, D10) -- publish the profile to the router guard, then apply the guard to
  // the route the initial navigation landed on (it ran before the profile existed). Awaited BEFORE
  // the app mounts: onReady waits for that navigation (lazy chunk included), and the replace must
  // resolve first, or the restricted view mounts, fetches and toasts a 403 before the redirect.
  currentProfile = profile;
  await new Promise((resolve) => { router.onReady(resolve); });
  const cur = router.currentRoute;
  const redirect = routeRedirect(profile, cur.name, cur.params);
  if (redirect) {
    await router.replace(redirect).catch(() => {});
  }
  // Fork (SUBSCRIBERS-FILTERS-SPEC D3; was GLOBAL-LANG-SPEC D4) -- the first navigation ran before
  // the server config (app.lang_enable) was loaded, so the guard left it alone; decide it now,
  // before the app mounts, and let the guard decide every later navigation.
  langConfigReady = true;
  const landed = router.currentRoute;
  const langRedirect = subscribersFixOf(landed);
  if (langRedirect) {
    await router.replace(langRedirect).catch(() => {});
  }

  // Set the page title after i18n has loaded.
  const to = router.history.current;
  const title = to.meta.title ? `${i18n.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} Curated Email Campaign Manager`;

  if (app) {
    app.$mount('#app');
  }
}

const v = new Vue({
  router,
  store,
  i18n,
  render: (h) => h(App),

  data: {
    isLoaded: false,
  },

  methods: {
    loadConfig() {
      initConfig();
    },

    // awaitRestart handles app restart polling after settings changes.
    // Shows a toast and polls until the backend is back up.
    // Returns a promise that resolves with { needsRestart: boolean }.
    awaitRestart(response) {
      return new Promise((resolve) => {
        // If there are running campaigns, app won't auto restart.
        if (response && typeof response === 'object' && response.needsRestart) {
          this.loadConfig();
          resolve({ needsRestart: true });
          return;
        }

        Vue.prototype.$utils.toast(i18n.t('settings.messengers.messageSaved'));

        // Poll until backend is back up.
        const pollId = setInterval(() => {
          api.getHealth().then(() => {
            clearInterval(pollId);
            this.loadConfig();
            resolve({ needsRestart: false });
          });
        }, 1000);
      });
    },
  },

  mounted() {
    v.isLoaded = true;
  },
});

initConfig(v);
