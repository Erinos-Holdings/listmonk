<template>
  <div id="app">
    <!-- Fork (campaign review, D11) -- a bare route renders its view alone. -->
    <router-view v-if="$root.isLoaded && isBare" :key="$route.fullPath" />

    <b-navbar :fixed-top="true" v-if="$root.isLoaded && !isBare">
      <template #brand>
        <div class="logo">
          <router-link :to="{ name: 'dashboard' }">
            <img class="full" src="@/assets/curated-ecm-logo.png" alt="" />
            <img class="favicon" src="@/assets/favicon.png" alt="" />
          </router-link>
        </div>
      </template>
      <template #end>
        <navigation v-if="isMobile" :is-mobile="isMobile" :active-item="activeItem" :active-group="activeGroup"
          @toggleGroup="toggleGroup" @doLogout="doLogout" />

        <b-navbar-item tag="a" href="#" @click.prevent="emitPageRefresh" data-cy="btn-refresh"
          :aria-label="$t('globals.buttons.refresh')">
          <b-tooltip :label="$t('globals.buttons.refresh')" type="is-dark" position="is-bottom">
            <b-icon icon="refresh" /> <span class="is-hidden-tablet">{{ $t('globals.buttons.refresh') }}</span>
          </b-tooltip>
        </b-navbar-item>

        <b-navbar-dropdown class="user" tag="div" right>
          <template v-if="profile.username" #label>
            <span class="user-avatar">
              <img v-if="profile.avatar" :src="profile.avatar" alt="" />
              <span v-else>{{ profile.username[0].toUpperCase() }}</span>
            </span>
          </template>

          <b-navbar-item class="user-name" tag="router-link" to="/user/profile">
            <strong>{{ profile.username }}</strong>
            <div class="is-size-7">{{ profile.name }}</div>
          </b-navbar-item>

          <b-navbar-item href="#">
            <router-link to="/user/profile">
              <b-icon icon="account-outline" /> {{ $t('users.profile') }}
            </router-link>
          </b-navbar-item>
          <b-navbar-item href="#">
            <a href="#" @click.prevent="doLogout"><b-icon icon="logout-variant" /> {{ $t('users.logout') }}</a>
          </b-navbar-item>
        </b-navbar-dropdown>
      </template>
    </b-navbar>

    <div class="wrapper" v-if="$root.isLoaded && !isBare">
      <section class="sidebar">
        <b-sidebar position="static" mobile="hide" :fullheight="true" :open="true" :can-cancel="false">
          <div>
            <b-menu :accordion="false">
              <navigation v-if="!isMobile" :is-mobile="isMobile" :active-item="activeItem" :active-group="activeGroup"
                @toggleGroup="toggleGroup" />
            </b-menu>
          </div>
        </b-sidebar>
      </section>
      <!-- sidebar-->

      <!-- body //-->
      <div class="main">
        <div class="global-notices" v-if="isGlobalNotices">
          <div v-if="serverConfig.needs_restart" class="notification is-danger">
            {{ $t('settings.needsRestart') }}
            &mdash;
            <b-button class="is-primary" size="is-small"
              @click="$utils.confirm($t('settings.confirmRestart'), reloadApp)">
              {{ $t('settings.restart') }}
            </b-button>
          </div>

          <template v-if="serverConfig.update">
            <div v-if="serverConfig.update.update.is_new" class="notification is-success">
              {{ $t('settings.updateAvailable', {
                version: `${serverConfig.update.update.release_version}
              (${$utils.getDate(serverConfig.update.update.release_date).format('DD MMM YY')})`,
              }) }}
              <a :href="serverConfig.update.update.url" target="_blank" rel="noopener noreferer">View</a>
            </div>

            <template v-if="serverConfig.update.messages && serverConfig.update.messages.length > 0">
              <div v-for="m in serverConfig.update.messages" class="notification"
                :class="{ [m.priority === 'high' ? 'is-danger' : 'is-info']: true }" :key="m.title">
                <h3 class="is-size-5" v-if="m.title"><strong>{{ m.title }}</strong></h3>
                <p v-if="m.description">{{ m.description }}</p>
                <a v-if="m.url" :href="m.url" target="_blank" rel="noopener noreferer">View</a>
              </div>
            </template>
          </template>

          <div v-if="serverConfig.has_legacy_user" class="notification is-danger">
            <b-icon icon="warning-empty" />
            Remove the <code>admin_username</code> and <code>admin_password</code> fields from the TOML
            configuration file or environment variables. If you are using APIs, create and use new API credentials
            before removing them. Visit
            <router-link :to="{ name: 'users' }">
              Admin -> Settings -> Users
            </router-link> dashboard. <a href="https://listmonk.app/docs/upgrade/#upgrading-to-v4xx" target="_blank"
              rel="noopener noreferer">Learn more.</a>
          </div>
        </div>

        <router-view :key="$route.fullPath" />
      </div>
    </div>

    <b-loading v-if="!$root.isLoaded" active />
  </div>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import { NavigationFailureType, isNavigationFailure } from 'vue-router';
