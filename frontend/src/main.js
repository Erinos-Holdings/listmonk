import Vue from 'vue';
import Buefy from 'buefy';
import VueI18n from 'vue-i18n';

import App from './App.vue';
import router from './router';
import store from './store';
import * as api from './api';
import Utils from './utils';
import { routeRedirect } from './accessPolicy.mjs'; // eslint-disable-line import/extensions

// Internationalisation.
Vue.use(VueI18n);
const i18n = new VueI18n();

Vue.use(Buefy, {});
Vue.config.productionTip = false;

// Fork (brand analytics, integrations BRAND-ANALYTICS-SPEC D10). The logged-in profile, where the
// router guard and initConfig can both read it. It loads asynchronously AFTER the initial
// navigation starts (initConfig below), so it is null on a first load from a direct URL: the guard
// then makes no redirect decision and initConfig re-evaluates the landed route once it is known.
let currentProfile = null;

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
  } else {
    next();
  }
});

router.afterEach((to) => {
  Vue.nextTick(() => {
    const t = to.meta.title && i18n.te(to.meta.title) ? `${i18n.tc(to.meta.title, 0)} /` : '';
    document.title = `${t} listmonk`;
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
  // the route the initial navigation landed on (it ran before the profile existed). onReady waits
  // for that navigation to resolve, so a pending lazy route is judged once it is current.
  currentProfile = profile;
  router.onReady(() => {
    const cur = router.currentRoute;
    const redirect = routeRedirect(profile, cur.name, cur.params);
    if (redirect) {
      router.replace(redirect).catch(() => {});
    }
  });

  // Set the page title after i18n has loaded.
  const to = router.history.current;
  const title = to.meta.title ? `${i18n.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} listmonk`;

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
