<template>
  <!-- Fork (campaign review, integrations CAMPAIGN-INSPECT-SPEC D11/§3.2) -- the checklist window: a
       bare route opened from the campaign page's Inspect button (window lm-review-<id>). It renders
       the review Lambda's report and the FORK's verdict (never recomputed here), records the three
       dispositions, applies the closed "Fix for me" list (reviewFixes.mjs) and re-inspects. -->
  <section class="campaign-review" data-cy="campaign-review">
    <header class="review-header">
      <div>
        <h4 class="title is-4">{{ campaign.name || `#${id}` }}</h4>
        <p class="subtitle is-6 mb-1">{{ campaign.subject }}</p>
        <p class="is-size-7 has-text-grey">
          <span v-for="l in (campaign.lists || [])" :key="l.id" class="mr-2">{{ l.name }}</span>
          <span v-if="lang" class="mr-2">· {{ lang.toUpperCase() }}</span>
          <span v-if="report && report.provenance">· {{ $t('campaigns.review.sectionProvenance') }}: {{ report.rubricVersion }}</span>
        </p>
      </div>
      <div class="verdict-banner" :class="`is-${verdictName}`" data-cy="review-verdict">
        <template v-if="isRunning">{{ $t('campaigns.review.running') }}</template>
        <template v-else-if="verdictName === 'pass'">{{ $t('campaigns.review.verdictPass') }}</template>
        <template v-else-if="verdictName === 'blocked'">
          {{ $t('campaigns.review.verdictBlocked', { n: blockers.length }) }}
        </template>
        <template v-else>{{ $t('campaigns.review.verdictNone') }}</template>
      </div>
    </header>

    <!-- Running: the Lambda's progress stage. -->
    <div v-if="isRunning" class="review-progress" data-cy="review-progress">
      <b-progress :value="progressValue" type="is-info" show-value format="percent" />
      <p class="is-size-7">{{ stageLabel }}</p>
    </div>

    <b-message v-if="row && row.status === 'failed'" type="is-danger" data-cy="review-failed">
      {{ $t('campaigns.review.failed') }}
      <b-button size="is-small" class="ml-2" @click="reinspect" :loading="busy">{{ $t('campaigns.review.retry') }}</b-button>
    </b-message>
    <b-message v-else-if="row && row.status === 'stale'" type="is-warning">
      {{ $t('campaigns.review.stale') }}
      <b-button size="is-small" class="ml-2" @click="reinspect" :loading="busy">{{ $t('campaigns.review.reinspect') }}</b-button>
    </b-message>
    <b-message v-else-if="isEdited" type="is-warning" data-cy="review-edited">
      {{ $t('campaigns.review.edited') }}
    </b-message>

    <template v-if="report && !isRunning">
      <!-- AI unavailable / partial: the user override (pseudo-key A). -->
      <b-message v-if="report.ai && report.ai.status !== 'ok'" type="is-warning" data-cy="review-ai">
        {{ $t('campaigns.review.aiUnavailable', { status: report.ai.status }) }}
        <b-button size="is-small" class="ml-2" :type="stagedAction('A') === 'accept' ? 'is-primary' : ''"
          :disabled="decidedAction('A') === 'accept'" @click="stage('A', 'A', 'accept')">
          {{ $t('campaigns.review.aiAccept') }}
        </b-button>
        <span v-if="decided('A')" class="is-size-7 ml-2">{{ decidedLine(decided('A')) }}</span>
      </b-message>

      <section v-for="sec in sections" :key="sec.key" class="review-section" :data-cy="`section-${sec.key}`">
        <h5 class="title is-5">{{ sec.label }} <span class="tag">{{ sec.entries.length }}</span></h5>
        <div v-for="e in sec.entries" :key="e.key" class="review-item box" :data-cy="`item-${e.item.id}`">
          <div class="item-head">
            <strong>{{ e.item.id }}</strong> {{ e.item.title }}
            <b-tag :type="tagType(e)" class="ml-2">{{ e.finding.severity || e.item.verdict }}</b-tag>
            <b-tag v-if="stillOpen(e)" type="is-warning" class="ml-1">{{ $t('campaigns.review.stillOpen') }}</b-tag>
          </div>
          <p class="todo">{{ e.finding.todo }}</p>
          <p class="is-size-7">
            <span class="has-text-grey">{{ $t('campaigns.review.evidence') }}:</span>
            <img v-if="isImageUrl(e.finding.evidence)" :src="e.finding.evidence" alt="" class="evidence-thumb" />
            <q v-else>{{ e.finding.evidence }}</q>
          </p>
          <p class="is-size-7">
            <span class="has-text-grey">{{ $t('campaigns.review.location') }}:</span> {{ e.finding.location }}
          </p>
          <p v-if="decided(e.key)" class="is-size-7 has-text-grey">{{ decidedLine(decided(e.key)) }}</p>
          <div class="buttons mt-2">
            <b-button size="is-small" :disabled="!e.finding.fix" data-cy="btn-fix-for-me"
              :type="stagedAction(e.key) === 'fixed' ? 'is-primary' : ''" @click="stage(e.key, e.item.id, 'fixed', e.finding.fix)">
              {{ $t('campaigns.review.fixForMe') }}
            </b-button>
            <b-button size="is-small" data-cy="btn-ill-fix-it" :type="stagedAction(e.key) === 'fixme' ? 'is-primary' : ''"
              @click="stage(e.key, e.item.id, 'fixme')">
              {{ $t('campaigns.review.illFixIt') }}
            </b-button>
            <b-tooltip v-if="!e.item.acceptable" :label="$t('campaigns.review.cannotAccept')" type="is-dark">
              <b-button size="is-small" disabled>{{ $t('campaigns.review.acceptRisk') }}</b-button>
            </b-tooltip>
            <b-button v-else size="is-small" data-cy="btn-accept" :type="stagedAction(e.key) === 'accept' ? 'is-primary' : ''"
              @click="stage(e.key, e.item.id, 'accept')">
              {{ e.finding.severity === 'high' ? $t('campaigns.review.acknowledge') : $t('campaigns.review.acceptRisk') }}
            </b-button>
          </div>
        </div>
      </section>

      <b-collapse :open="false" class="review-section">
        <template #trigger="props">
          <h5 class="title is-5 is-clickable">
            <b-icon :icon="props.open ? 'chevron-down' : 'chevron-right'" size="is-small" />
            {{ $t('campaigns.review.sectionPassed') }} <span class="tag">{{ passed.length }}</span>
          </h5>
        </template>
        <ul class="passed-list is-size-7">
          <li v-for="i in passed" :key="i.id">
            <strong>{{ i.id }}</strong> {{ i.title }} —
            <template v-if="i.verdict === 'n/a'">n/a: {{ i.reason }}</template>
            <template v-else>{{ $t('campaigns.review.examined', { n: i.examined || 0 }) }}</template>
          </li>
        </ul>
      </b-collapse>

      <section v-if="report.context && report.context.length" class="review-section">
        <h5 class="title is-5">{{ $t('campaigns.review.sectionContext') }}</h5>
        <ul class="is-size-7">
          <li v-for="(c, n) in report.context" :key="n">