import {
  RESULT_TIMEOUT_MS, acceptSelect, ackMessage, campaignLocation, isFinalResult, pushFailureResult, receiverRoute,
} from './reviewNavigate.mjs'; // eslint-disable-line import/extensions
import { clearAllDrafts } from './drafts';
import { uris } from './constants';
import stepUp, { stepUpError } from './stepUp';

import Navigation from './components/Navigation.vue';

export default Vue.extend({
  name: 'App',

  components: {
    Navigation,
  },

  data() {
    return {
      // Fork (session expiry): throttle for checkSessionOnFocus.
      lastSessionCheck: 0,
      activeItem: {},
      activeGroup: {},
      windowWidth: window.innerWidth,
    };
  },

  watch: {
    $route(to) {
      // Set the current route name to true for active+expanded keys in the
      // menu to pick up.
      this.activeItem = { [to.name]: true };
      if (to.meta.group) {
        this.activeGroup = { [to.meta.group]: true };
      } else {
        // Reset activeGroup to collapse menu items on navigating
        // to non group items from sidebar
        this.activeGroup = {};
      }
    },
  },

  methods: {
    toggleGroup(group, state) {
      this.activeGroup = state ? { [group]: true } : {};
    },

    emitPageRefresh() {
      this.$root.$emit('page.refresh');
    },

    // Fork (integrations STEPUP-ADMIN-SPEC D4) -- the reload steps up first.
    reloadApp() {
      stepUp(this).then(() => this.$api.reloadApp()).then(() => {
        this.$utils.toast('Reloading app ...');

        // Poll until there's a 200 response, waiting for the app
        // to restart and come back up.
        const pollId = setInterval(() => {
          this.$api.getHealth().then(() => {
            clearInterval(pollId);
            document.location.reload();
          });
        }, 500);
      }).catch(stepUpError(this));
    },

    doLogout() {
      // Fork (session expiry): a stashed draft must not survive into the next login on
      // a shared browser. The restore path also checks userId for a logout that never ran.
      clearAllDrafts();
      this.$api.logout().then(() => {
        document.location.href = uris.root;
      });
    },

    // Fork (session expiry): explain before leaving. This listener is registered at app
    // mount, BEFORE any view's, so it runs first in the synchronous dispatch — the dialog is
    // therefore deferred one tick, by which time the campaign editor's stash listener has
    // run and settled detail.stashed. The dialog cannot be dismissed any other way — Esc or
    // click-outside would strand the admin on a page whose every request will fail.
    onSessionExpired(e) {
      const d = e.detail;
      if (!d) {
        return;
      }
      d.handled = true;
      setTimeout(() => {
        let msgKey = 'users.sessionExpiredPlain';
        if (d.stashed) {
          msgKey = 'users.sessionExpiredStashed';
        } else if (d.skipped) {
          msgKey = 'users.sessionExpiredSkipped';
        }
        this.$buefy.dialog.alert({
          title: this.$t('users.sessionExpiredTitle'),
          message: this.$t(msgKey),
          confirmText: this.$t('users.sessionExpiredSignIn'),
          type: 'is-primary',
          hasIcon: true,
          icon: 'clock-alert-outline',
          canCancel: false,
          scroll: 'keep',
          onConfirm: () => d.proceed(),
        });
      }, 0);
    },

    // Fork (session expiry): a tab returning to the foreground checks its session before
    // the admin starts typing. A dead session fails through the api interceptor, which
    // stashes and redirects; success is a no-op and doubles as the keep-alive touch.
    checkSessionOnFocus() {
      if (document.visibilityState !== 'visible') {
        return;
      }
      const now = Date.now();
      if (now - this.lastSessionCheck < 60000) {
        return;
      }
      this.lastSessionCheck = now;
      this.$api.pingSession().catch(() => {});
    },

    // Fork (review navigation) -- integrations REVIEW-NAVIGATION-SPEC §4.5: the receiver for the
    // Inspect window's block references. App.vue survives route changes, so it holds the ONE
    // pending request; the campaign page does the selecting and answers via `review.select-result`.
    // Acts only on what reviewNavigate.mjs `acceptSelect` accepts (our origin, a top-level other
    // window, the type, valid ids, not the Inspect window itself); anything else gets no reply.
    onReviewMessage(ev) {
      const { origin } = window.location;
      const req = acceptSelect({
        origin: ev.origin, ourOrigin: origin, source: ev.source, self: window, data: ev.data, routeName: this.$route.name,
      });
      if (!req) {
        return;
      }
      try {
        ev.source.postMessage(ackMessage(req.token, 'received'), origin);
      } catch (e) {
        return;
      }
      // A new request replaces the pending one; the replaced one gets no second reply.
      if (this.reviewPending) {
        clearTimeout(this.reviewPending.timer);
      }
      const pending = {
        source: ev.source, token: req.token, campaignId: req.campaignId, blockId: req.blockId, startedAt: Date.now(), timer: null,
      };
      pending.timer = setTimeout(() => this.replyReview(pending, 'unknown'), RESULT_TIMEOUT_MS);
      this.reviewPending = pending;

      if (receiverRoute({ routeName: this.$route.name, routeId: this.$route.params.id, campaignId: req.campaignId }) === 'emit') {
        this.$events.$emit('review.select', { campaignId: req.campaignId, blockId: req.blockId });
        return;
      }
      // Another route: navigate. The campaign page's leave guard may decline (Cancel settles the
      // navigation with next(false), so the push rejects); a resolved push mounts a campaign page
      // that selects from the query and emits the result.
      this.$router.push(campaignLocation(req.campaignId, req.blockId)).catch((err) => {
        this.replyReview(pending, pushFailureResult(err, (e) => isNavigationFailure(e, NavigationFailureType.aborted)));
      });
    },

    onReviewSelectResult(r) {
      const p = this.reviewPending;
      if (!p || !r || r.blockId !== p.blockId || (r.campaignId !== undefined && r.campaignId !== p.campaignId)) {
        return;
      }
      if (isFinalResult(r.result)) {
        this.replyReview(p, r.result);
      }
    },

    // The second (and last) reply to a pending request, once.
    replyReview(p, result) {
      if (this.reviewPending !== p) {
        return;
      }
      clearTimeout(p.timer);
      this.reviewPending = null;
      try {
        p.source.postMessage(ackMessage(p.token, result), window.location.origin);
      } catch (e) {
        // The Inspect window is gone; nothing to tell.
      }
      if (result === 'selected') {
        // The raise is the browser's decision.
        window.focus();
      }
    },

    listenEvents() {
      const reMatchLog = /(.+?)\.go:\d+:(.+?)$/im;
      const evtSource = new EventSource(uris.errorEvents, { withCredentials: true });
      let numEv = 0;
      evtSource.onmessage = (e) => {
        if (numEv > 50) {
          return;
        }
        numEv += 1;

        const d = JSON.parse(e.data);
        if (d && d.type === 'error') {
          const msg = reMatchLog.exec(d.message.trim());
          this.$utils.toast(msg[2], 'is-danger', null, true);
        }
      };
    },
  },

  computed: {
    ...mapState(['serverConfig', 'profile']),

    // Fork (campaign review, D11): routes with meta.bare render without the sidebar and navbar.
    isBare() {
      return !!(this.$route.meta && this.$route.meta.bare);
    },

    isGlobalNotices() {
      return (this.serverConfig.needs_restart
        || this.serverConfig.has_legacy_user
        || (this.serverConfig.update
          && this.serverConfig.update.messages
          && this.serverConfig.update.messages.length > 0));
    },

    version() {
      return import.meta.env.VUE_APP_VERSION;
    },

    isMobile() {
      return this.windowWidth <= 768;
    },
  },

  mounted() {
    // Lists is required across different views. On app load, fetch the lists
    // and have them in the store.
    this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });

    window.addEventListener('resize', () => {
      this.windowWidth = window.innerWidth;
    });

    this.listenEvents();

    this.lastSessionCheck = Date.now();
    document.addEventListener('visibilitychange', this.checkSessionOnFocus);
    window.addEventListener('listmonk:session-expired', this.onSessionExpired);

    // Fork (review navigation): the Inspect window's block references (§4.5). The pending request
    // holds another window, so it is kept off the reactive data.
    this.reviewPending = null;
    window.addEventListener('message', this.onReviewMessage);
    this.$events.$on('review.select-result', this.onReviewSelectResult);
  },
});
</script>

<style lang="scss">
@import "assets/style.scss";
@import "assets/icons/fontello.css";
</style>
