<template>
  <!-- Fork (hourly health, integrations listmonk/HOURLY-HEALTH-SPEC D4). The navbar's brand-health
       sync indicator, immediately left of Refresh: a glyph, then "Last brand health sync" (the
       newest stored computedAt among the loaded brand-health documents -- the store's one read)
       over "Next brand health sync" (the next scheduled hourly run). Every rule is healthSync.mjs;
       the glyph is a button only while the on-demand refresh is configured and the user holds
       brands:manage (healthRefresh.mjs::canRefresh, the SES pane's rule), and a click runs the SES
       pane's start-then-poll flow (healthRefresh.mjs::createRefresh). The glyph is an inline SVG of
       the mdi heart-pulse path (frontend/fontello/config.json) because the subset icon font does
       not carry it.
       integrations SYNC-ALL-SPEC D4: the sync is both display-feeding jobs -- brand health and
       campaign conversions (GET /api/campaigns/conversions' as_of, read only by a
       campaigns:get_analytics holder). "Last" is the OLDER of the two stamps; the hover lists each
       job's own time; a glyph click is done when BOTH stamps advance. -->
  <b-navbar-item tag="div" class="health-sync is-hidden-mobile" data-cy="health-sync">
    <b-tooltip :label="tooltip" type="is-dark" position="is-bottom" multilined>
      <span class="health-sync-inner">
        <a v-if="canClick" href="#" class="health-sync-glyph" :class="{ 'is-spinning': spinning }" role="button"
          :aria-label="$t('dashboard.ses.refresh')" data-cy="health-sync-refresh" @click.prevent="recompute">
          <svg viewBox="0 0 1000 1000" width="20" height="20" aria-hidden="true"><path :d="glyphPath" /></svg>
        </a>
        <span v-else class="health-sync-glyph" aria-hidden="true">
          <svg viewBox="0 0 1000 1000" width="20" height="20"><path :d="glyphPath" /></svg>
        </span>
        <span class="health-sync-lines">
          <span class="is-size-7" data-cy="health-sync-last">{{ $t('health.sync.last', { time: lines.last }) }}</span>
          <span class="is-size-7" data-cy="health-sync-next">{{ $t('health.sync.next', { time: lines.next }) }}</span>
        </span>
      </span>
    </b-tooltip>
  </b-navbar-item>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import { rereadDelay, syncLines, syncStamps } from '../healthSync.mjs'; // eslint-disable-line import/extensions
import {
  advancedAll, canRefresh, createRefresh, isActive,
} from '../healthRefresh.mjs'; // eslint-disable-line import/extensions

// mdi heart-pulse (Apache-2.0), in the 1000-unit box frontend/fontello/config.json stores it in.
const HEART_PULSE = [
  'M312.5 166Q250 166 197.3 197.3T114.3 281.3 84 396.5Q84 423.8 91.8 459H261.7L314.5 318.4Q322.3 300.8 337.9 294.9',
  'T369.1 294.9 392.6 318.4L478.5 541 503.9 482.4Q507.8 472.7 518.6 465.8T541 459H908.2Q916 423.8 916 396.5 916 334',
  ' 885.7 281.3T802.7 197.3 687.5 166Q630.9 166 581.1 192.4T500 263.7Q468.8 218.8 418.9 192.4T312.5 166V166Z',
  'M125 521.5Q107.4 521.5 95.7 533.2T84 562.5 95.7 591.8 125 603.5H226.6L459 834Q482.4 853.5 491.2 858.4',
  'T508.8 858.4 541 834L773.4 603.5H875Q892.6 603.5 904.3 591.8T916 562.5 904.3 533.2 875 521.5H558.6L519.5 617.2',
  'Q511.7 636.7 495.1 643.6T462.9 642.6 439.5 617.2L353.5 396.5 314.5 492.2Q308.6 505.9 298.8 513.7T275.4 521.5H125Z',
].join('');

export default Vue.extend({
  data() {
    return {
      now: Date.now(),
      refreshState: 'idle',
      glyphPath: HEART_PULSE,
      // The conversions doc's meta (GET /api/campaigns/conversions), null until read.
      conversionsMeta: null,
    };
  },

  computed: {
    ...mapState(['serverConfig', 'brandHealth']),

    // The jobs this user's indicator tracks: conversions only for a campaigns:get_analytics holder
    // (the meta read's permission); otherwise brand health alone, as before.
    jobs() {
      return this.$can('campaigns:get_analytics') ? ['brandHealth', 'conversions'] : ['brandHealth'];
    },

    lines() {
      return syncLines(this.brandHealth, this.now, this.conversionsMeta, this.jobs);
    },

    tooltip() {
      const j = this.lines.jobs;
      const each = this.jobs.length > 1
        ? this.$t('health.sync.jobs', { health: j.brandHealth, conversions: j.conversions })
        : this.$t('health.sync.jobHealth', { health: j.brandHealth });
      return `${each}. ${this.$t('health.sync.tip')}`;
    },

    canClick() {
      return canRefresh(this.serverConfig, (p) => this.$can(p));
    },

    spinning() {
      return isActive(this.refreshState);
    },
  },

  methods: {
    // page.refresh: the loud read (the user asked for it).
    reload() {
      this.now = Date.now();
      this.loadConversionsMeta();
      return this.$store.dispatch('loadBrandHealth');
    },

    // The conversions stamp's quiet read (SYNC-ALL-SPEC D3/D4); resolves to the meta, or the last
    // known one when the read fails. Skipped for a user who cannot read it.
    loadConversionsMeta() {
      if (!this.jobs.includes('conversions')) {
        return Promise.resolve(null);
      }
      return this.$api.getConversionsMetaQuiet().then((meta) => {
        this.conversionsMeta = meta || {};
        return this.conversionsMeta;
      }).catch(() => this.conversionsMeta);
    },

    // Both stamps, read quietly: { brandHealth, conversions }.
    readStamps() {
      return Promise.all([this.$store.dispatch('loadBrandHealthQuiet'), this.loadConversionsMeta()])
        .then(([rows, meta]) => syncStamps(rows, meta));
    },

    // One setTimeout chain: REREAD_DELAY_MS after each top of the hour, re-read both stamps
    // quietly (stage 4 F1: no loading flag, no toast).
    schedule() {
      this.timer = setTimeout(() => {
        this.timer = null;
        this.now = Date.now();
        this.readStamps();
        this.schedule();
      }, rereadDelay(Date.now()));
    },

    recompute() {
      this.refresher.begin(syncStamps(this.brandHealth, this.conversionsMeta));
    },
  },

  watch: {
    brandHealth() {
      this.now = Date.now();
    },

    conversionsMeta() {
      this.now = Date.now();
    },
  },

  created() {
    this.$root.$on('page.refresh', this.reload);
    // The SES pane's flow, its poll reading both stamps (the indicator's own reads): done once
    // EVERY tracked job's stamp moves past the click's baseline (SYNC-ALL-SPEC D4; the Function URL
    // starts both jobs, which run in parallel within the unchanged 3-minute cap).
    this.refresher = createRefresh({
      start: () => this.$api.refreshSystemHealth(),
      read: () => this.readStamps(),
      isDone: (baseline, stamps) => advancedAll(baseline, stamps, this.jobs),
      onState: (state) => {
        this.refreshState = state;
      },
    });
    this.loadConversionsMeta();
    this.schedule();
  },

  destroyed() {
    this.$root.$off('page.refresh', this.reload);
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.refresher.stop();
  },
});
</script>

<style lang="scss">
.health-sync {
  .health-sync-inner {
    display: inline-flex;
    align-items: center;
  }

  .health-sync-glyph {
    display: inline-flex;
    align-items: center;
    margin-right: 0.5rem;
    color: inherit;

    svg {
      fill: currentColor;
    }

    &.is-spinning svg {
      animation: health-sync-spin 1s linear infinite;
    }
  }

  .health-sync-lines {
    display: inline-flex;
    flex-direction: column;
    line-height: 1.25;
    white-space: nowrap;
  }
}

@keyframes health-sync-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