<strong>{{ c.label }}</strong>: {{ c.value }}
            <span v-if="c.asOf" class="has-text-grey">({{ c.asOf }})</span>
</li>
        </ul>
      </section>

      <section v-if="report.structure" class="review-section" data-cy="section-structure">
        <h5 class="title is-5">{{ $t('campaigns.review.sectionStructure') }}</h5>
        <p v-if="report.structure.verified" class="is-size-7">
          {{ $t('campaigns.review.structureVerified', { date: report.structure.verified.verified_at.slice(0, 10), id: report.structure.verified.test_id }) }}
        </p>
        <div v-else class="is-size-7">
          <p>{{ $t('campaigns.review.structureOffer', { differs: (report.structure.differs || []).join(', ') }) }}</p>
          <pre class="structure-cmd">{{ report.structure.command }}</pre>
          <b-button size="is-small" :type="stagedAction('R') === 'accept' ? 'is-primary' : ''" @click="stage('R', 'R', 'accept')">
            {{ $t('campaigns.review.declineMatrix') }}
          </b-button>
          <span v-if="decided('R')" class="ml-2 has-text-grey">{{ decidedLine(decided('R')) }}</span>
        </div>
      </section>

      <b-collapse :open="false" class="review-section">
        <template #trigger="props">
          <h5 class="title is-5 is-clickable">
            <b-icon :icon="props.open ? 'chevron-down' : 'chevron-right'" size="is-small" />
            {{ $t('campaigns.review.sectionProvenance') }}
          </h5>
        </template>
        <ul class="is-size-7 provenance">
          <li>rubric {{ report.rubricVersion }} · job {{ report.jobId }} · bundle {{ report.bundleHash.slice(0, 12) }}</li>
          <li v-if="report.provenance">
