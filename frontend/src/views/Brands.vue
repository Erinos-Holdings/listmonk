<template>
  <!-- Fork (brand health, integrations BRAND-HEALTH-SPEC D12). DISPOSABLE Vue (upstream deletes
  this SPA at v7). The page renders the documents the integrations BrandHealth Lambda writes
  (GET /api/brands/health, /api/brands/health/:brand) and computes nothing: every status, note,
  action and threshold is the document's. The only arithmetic is display formatting. -->
  <section class="brands">
    <header class="columns page-header">
      <div class="column">
        <h1 class="title is-4 mb-2">
          <router-link v-if="brandParam" :to="{ name: 'brands' }">{{ $t('brands.title') }}</router-link>
          <template v-else>{{ $t('brands.title') }}</template>
          <span v-if="!brandParam && rows.length">({{ rows.length }})</span>
          <span v-if="current"> / {{ nameOf(current) }}</span>
        </h1>
        <p class="is-size-7 has-text-grey">{{ $t('brands.gmailNote') }}</p>
      </div>
      <!-- BRANDS-UX-SPEC D4: hides exactly the documents whose own overall status is unknown
      (health-sort.mjs); the title's count stays the total. Sticky per browser. -->
      <div v-if="!brandParam && $can('brands:get')" class="column is-narrow has-text-right">
        <b-switch v-model="hideUnlaunched" data-cy="hide-unlaunched" @input="onHideChange">
          {{ $t('brands.hideUnlaunched', { n: unlaunchedCount }) }}
        </b-switch>
      </div>
    </header>

    <p v-if="!$can('brands:get')" class="has-text-grey">{{ $t('brands.noPermission') }}</p>

    <!-- Table -->
    <!-- BRANDS-UX-SPEC D3: backend-sorting -- Buefy never sorts; the view owns {field, order} and
    sorts (and filters) its own rows through health-sort.mjs. First click on any non-Brand column
    is descending (worst / largest first); the tiebreak is always Brand A-Z. -->
    <b-table v-else-if="!brandParam" ref="table" :data="visibleRows" :loading="loading.brands" hoverable
      backend-sorting :default-sort="[sort.field, sort.order]" @sort="onSort">
      <b-table-column v-slot="props" field="name" :label="$t('brands.brand')" sortable
        header-class="cy-name">
        <router-link :to="{ name: 'brand', params: { brand: props.row.brand } }">{{ nameOf(props.row) }}</router-link>
        <b-tag v-if="props.row.default" class="is-small ml-2">{{ $t('brands.defaultSender') }}</b-tag>
        <b-tag v-if="props.row.unmapped" class="is-small ml-2">{{ $t('brands.unmapped') }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="props" field="domain" :label="$t('brands.domain')">
        {{ props.row.domain }}
      </b-table-column>
      <b-table-column v-slot="props" field="channel" :label="$t('brands.channel')" sortable>
        {{ (props.row.facts && props.row.facts.channel) || '—' }}
      </b-table-column>
      <b-table-column v-slot="props" field="status" :label="$t('brands.health')" sortable
        header-class="cy-status">
        <health-chip :status="props.row.status" :note="props.row.note || ''" :to="chipTo(props.row, props.row.status)" />
      </b-table-column>
      <b-table-column v-for="k in inputKeys" :key="k" v-slot="props" :field="`input-${k}`"
        :label="$t(`brands.inputs.${k}`)" sortable>
        <health-chip :status="inputOf(props.row, k).status" :note="inputError(props.row, k)"
          :detail="inputOf(props.row, k).asOf ? $t('brands.asOf', { date: inputOf(props.row, k).asOf }) : ''"
          :to="chipTo(props.row, inputOf(props.row, k).status)" />
      </b-table-column>
      <b-table-column v-slot="props" field="day" :label="$t('brands.asOfCol')" sortable>
        {{ props.row.day }}
      </b-table-column>

      <template #empty v-if="!loading.brands">
        <p class="has-text-grey">{{ $t('brands.empty') }}</p>
      </template>
    </b-table>

    <!-- Detail -->
    <div v-else>
      <p v-if="!current && !loading.brands" class="has-text-grey">{{ $t('brands.notFound', { brand: brandParam }) }}</p>

      <div v-if="current" class="brand-detail">
        <div class="box">
          <div class="level">
            <div class="level-left">
              <div>
                <h2 class="title is-5 mb-1">{{ nameOf(current) }}</h2>
                <p class="is-size-7 has-text-grey">
                  {{ current.domain }}
                  <template v-if="current.default"> · {{ $t('brands.defaultSender') }}</template>
                  <template v-if="current.unmapped"> · {{ $t('brands.unmapped') }}</template>
                  · {{ $t('brands.asOf', { date: current.day }) }}
                </p>
              </div>
            </div>
            <div class="level-right">
              <health-chip :status="current.status" :note="current.note || ''" />
            </div>
          </div>
          <p v-if="current.note" class="is-size-7">{{ current.note }}</p>
          <div v-if="current.errors && current.errors.length" class="notification is-light mt-3">
            <p class="has-text-weight-semibold">{{ $t('brands.errors') }}</p>
            <ul><li v-for="(e, i) in current.errors" :key="i">{{ e }}</li></ul>
          </div>
        </div>

        <!-- Recommended actions -->
        <div v-if="current.actions && current.actions.length" class="box">
          <h3 class="title is-6">{{ $t('brands.actions') }}</h3>
          <div v-for="(a, i) in current.actions" :key="i" class="mb-3">
            <p class="has-text-weight-semibold">
              {{ a.title }} <span class="is-size-7 has-text-grey">({{ $t(`brands.inputs.${a.input}`) }})</span>
            </p>
            <p class="is-size-7">{{ a.detail }}</p>
          </div>
        </div>

        <!-- Status strip -->
        <div class="box">
          <h3 class="title is-6">{{ $t('brands.strip') }}</h3>
          <div class="status-strip">
            <b-tooltip v-for="d in strip" :key="d.day" :label="`${d.day} · ${d.status ? $t(`brands.status.${d.status}`) : $t('brands.noRow')}`"
              type="is-dark">
              <span :class="['strip-cell', d.status ? `health-${d.status}` : 'strip-empty']" />
            </b-tooltip>
          </div>
        </div>

        <!-- BRANDS-UX-SPEC D10: one left-aligned stack of half-width boxes (full width below the
        tablet breakpoint), in this order. Each box's source line (D9) is the document's own
        inputs[k].sources (facts.registrySource for Registry) -- no URL is composed here. -->
        <div class="brand-stack">
          <!-- Configuration -->
          <div class="box">
            <h3 class="title is-6">
              {{ $t('brands.inputs.config') }} <health-chip :status="inputOf(current, 'config').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'config')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('config')" />
            <p v-if="detailOf('config').rolledUpTo" class="is-size-7 mb-2">
              {{ $t('brands.rolledUpTo', { parent: detailOf('config').rolledUpTo }) }}
            </p>
            <p v-if="typeof inputOf(current, 'config').detail === 'string'" class="is-size-7">
              {{ inputOf(current, 'config').detail }}
            </p>
            <table v-else class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <tbody>
                <tr v-for="r in (detailOf('config').rows || [])" :key="r.requirement">
                  <td>{{ r.requirement }}</td><td><value-tag :level="complianceLevel(r.status)" :tip="$t('brands.tips.compliance')">{{ r.status }}</value-tag></td>
                </tr>
                <tr v-if="detailOf('config').oneClickUnsubscribe">
                  <td>{{ $t('brands.oneClick') }}</td>
                  <td>
                    <value-tag :level="complianceLevel(detailOf('config').oneClickUnsubscribe.status)" :tip="$t('brands.tips.compliance')">
                      {{ detailOf('config').oneClickUnsubscribe.status }}
                    </value-tag>
                  </td>
                </tr>
                <tr v-if="detailOf('config').honorUnsubscribe">
                  <td>{{ $t('brands.honorUnsub') }}</td>
                  <td>
                    <value-tag :level="complianceLevel(detailOf('config').honorUnsubscribe.status)" :tip="$t('brands.tips.compliance')">
                      {{ detailOf('config').honorUnsubscribe.status }}
                    </value-tag>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Verdict -->
          <div class="box">
            <h3 class="title is-6">
              {{ $t('brands.inputs.verdict') }} <health-chip :status="inputOf(current, 'verdict').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'verdict')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('verdict')" />
            <p v-if="typeof inputOf(current, 'verdict').detail === 'string'" class="is-size-7">
              {{ inputOf(current, 'verdict').detail }}
            </p>
            <table v-else class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <tbody>
                <tr>
                  <td>{{ $t('brands.verdictState') }}</td>
                  <td><value-tag :level="verdictLevel(detailOf('verdict'))" :tip="$t('brands.tips.verdict')">{{ detailOf('verdict').state || '—' }}</value-tag></td>
                </tr>
                <tr>
                  <td>{{ $t('brands.verdictReason') }}</td>
                  <td><value-tag :level="verdictLevel(detailOf('verdict'))" :tip="$t('brands.tips.verdict')">{{ detailOf('verdict').reason || '—' }}</value-tag></td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Spam rate -->
          <div class="box">
            <h3 class="title is-6">
              {{ $t('brands.inputs.spamRate') }} <health-chip :status="inputOf(current, 'spamRate').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'spamRate')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('spamRate')" />
            <p v-if="!spamSeries.length" class="is-size-7 has-text-grey">{{ $t('brands.noGmailDays') }}</p>
            <table v-else class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <thead><tr><th>{{ $t('brands.day') }}</th><th>{{ $t('brands.inputs.spamRate') }}</th><th /></tr></thead>
              <tbody>
                <tr v-for="p in spamSeries" :key="p.day">
                  <td>{{ p.day }}</td>
                  <td><value-tag :level="rateLevel(p.spamRate, detailOf('spamRate').thresholds)" :tip="tipSpamRate()">{{ pct(p.spamRate, 2) }}</value-tag></td>
                  <td class="bar-cell"><span class="bar" :style="{ width: barWidth(p.spamRate) }" /></td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Engagement (no source line: each send row links its campaign) -->
          <div class="box">
            <h3 class="title is-6">
              {{ $t('brands.inputs.engagement') }} <health-chip :status="inputOf(current, 'engagement').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'engagement')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('engagement')" />
            <p v-if="detailOf('engagement').viewRate !== undefined && detailOf('engagement').viewRate !== null"
              class="is-size-7 mb-2">
              {{ $t('brands.viewRate') }}:
              <value-tag :level="viewRateLevel(detailOf('engagement').viewRate, detailOf('engagement').thresholds)" :tip="tipViewRate()">
                {{ pct(detailOf('engagement').viewRate, 1) }}
              </value-tag>
            </p>
            <table v-if="(detailOf('engagement').sends || []).length" class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <thead>
                <tr>
                  <th>{{ $t('brands.campaign') }}</th><th>{{ $t('brands.sent') }}</th>
                  <th>{{ $t('brands.views') }}</th><th>{{ $t('brands.rate') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="s in detailOf('engagement').sends" :key="s.id">
                  <td>
                    <router-link v-if="s.id" :to="{ name: 'campaign', params: { id: s.id } }">{{ s.name || s.id }}</router-link>
                    <template v-else>{{ s.name }}</template>
                  </td>
                  <td>{{ s.sent }}</td><td>{{ s.views }}</td>
                  <td>
                    <value-tag v-if="s.sent" :level="viewRateLevel(s.views / s.sent, detailOf('engagement').thresholds)" :tip="tipViewRate()">
                      {{ pct(s.views / s.sent, 1) }}
                    </value-tag>
                    <template v-else>—</template>
                  </td>
                </tr>
              </tbody>
            </table>
            <p v-else class="is-size-7 has-text-grey">{{ $t('brands.noSends') }}</p>
          </div>

          <!-- Blocklists (no source line: each zone links its own lookup when the row carries url) -->
          <div class="box">
            <h3 class="title is-6">
              {{ $t('brands.inputs.dnsbl') }} <health-chip :status="inputOf(current, 'dnsbl').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'dnsbl')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('dnsbl')" />
            <table class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <tbody>
                <tr v-for="l in (detailOf('dnsbl').lists || [])" :key="l.zone">
                  <td>
                    <a v-if="isHttpUrl(l.url)" :href="l.url" target="_blank" rel="noopener">{{ l.zone }}</a>
                    <template v-else>{{ l.zone }}</template>
                  </td>
                  <td><value-tag :level="dnsblLevel(l.result)" :tip="$t('brands.tips.dnsbl')">{{ l.result }}</value-tag></td><td>{{ l.code || '' }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- SES (per-brand bounce/complaint rates; SES-HEALTH-SPEC D4). Everything shown is the
          document's: counts, rates (null when under the floor), which rate is classified, the
          floors, thresholds and the matched alarm states. -->
          <div class="box" data-cy="brand-ses">
            <h3 class="title is-6">
              {{ $t('brands.inputs.ses') }} <health-chip :status="inputOf(current, 'ses').status" />
              <span class="is-size-7 has-text-grey">{{ asOfText(inputOf(current, 'ses')) }}</span>
            </h3>
            <source-line :sources="sourcesOf('ses')" />
            <p v-if="typeof inputOf(current, 'ses').detail === 'string'" class="is-size-7">
              {{ inputOf(current, 'ses').detail }}
            </p>
            <template v-else-if="detailOf('ses').window">
              <p class="is-size-7 has-text-grey mb-2">
                {{ $t('brands.ses.window', { start: detailOf('ses').window.start, end: detailOf('ses').window.end }) }}
              </p>
              <table class="table is-narrow is-fullwidth is-size-7">
                <detail-cols />
                <tbody>
                  <tr><td>{{ $t('brands.ses.sends') }}</td><td>{{ detailOf('ses').sends }}</td></tr>
                  <tr><td>{{ $t('brands.ses.deliveries') }}</td><td>{{ detailOf('ses').deliveries }}</td></tr>
                  <tr><td>{{ $t('brands.ses.bounces') }}</td><td>{{ detailOf('ses').bounces }}</td></tr>
                  <tr><td>{{ $t('brands.ses.complaints') }}</td><td>{{ detailOf('ses').complaints }}</td></tr>
                  <tr><td>{{ $t('brands.ses.rejects') }}</td><td>{{ detailOf('ses').rejects }}</td></tr>
                  <tr>
                    <td>{{ $t('brands.ses.bounceRate') }}</td>
                    <td>
                      <value-tag v-if="sesClassified('bounce')" :level="rateLevel(detailOf('ses').bounceRate, sesThreshold('bounce'))" :tip="tipSesRate('bounce')">
                        {{ pct(detailOf('ses').bounceRate, 2) }}
                      </value-tag>
                      <value-tag v-else level="unknown" :tip="tipSesRate('bounce')">{{ $t('brands.ses.notClassified') }}</value-tag>
                      <span v-if="sesThreshold('bounce')" class="has-text-grey">
                        ({{ pct(sesThreshold('bounce').warn, 1) }} / {{ pct(sesThreshold('bounce').issues, 1) }})
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td>{{ $t('brands.ses.complaintRate') }}</td>
                    <td>
                      <value-tag v-if="sesClassified('complaint')" :level="rateLevel(detailOf('ses').complaintRate, sesThreshold('complaint'))" :tip="tipSesRate('complaint')">
                        {{ pct(detailOf('ses').complaintRate, 3) }}
                      </value-tag>
                      <value-tag v-else level="unknown" :tip="tipSesRate('complaint')">{{ $t('brands.ses.notClassified') }}</value-tag>
                      <span v-if="sesThreshold('complaint')" class="has-text-grey">
                        ({{ pct(sesThreshold('complaint').warn, 1) }} / {{ pct(sesThreshold('complaint').issues, 1) }})
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
              <h4 class="title is-6 mt-3 mb-2">{{ $t('brands.ses.alarms') }}</h4>
              <table class="table is-narrow is-fullwidth is-size-7">
                <detail-cols />
                <tbody>
                  <tr>
                    <td>{{ $t('brands.ses.alarmComplaints') }}</td>
                    <td>
                      <value-tag :level="alarmLevel(sesAlarm('complaints'))" :tip="$t('brands.tips.alarm', { metric: 'complaint' })">
                        {{ sesAlarm('complaints') || $t('brands.ses.noAlarm') }}
                      </value-tag>
                    </td>
                  </tr>
                  <tr>
                    <td>{{ $t('brands.ses.alarmBounces') }}</td>
                    <td><value-tag :level="alarmLevel(sesAlarm('bounces'))" :tip="$t('brands.tips.alarm', { metric: 'bounce' })">{{ sesAlarm('bounces') || $t('brands.ses.noAlarm') }}</value-tag></td>
                  </tr>
                </tbody>
              </table>
              <p v-if="!sesAlarm('complaints') || !sesAlarm('bounces')" class="is-size-7 has-text-grey">
                {{ $t('brands.ses.noAlarmHint') }}
              </p>
            </template>
          </div>

          <!-- Registry facts + lists. The facts table omits the document fields that are links or
          marks, not facts (explicit key list, BRANDS-UX-SPEC D9/review M3). -->
          <div class="box">
            <h3 class="title is-6">{{ $t('brands.facts') }}</h3>
            <source-line :sources="registrySources" />
            <table class="table is-narrow is-fullwidth is-size-7">
              <detail-cols />
              <tbody>
                <tr v-for="f in factRows" :key="f.key"><td>{{ f.key }}</td><td>{{ f.value }}</td></tr>
              </tbody>
            </table>
            <h4 class="title is-6 mt-4">{{ $t('globals.terms.lists') }}</h4>
            <ul v-if="(current.lists || []).length" class="is-size-7">
              <li v-for="l in current.lists" :key="l.id">
                <router-link :to="{ name: 'list', params: { id: l.id } }">{{ l.name }}</router-link>
              </li>
            </ul>
            <p v-else class="is-size-7 has-text-grey">{{ $t('brands.noLists') }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import HealthChip from '../components/HealthChip.vue';
import {
  compareDay, compareStatus, compareText, isUnlaunchedBrandDoc, sortRows,
} from '../health-sort';

// Fork (SES health, SES-HEALTH-SPEC D4/M4): `ses` is the sixth input; rows written before it
// existed lack it and render the unknown chip.
const INPUT_KEYS = ['config', 'verdict', 'spamRate', 'engagement', 'dnsbl', 'ses'];
const STRIP_DAYS = 30;
// BRANDS-UX-SPEC D5: chips in these states link to the brand's page.
const LINKED_STATUSES = ['warn', 'issues'];
// BRANDS-UX-SPEC D9/review M3: document facts that are a link or a mark, not a fact row.
const FACTS_EXCLUDED = ['logoUrl', 'registrySource'];
// BRANDS-UX-SPEC D3: text columns (Brand, Channel) sort A-Z on the first click; every other
// column is worst / largest first.
const ASC_FIRST_FIELDS = ['name', 'channel'];
const PREF_HIDE = 'brands.hideUnlaunched';

const isHttpUrl = (u) => typeof u === 'string' && /^https?:\/\//i.test(u);

// One small line of document-written links under a box title (BRANDS-UX-SPEC D9): labels
// separated by " · ", each opening in a new tab. Nothing when the array is empty or absent
// (documents written before `sources` existed).
const SourceLine = {
  name: 'SourceLine',
  props: { sources: { type: Array, default: () => [] } },
  render(h) {
    const list = (Array.isArray(this.sources) ? this.sources : []).filter((x) => x && isHttpUrl(x.url));
    if (!list.length) {
      return h();
    }
    const kids = [];
    list.forEach((x, i) => {
      if (i) {
        kids.push(' · ');
      }
      kids.push(h('a', { attrs: { href: x.url, target: '_blank', rel: 'noopener' } }, x.label || x.url));
    });
    return h('p', { class: 'is-size-7 source-line mb-2' }, kids);
  },
};

// One <colgroup> for every detail table so the 2nd/3rd/4th columns line up across the stacked
// boxes whatever a table's column count (widths in the .brand-stack styles).
const DetailCols = {
  name: 'DetailCols',
  render(h) {
    return h('colgroup', [1, 2, 3, 4].map((i) => h('col', { class: `dc${i}` })));
  },
};

// A value in the health-chip palette (ok / warn / issues / unknown) so the rows that drive a
// box's rollup stand out; no level → plain text. `tip` is the one-line rule the value is judged
// by (thresholds from the document), shown on hover so the page itself stays uncluttered.
const ValueTag = {
  name: 'ValueTag',
  props: { level: { type: String, default: '' }, tip: { type: String, default: '' } },
  render(h) {
    if (!this.level) {
      return h('span', this.$slots.default);
    }
    const tag = h('span', { class: ['tag', 'health-tag', `health-${this.level}`] }, this.$slots.default);
    const body = this.tip
      ? h('b-tooltip', { props: { label: this.tip, type: 'is-dark', multilined: true } }, [tag])
      : tag;
    return h('span', { class: 'health-chip' }, [body]);
  },
};

export default Vue.extend({
  components: {
    HealthChip, SourceLine, DetailCols, ValueTag,
  },

  data() {
    return {
      rows: [],
      history: [],
      inputKeys: INPUT_KEYS,
      // BRANDS-UX-SPEC D3/M6: the view owns the sort (Buefy's backend-sorting never sorts and
      // never emits the initial default-sort); a re-fetch never resets it.
      sort: { field: 'name', order: 'asc' },
      hideUnlaunched: this.$utils.getPref(PREF_HIDE) === true,
    };
  },

  computed: {
    ...mapState(['loading']),

    unlaunchedCount() {
      return this.rows.filter(isUnlaunchedBrandDoc).length;
    },

    // The table's rows: filtered by the hide toggle, sorted by the view's own sort state.
    visibleRows() {
      const rows = this.hideUnlaunched ? this.rows.filter((r) => !isUnlaunchedBrandDoc(r)) : this.rows;
      const comparators = {
        status: (a, b, o) => compareStatus(a.status, b.status, o),
        day: (a, b, o) => compareDay(a.day, b.day, o),
        channel: (a, b, o) => compareText(a.facts && a.facts.channel, b.facts && b.facts.channel, o),
      };
      INPUT_KEYS.forEach((k) => {
        comparators[`input-${k}`] = (a, b, o) => compareStatus(this.inputOf(a, k).status, this.inputOf(b, k).status, o);
      });
      return sortRows(rows, this.sort.field, this.sort.order, this.nameOf, comparators);
    },

    factRows() {
      const f = (this.current && this.current.facts) || {};
      return Object.keys(f).filter((k) => !FACTS_EXCLUDED.includes(k)).map((k) => ({ key: k, value: f[k] }));
    },

    registrySources() {
      const s = this.current && this.current.facts && this.current.facts.registrySource;
      return s && typeof s === 'object' ? [s] : [];
    },

    brandParam() {
      return this.$route.params.brand || '';
    },

    current() {
      if (!this.brandParam) {
        return null;
      }
      return this.rows.find((r) => r.brand === this.brandParam) || null;
    },

    // The last 30 UTC days, newest last; a day with no row is an empty cell.
    strip() {
      const byDay = {};
      this.history.forEach((d) => { byDay[d.day] = d.status; });
      const out = [];
      const now = new Date();
      for (let i = STRIP_DAYS - 1; i >= 0; i -= 1) {
        const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - i));
        const key = d.toISOString().slice(0, 10);
        out.push({ day: key, status: byDay[key] || '' });
      }
      return out;
    },

    spamBarFull() {
      const t = this.current && this.current.inputs && this.current.inputs.spamRate
        && this.current.inputs.spamRate.detail && this.current.inputs.spamRate.detail.thresholds;
      if (t && typeof t.issues === 'number' && t.issues > 0) {
        return t.issues;
      }
      return this.spamSeries.reduce((m, p) => Math.max(m, p.spamRate), 0);
    },

    // The spam-rate series from every stored row (newest row wins a day), newest first.
    spamSeries() {
      const byDay = {};
      [...this.history].reverse().forEach((doc) => {
        const s = doc.inputs && doc.inputs.spamRate && doc.inputs.spamRate.detail && doc.inputs.spamRate.detail.series;
        (Array.isArray(s) ? s : []).forEach((p) => {
          if (p && p.day && typeof p.spamRate === 'number') {
            byDay[p.day] = p;
          }
        });
      });
      return Object.keys(byDay).sort().reverse().map((k) => byDay[k]);
    },
  },

  methods: {
    nameOf(row) {
      if (row.unmapped) {
        return row.domain;
      }
      return (row.facts && row.facts.displayName) || row.brand;
    },

    inputOf(row, k) {
      return (row && row.inputs && row.inputs[k]) || { status: 'unknown' };
    },

    detailOf(k) {
      const d = this.inputOf(this.current, k).detail;
      return d && typeof d === 'object' ? d : {};
    },

    inputError(row, k) {
      const d = this.inputOf(row, k).detail;
      return typeof d === 'string' ? d : '';
    },

    sesClassified(kind) {
      const c = this.detailOf('ses').classified;
      return !!(c && c[kind]);
    },

    sesThreshold(kind) {
      const t = this.detailOf('ses').thresholds;
      return t && t[kind] && typeof t[kind].warn === 'number' && typeof t[kind].issues === 'number' ? t[kind] : null;
    },

    sesAlarm(which) {
      const a = this.detailOf('ses').alarms;
      return (a && a[which]) || '';
    },

    asOfText(input) {
      return input && input.asOf ? this.$t('brands.asOf', { date: input.asOf }) : '';
    },

    pct(v, digits) {
      if (typeof v !== 'number') {
        return '—';
      }
      return `${(v * 100).toFixed(digits)}%`;
    },

    // The bar's full width is the document's own issues threshold (spamRate.detail.thresholds,
    // written by the Lambda); a document without it falls back to a neutral scale -- the largest
    // value shown is full width. No threshold lives in the page.
    barWidth(v) {
      const full = this.spamBarFull;
      if (!full || typeof v !== 'number') {
        return '0%';
      }
      return `${Math.min(100, Math.max(0, (v / full) * 100))}%`;
    },

    sourcesOf(k) {
      const s = this.inputOf(this.current, k).sources;
      return Array.isArray(s) ? s : [];
    },

    // Per-VALUE levels. These restate, for display only, how the integrations Lambda classifies
    // each input (lib/brand-health.ts: configInput, verdictStatus, spamRateInput,
    // engagementInput, dnsblInput; lib/ses-health.ts classifyRate) using the document's OWN
    // thresholds where there are any -- the box chip beside them is still the Lambda's verdict.
    complianceLevel(status) {
      if (status === 'NEEDS_WORK') {
        return 'issues';
      }
      return status === 'COMPLIANT' ? 'ok' : 'unknown';
    },

    verdictLevel(v) {
      if (!v || v.reason === 'MESSAGE_VOLUME_LOW') {
        return 'unknown';
      }
      if (v.state === 'NEEDS_WORK') {
        return 'issues';
      }
      if (v.state === 'COMPLIANT') {
        return v.reason === 'USER_FEEDBACK_LOW' ? 'warn' : 'ok';
      }
      return 'unknown';
    },

    // A rate against {warn, issues} lines (spam rate, SES bounce/complaint); no lines → no tag.
    rateLevel(v, t) {
      if (typeof v !== 'number' || !t || typeof t.warn !== 'number' || typeof t.issues !== 'number') {
        return '';
      }
      if (v >= t.issues) {
        return 'issues';
      }
      return v >= t.warn ? 'warn' : 'ok';
    },

    // A view rate against {minViewRate, warnViewRate} floors (higher is better).
    viewRateLevel(v, t) {
      if (typeof v !== 'number' || !t || typeof t.minViewRate !== 'number' || typeof t.warnViewRate !== 'number') {
        return '';
      }
      if (v < t.minViewRate) {
        return 'issues';
      }
      return v < t.warnViewRate ? 'warn' : 'ok';
    },

    dnsblLevel(result) {
      if (result === 'listed') {
        return 'issues';
      }
      return result === 'clear' ? 'ok' : 'unknown';
    },

    // Hover text: the rule behind a tag, with the DOCUMENT's thresholds and floors filled in;
    // empty (no tooltip) when the document carries none.
    tipSpamRate() {
      const t = this.detailOf('spamRate').thresholds;
      return t && typeof t.warn === 'number' && typeof t.issues === 'number'
        ? this.$t('brands.tips.spamRate', { warn: this.pct(t.warn, 1), issues: this.pct(t.issues, 1) }) : '';
    },

    tipViewRate() {
      const t = this.detailOf('engagement').thresholds;
      return t && typeof t.minViewRate === 'number' && typeof t.warnViewRate === 'number'
        ? this.$t('brands.tips.viewRate', { sends: t.sends || '—', min: this.pct(t.minViewRate, 0), warn: this.pct(t.warnViewRate, 0) }) : '';
    },

    tipSesRate(kind) {
      const t = this.sesThreshold(kind);
      const f = this.detailOf('ses').floors || {};
      if (!t) {
        return '';
      }
      const warn = this.pct(t.warn, 1);
      const issues = this.pct(t.issues, 1);
      return kind === 'bounce'
        ? this.$t('brands.tips.bounceRate', { floor: f.bounceSends || '—', warn, issues })
        : this.$t('brands.tips.complaintRate', {
          floorDeliveries: f.complaintDeliveries || '—', floorCount: f.complaintCount || '—', warn, issues,
        });
    },

    alarmLevel(state) {
      if (!state) {
        return '';
      }
      if (state === 'ALARM') {
        return 'issues';
      }
      return state === 'OK' ? 'ok' : 'unknown';
    },

    isHttpUrl,

    chipTo(row, status) {
      return LINKED_STATUSES.includes(status) ? { name: 'brand', params: { brand: row.brand } } : null;
    },

    // The Lists.vue onSort pattern (BRANDS-UX-SPEC D3): on a click that CHANGES the column Buefy
    // has just set isAsc = true and emitted 'asc' -- for any column but Brand flip its isAsc (the
    // arrow reads it) and sort desc. A repeat click is Buefy's own toggle and passes through. If
    // isAsc is ever not a boolean (a Buefy upgrade), fall back to stock asc-first so the arrow and
    // the data cannot disagree.
    onSort(field, direction) {
      let order = direction;
      const { table } = this.$refs;
      if (!ASC_FIRST_FIELDS.includes(field) && field !== this.sort.field && direction === 'asc'
        && table && typeof table.isAsc === 'boolean') {
        table.isAsc = false;
        order = 'desc';
      }
      this.sort = { field, order };
    },

    onHideChange(v) {
      this.$utils.setPref(PREF_HIDE, v === true);
    },

    fetchRows() {
      if (!this.$can('brands:get')) {
        return;
      }
      this.$api.getBrandsHealth().then((data) => {
        this.rows = Array.isArray(data) ? data : [];
      });
    },

    fetchHistory() {
      this.history = [];
      if (!this.brandParam || !this.$can('brands:get')) {
        return;
      }
      const brand = this.brandParam;
      this.$api.getBrandHealthHistory(brand, 90).then((data) => {
        if (brand === this.brandParam) {
          this.history = Array.isArray(data) ? data : [];
        }
      });
    },
  },

  watch: {
    brandParam() {
      this.fetchHistory();
    },
  },

  mounted() {
    this.fetchRows();
    this.fetchHistory();
  },
});
</script>

<style lang="scss">
.brands {
  .status-strip {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }
  .strip-cell {
    display: inline-block;
    width: 14px;
    height: 22px;
    border-radius: 2px;
  }
  .strip-cell.health-ok { background: #3aa65b; }
  .strip-cell.health-warn { background: #e0a100; }
  .strip-cell.health-issues { background: #d33b3b; }
  .strip-cell.health-unknown { background: #bdbdbd; }
  .strip-cell.strip-empty { background: #f2f2f2; border: 1px dashed #d0d0d0; }
  // Every detail table shares one column grid (DetailCols), so the value columns line up
  // across the stacked boxes: label 45%, value 25%, then 15% + 15%.
  .brand-stack table {
    table-layout: fixed;
    col.dc1 { width: 45%; }
    col.dc2 { width: 25%; }
    col.dc3 { width: 15%; }
    col.dc4 { width: 15%; }
    td, th {
      overflow-wrap: anywhere;
    }
  }
  .bar-cell {
    width: 40%;
  }
  .bar {
    display: inline-block;
    height: 8px;
    background: #d33b3b;
    border-radius: 2px;
  }
  // BRANDS-UX-SPEC D10: every detail box -- headline, actions, strip and the stack -- is
  // left-aligned at half the content width; full width below Bulma's tablet breakpoint.
  .brand-detail > .box,
  .brand-stack > .box {
    max-width: 50%;
  }
  @media screen and (max-width: 768px) {
    .brand-detail > .box,
    .brand-stack > .box {
      max-width: 100%;
    }
  }
}
</style>