fork {{ report.provenance.forkVersion }} · builder {{ (report.provenance.bundleSha256 || '').slice(0, 12) }}
            · SpamAssassin {{ report.provenance.saVersion }} rules {{ (report.provenance.saRulesSha256 || '').slice(0, 12) }}
</li>
          <li v-for="c in ((report.ai && report.ai.calls) || [])" :key="c.call">
            A-{{ c.call }}: {{ c.status }} · {{ c.modelId }} · prompt {{ (c.promptHash || '').slice(0, 12) }}
            · {{ c.tokensIn }}/{{ c.tokensOut }} tokens · {{ c.ms }} ms<span v-if="c.cachedFrom"> · cached from {{ c.cachedFrom }}</span>
          </li>
          <li v-for="(at, what) in ((report.provenance && report.provenance.lookups) || {})" :key="what">{{ what }} {{ at }}</li>
        </ul>
      </b-collapse>
    </template>

    <!-- Submit bar. -->
    <footer class="review-submit">
      <b-button v-if="stagedFixes.length" type="is-primary" :loading="busy" data-cy="btn-apply-fixes" @click="submit(true)">
        {{ $t('campaigns.review.applyFixes', { n: stagedFixes.length }) }}
      </b-button>
      <b-button v-else-if="Object.keys(staged).length" type="is-primary" :loading="busy" data-cy="btn-submit" @click="submit(false)">
        {{ $t('globals.buttons.save') }}
      </b-button>
      <b-button v-if="!isRunning" :loading="busy" data-cy="btn-reinspect" @click="reinspect">
        {{ $t('campaigns.review.reinspect') }}
      </b-button>
      <b-button v-if="isDone" type="is-success" data-cy="btn-done" @click="closeWindow">{{ $t('campaigns.review.done') }}</b-button>
    </footer>

    <!-- The builder is loaded into its own frame only when a visual fix needs a recompile. -->
    <iframe ref="builderFrame" class="builder-frame" title="builder" aria-hidden="true" />
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import { applyFixes, reviewPayload, dispositionsAfterFixes } from '../reviewFixes.mjs'; // eslint-disable-line import/extensions
import { deriveContext, isOfficialName } from '../officialSweep.mjs'; // eslint-disable-line import/extensions

const STAGES = ['reading', 'deterministic', 'screenshots', 'ai', 'writing'];
const BUILDER_CACHE_BUST = Date.now();

export default Vue.extend({
  data() {
    return {
      id: parseInt(this.$route.params.id, 10),
      state: null,
      campaign: {},
      // key -> { key, rubric_id, action, fix? } staged in this window, posted on submit.
      staged: {},
      busy: false,
      pollID: null,
    };
  },

  computed: {
    ...mapState(['lists']),

    // The newest row: the running / failed / stale banners read it.
    row() {
      return this.state && this.state.review;
    },

    // The GATE's row (Stage 4 finding 2): the newest COMPLETE review of the campaign's current
    // bundle hash, with the fork's verdict over it -- exactly what Start is judged by. A newer
    // failed/stale row never hides it. The report, verdict and items render from it when present.
    gate() {
      return (this.state && this.state.gate) || null;
    },

    report() {
      if (this.gate && this.gate.review) {
        return this.gate.review.report;
      }
      return this.row && this.row.status === 'complete' ? this.row.report : null;
    },

    isRunning() {
      return !!this.row && this.row.status === 'running';
    },

    isEdited() {
      return !this.gate && !!this.row && this.row.status === 'complete' && this.row.bundle_hash !== this.state.current_hash;
    },

    verdictSource() {
      if (this.gate) {
        return this.gate.verdict;
      }
      return (this.state && this.state.verdict) || null;
    },

    verdictName() {
      return (this.verdictSource && this.verdictSource.verdict) || 'none';
    },

    blockers() {
      return (this.verdictSource && this.verdictSource.blockers) || [];
    },

    lang() {
      return (this.campaign.attribs && this.campaign.attribs.lang) || '';
    },

    progressValue() {
      const st = this.row && this.row.progress && this.row.progress.stage;
      const i = STAGES.indexOf(st);
      return i < 0 ? 5 : Math.round(((i + 1) / (STAGES.length + 1)) * 100);
    },

    stageLabel() {
      const st = this.row && this.row.progress && this.row.progress.stage;
      return STAGES.includes(st) ? this.$t(`campaigns.review.stage.${st}`) : this.$t('campaigns.review.running');
    },

    // Latest disposition per key (the log is newest first).
    latest() {
      const out = {};
      ((this.state && this.state.dispositions) || []).forEach((d) => {
        if (!out[d.item_key]) {
          out[d.item_key] = d;
        }
      });
      return out;
    },

    entries() {
      if (!this.report) {
        return [];
      }
      const out = [];
      this.report.items.forEach((item) => {
        if (item.verdict === 'pass' || item.verdict === 'n/a') {
          return;
        }
        (item.findings || []).forEach((finding) => out.push({ key: finding.key, item, finding }));
      });
      return out;
    },

    sections() {
      const blockers = [];
      const ack = [];
      const advisory = [];
      this.entries.forEach((e) => {
        if (e.item.tier === 'D') {
          (e.item.verdict === 'fail' ? blockers : advisory).push(e);
        } else if (e.finding.severity === 'critical') {
          blockers.push(e);
        } else if (e.finding.severity === 'high') {
          ack.push(e);
        } else {
          advisory.push(e);
        }
      });
      return [
        { key: 'blockers', label: this.$t('campaigns.review.sectionBlockers'), entries: blockers },
        { key: 'acknowledge', label: this.$t('campaigns.review.sectionAcknowledge'), entries: ack },
        { key: 'advisory', label: this.$t('campaigns.review.sectionAdvisory'), entries: advisory },
      ].filter((s) => s.entries.length);
    },

    passed() {
      return this.report ? this.report.items.filter((i) => i.verdict === 'pass' || i.verdict === 'n/a') : [];
    },

    stagedFixes() {
      return Object.values(this.staged).filter((s) => s.action === 'fixed' && s.fix);
    },

    // Done when the verdict is pass, or every open entry has a decision (recorded or staged).
    isDone() {
      if (this.isRunning || !this.report) {
        return false;
      }
      if (this.verdictName === 'pass') {
        return true;
      }
      return this.entries.every((e) => this.staged[e.key] || this.latest[e.key]);
    },
  },

  methods: {
    load() {
      return this.$api.getCampaignReview(this.id, true).then((d) => {
        this.state = d;
        this.schedulePoll();
      }).catch(() => this.schedulePoll());
    },

    // 2 s while running, 5 s otherwise (D11).
    schedulePoll() {
      clearTimeout(this.pollID);
      this.pollID = setTimeout(this.load, this.isRunning ? 2000 : 5000);
    },

    loadCampaign() {
      return this.$api.getCampaignRaw(this.id).then((c) => {
        this.campaign = c;
      });
    },

    stage(key, rubricId, action, fix) {
      if (this.staged[key] && this.staged[key].action === action) {
        this.$delete(this.staged, key);
        return;
      }
      this.$set(this.staged, key, {
        key, rubric_id: rubricId, action, ...(fix ? { fix } : {}),
      });
    },

    stagedAction(key) {
      return this.staged[key] ? this.staged[key].action : null;
    },

    decided(key) {
      return this.latest[key] || null;
    },

    decidedAction(key) {
      return this.latest[key] ? this.latest[key].action : null;
    },

    decidedLine(d) {
      const label = { accept: this.$t('campaigns.review.acceptRisk'), fixme: this.$t('campaigns.review.illFixIt'), fixed: this.$t('campaigns.review.fixForMe') }[d.action] || d.action;
      return this.$t('campaigns.review.decidedBy', { action: label, user: d.username || '?', date: this.$utils.niceDate(d.created_at, true) });
    },

    // An "I'll fix it" item that is still in the report after a re-inspection.
    stillOpen(e) {
      const d = this.latest[e.key];
      return !!d && d.action === 'fixme' && !this.staged[e.key];
    },

    tagType(e) {
      const s = e.finding.severity || e.item.verdict;
      return {
        critical: 'is-danger', fail: 'is-danger', high: 'is-warning', warn: 'is-warning',
      }[s] || 'is-light';
    },

    // Only this listmonk host's own uploads render as a thumbnail (Stage 4 finding 17): evidence is
    // model- or author-supplied text, and an arbitrary URL in an <img src> would be fetched by the
    // browser. Same-origin, so no host is hard-coded.
    isImageUrl(s) {
      if (typeof s !== 'string') {
        return false;
      }
      const v = s.trim();
      return v.startsWith(`${window.location.origin}/uploads/`) && /^\S+\.(png|jpe?g|gif|webp)(\?\S*)?$/i.test(v);
    },

    // The builder UMD in this window's own frame, for compileDocument (the sweep's compile).
    loadBuilder() {
      const frame = this.$refs.builderFrame;
      const win = frame && frame.contentWindow;
      if (win && win.EmailBuilder) {
        return Promise.resolve(win.EmailBuilder);
      }
      return new Promise((resolve, reject) => {
        const script = frame.contentDocument.createElement('script');
        script.src = `/admin/static/email-builder/email-builder.umd.js?v=${BUILDER_CACHE_BUST}`;
        script.onload = () => (frame.contentWindow.EmailBuilder ? resolve(frame.contentWindow.EmailBuilder) : reject(new Error('no EmailBuilder')));
        script.onerror = reject;
        frame.contentDocument.head.appendChild(script);
      });
    },

    // Apply the staged fixes to the SAVED campaign, recompile a visual body, PUT it. Returns the fix
    // objects that were applied AND saved (empty when nothing could be).
    async applyStagedFixes() {
      const raw = await this.$api.getCampaignRaw(this.id);
      const {
        campaign, applied, skipped, recompile,
      } = applyFixes(raw, this.stagedFixes.map((s) => s.fix));
      if (skipped.length) {
        this.$utils.toast(this.$t('campaigns.review.fixesSkipped', { n: skipped.length }), 'is-warning');
      }
      if (!applied.length) {
        return [];
      }
      let { body } = campaign;
      if (recompile) {
        let em;
        try {
          em = await this.loadBuilder();
        } catch (e) {
          em = null;
        }
        if (!em || !em.compileDocument) {
          this.$utils.toast(this.$t('campaigns.review.builderUnavailable'), 'is-danger');
          return [];
        }
        const [tpls, lists] = await Promise.all([
          this.$api.getTemplatesRaw(),
          (this.lists && this.lists.results && this.lists.results.length) ? Promise.resolve(this.lists.results) : this.$api.getLists({ minimal: true, per_page: 'all' }).then((l) => l.results || l),
        ]);
        const refs = (Array.isArray(tpls) ? tpls : [])
          .filter((t) => t.type === 'campaign_visual' && isOfficialName(t.name))
          .map((t) => ({ id: t.id, name: t.name, body_source: t.body_source }));
        const ctx = deriveContext(campaign, lists || []);
        body = em.compileDocument(JSON.parse(campaign.body_source), { lang: ctx.lang, brand: ctx.brand }, refs);
      }
      await this.$api.updateCampaign(this.id, reviewPayload(campaign, body));
      this.$utils.toast(this.$t('campaigns.review.fixesApplied', { n: applied.length }));
      return applied;
    },

    // Stage 4 finding 6: apply the fixes FIRST, then record `fixed` only for the ones applied and
    // `fixme` for the ones that could not be, then re-inspect.
    async submit(withFixes) {
      this.busy = true;
      try {
        let applied = [];
        if (withFixes && this.stagedFixes.length) {
          applied = await this.applyStagedFixes();
        }
        const items = dispositionsAfterFixes(Object.values(this.staged), applied);
        if (items.length) {
          await this.$api.postReviewDispositions(this.id, items);
        }
        this.staged = {};
        if (applied.length > 0) {
          await this.$api.startCampaignReview(this.id);
        }
        await this.loadCampaign();
        await this.load();
      } finally {
        this.busy = false;
      }
    },

    async reinspect() {
      this.busy = true;
      try {
        await this.$api.startCampaignReview(this.id);
        await this.load();
      } catch (e) {
        await this.load();
      } finally {
        this.busy = false;
      }
    },

    closeWindow() {
      window.close();
    },
  },

  mounted() {
    this.loadCampaign();
    this.load();
  },

  beforeDestroy() {
    clearTimeout(this.pollID);
  },
});
</script>

<style scoped>
.campaign-review {
  max-width: 980px;
  margin: 0 auto;
  padding: 1.5rem 1.5rem 5rem;
}
.review-header {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: flex-start;
  margin-bottom: 1rem;
}
.verdict-banner {
  padding: 0.5rem 1rem;
  border-radius: 4px;
  font-weight: bold;
  background: #eee;
}
.verdict-banner.is-pass {
  background: #d9f2e1;
}
.verdict-banner.is-blocked {
  background: #fde2e2;
}
.review-section {
  margin: 1.25rem 0;
}
.review-item .todo {
  font-size: 1.05em;
  margin: 0.4rem 0;
}
.evidence-thumb {
  max-height: 80px;
  vertical-align: middle;
}
.structure-cmd {
  white-space: pre-wrap;
  font-size: 0.8em;
}
.review-submit {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 0.75rem 1.5rem;
  background: #fff;
  border-top: 1px solid #ddd;
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
}
.builder-frame {
  position: absolute;
  width: 0;
  height: 0;
  border: 0;
  visibility: hidden;
}
</style>
