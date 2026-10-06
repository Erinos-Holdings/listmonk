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

    <!-- integrations REVIEW-NAVIGATION-SPEC §4.6: the live checklist, pinned above the sections
         while the review runs and after it completes (reviewChecklist.mjs). -->
    <ul v-if="checklistRows.length" class="review-checklist" data-cy="review-checklist">
      <li v-for="r in checklistRows" :key="r.key" :class="`is-${r.state}`" :data-cy="`checklist-${r.key}`">
        <!-- A done row: the green check circle; otherwise an empty grey circle (circle before and after). -->
        <b-icon v-if="r.checked" icon="check-circle-outline" size="is-small" class="has-text-success" />
        <span v-else class="checklist-circle" aria-hidden="true" />
        <span>{{ r.label }}</span>
        <span v-if="r.spinner" class="checklist-spinner" aria-hidden="true" />
      </li>
    </ul>

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

    <!-- EARLY FINDINGS -->
    <!-- §4.6: the rule-check findings from progress.items while the review runs -- read-only (no
         buttons, nothing can be staged against them, I5). The report replaces them. -->
    <template v-if="isRunning && earlySections.length">
      <section v-for="sec in earlySections" :key="`early-${sec.key}`" class="review-section" :data-cy="`early-${sec.key}`">
        <h5 class="title is-5">{{ sec.label }} <span class="tag">{{ sec.entries.length }}</span></h5>
        <div v-for="e in sec.entries" :key="`early-${e.key}`" class="review-item box is-early" :data-cy="`early-item-${e.item.id}`">
          <div class="item-head">
            <strong>{{ e.item.id }}</strong> {{ e.item.failTitle || e.item.title }}
            <b-tag :type="tagType(e)" class="ml-2">{{ e.finding.severity || e.item.verdict }}</b-tag>
          </div>
          <p class="todo">{{ e.finding.todo }}</p>
          <div v-if="e.finding.proposal" class="proposal" data-cy="review-proposal">
            <p class="proposal-text">{{ e.finding.proposal }}</p>
            <b-button size="is-small" class="proposal-copy" data-cy="btn-copy-proposal" @click="copyText(e.finding.proposal)">
              {{ $t('campaigns.review.structureCopy') }}
            </b-button>
          </div>
          <p v-if="links(e.item).length" class="is-size-7 standards">
            <span class="has-text-grey">{{ $t('campaigns.review.standard') }}:</span>
            <span v-for="(r, n) in links(e.item)" :key="`l-${n}`"><a :href="r.url" target="_blank"
              rel="noopener noreferrer">{{ r.label }}</a><span v-if="n < links(e.item).length - 1">, </span></span>
          </p>
          <p v-if="reference(e.finding).kind === 'image'" class="where-line">
            <a href="#" class="where-thumb" data-cy="btn-goto-block" :aria-label="$t('campaigns.review.gotoBlock', { n: reference(e.finding).n })"
              @click.prevent="gotoBlock(reference(e.finding).blockId)"><img :src="reference(e.finding).image" alt="" class="evidence-thumb" /><span
                class="lm-structure-number">{{ reference(e.finding).n }}</span></a>
          </p>
          <p v-else-if="reference(e.finding).kind === 'badge'" class="is-size-7 where-line">
            <a href="#" class="where-badge" data-cy="btn-goto-block" :aria-label="$t('campaigns.review.gotoBlock', { n: reference(e.finding).n })"
              @click.prevent="gotoBlock(reference(e.finding).blockId)"><span class="lm-structure-number">{{ reference(e.finding).n }}</span></a>
            {{ reference(e.finding).text }}
          </p>
          <p v-else-if="reference(e.finding).kind === 'plain'" class="is-size-7">
            <span class="has-text-grey">{{ $t('campaigns.review.location') }}:</span> {{ reference(e.finding).text }}
          </p>
          <p class="is-size-7 has-text-grey">{{ $t('campaigns.review.decideLater') }}</p>
        </div>
      </section>
    </template>
    <!-- /EARLY FINDINGS -->

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
            <!-- The rule's pre-written failure title (report `failTitle`, user 2026-10-06); an older report has none. -->
            <strong>{{ e.item.id }}</strong> {{ e.item.failTitle || e.item.title }}
            <b-tag :type="tagType(e)" class="ml-2">{{ e.finding.severity || e.item.verdict }}</b-tag>
            <b-tag v-if="stillOpen(e)" type="is-warning" class="ml-1">{{ $t('campaigns.review.stillOpen') }}</b-tag>
          </div>
          <p class="todo">{{ e.finding.todo }}</p>
          <!-- Polish pass 2: the exact replacement text alone (report `proposal`), with Copy. -->
          <div v-if="e.finding.proposal" class="proposal" data-cy="review-proposal">
            <p class="proposal-text">{{ e.finding.proposal }}</p>
            <b-button size="is-small" class="proposal-copy" data-cy="btn-copy-proposal" @click="copyText(e.finding.proposal)">
              {{ $t('campaigns.review.structureCopy') }}
            </b-button>
          </div>
          <!-- REVIEW-NAVIGATION-SPEC §4.3: the standards links (https only). -->
          <p v-if="links(e.item).length" class="is-size-7 standards" data-cy="review-standards">
            <span class="has-text-grey">{{ $t('campaigns.review.standard') }}:</span>
            <span v-for="(r, n) in links(e.item)" :key="`l-${n}`"><a :href="r.url" target="_blank"
              rel="noopener noreferrer">{{ r.label }}</a><span v-if="n < links(e.item).length - 1">, </span></span>
          </p>
          <!-- The reference (reviewNavigate.mjs referenceLine): the thumbnail IS the click-through,
               with the editor's block-number badge on its corner; a badge alone when there is no
               image; plain Location text for an official footer or no `where`. -->
          <p v-if="reference(e.finding).kind === 'image'" class="where-line" data-cy="review-where">
            <a href="#" class="where-thumb" data-cy="btn-goto-block" :aria-label="$t('campaigns.review.gotoBlock', { n: reference(e.finding).n })"
              @click.prevent="gotoBlock(reference(e.finding).blockId)"><img :src="reference(e.finding).image" alt="" class="evidence-thumb" /><span
                class="lm-structure-number">{{ reference(e.finding).n }}</span></a>
          </p>
          <p v-else-if="reference(e.finding).kind === 'badge'" class="is-size-7 where-line" data-cy="review-where">
            <a href="#" class="where-badge" data-cy="btn-goto-block" :aria-label="$t('campaigns.review.gotoBlock', { n: reference(e.finding).n })"
              @click.prevent="gotoBlock(reference(e.finding).blockId)"><span class="lm-structure-number">{{ reference(e.finding).n }}</span></a>
            {{ reference(e.finding).text }}
          </p>
          <p v-else-if="reference(e.finding).kind === 'plain'" class="is-size-7" data-cy="review-where">
            <span class="has-text-grey">{{ $t('campaigns.review.location') }}:</span> {{ reference(e.finding).text }}
          </p>
          <p v-if="decided(e.key)" class="is-size-7 has-text-grey">{{ decidedLine(decided(e.key)) }}</p>
          <div class="buttons mt-2">
            <b-button size="is-small" :disabled="!e.finding.fix" data-cy="btn-fix-for-me"
              :type="fixType(e)" @click="stage(e.key, e.item.id, 'fixed', e.finding.fix)">
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

      <!-- Context, Structure and Provenance (reviewChecklist.mjs auxSections) after the finding
           sections: fail, then warn, then the ok group -- Passed first, then the ok sections. A
           section with a concern opens by default; the pill only draws the eye. -->
      <template v-for="s in auxLayout">
        <b-collapse v-if="s.key === 'passed'" :key="s.key" :open="false" class="review-section" data-cy="section-passed">
          <template #trigger="props">
            <h5 class="title is-5 is-clickable aux-head">
              <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
              {{ $t('campaigns.review.sectionPassed') }} <b-tag type="is-success">{{ passed.length }}</b-tag>
            </h5>
          </template>
          <ul class="passed-list is-size-7">
            <!-- An AI question (tier A) carries the Inspect button's AI glyph; a rule check (tier D) none. -->
            <li v-for="i in passed" :key="i.id">
              <b-icon v-if="i.tier === 'A'" icon="creation" size="is-small" class="passed-ai" data-cy="passed-ai" />
              <strong>{{ i.id }}</strong> {{ i.title }} —
              <template v-if="i.verdict === 'n/a'">n/a: {{ i.reason }}</template>
              <template v-else>{{ $t('campaigns.review.examined', { n: i.examined || 0 }) }}</template>
            </li>
          </ul>
        </b-collapse>

        <b-collapse v-else-if="s.key === 'context'" :key="s.key" :open="s.open" class="review-section" data-cy="section-context">
          <template #trigger="props">
            <h5 class="title is-5 is-clickable aux-head">
              <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
              {{ $t('campaigns.review.sectionContext') }} <b-tag :type="s.pillType" class="ml-2">{{ s.pill }}</b-tag>
            </h5>
          </template>
          <!-- Polish pass 2: the card font size; a concern line bold on a warning band (contrast: never
               yellow text). Under it, the campaign-context rule's (D7.1) instruction, the page that
               helps, and the three buttons for that finding -- Acknowledge is the only enabled one. -->
          <ul class="context-list" data-cy="context-list">
            <li v-for="c in contextLines" :key="c.index" class="context-line"
              :class="{ 'is-concern has-background-warning-light': s.highlight.has(c.index) || !!c.finding }">
              <strong>{{ c.label }}</strong>: {{ c.value }}
              <span v-if="c.asOf" class="has-text-grey">({{ c.asOf }})</span>
              <ul v-if="c.finding" class="context-advice" data-cy="context-advice">
                <li>
                  {{ c.finding.todo }}
                  <a v-if="c.href" :href="c.href" target="_blank" rel="noopener" class="ml-1" data-cy="context-open">
                    {{ $t('campaigns.review.contextOpen', { page: $t(`campaigns.review.contextPage.${c.page}`) }) }}</a>
                  <p v-if="decided(c.finding.key)" class="is-size-7 has-text-grey">{{ decidedLine(decided(c.finding.key)) }}</p>
                  <div class="buttons mt-1">
                    <b-button size="is-small" disabled data-cy="btn-fix-for-me">{{ $t('campaigns.review.fixForMe') }}</b-button>
                    <b-button size="is-small" disabled data-cy="btn-ill-fix-it">{{ $t('campaigns.review.illFixIt') }}</b-button>
                    <b-button size="is-small" data-cy="btn-accept" :type="stagedAction(c.finding.key) === 'accept' ? 'is-primary' : ''"
                      @click="stage(c.finding.key, 'D7.1', 'accept')">
                      {{ $t('campaigns.review.acknowledge') }}
                    </b-button>
                  </div>
                </li>
              </ul>
            </li>
          </ul>
        </b-collapse>

        <!-- INSPECT-SCOPE-SPEC §2.5: rendered from D4.2's coverage object; a report without it
             (STRUCTURE_GATE off, older reports) renders the legacy content. -->
        <b-collapse v-else-if="s.key === 'structure'" :key="s.key" :open="s.open" class="review-section" data-cy="section-structure">
          <template #trigger="props">
            <h5 class="title is-5 is-clickable aux-head">
              <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
              {{ $t('campaigns.review.sectionStructure') }} <b-tag :type="s.pillType" class="ml-2">{{ s.pill }}</b-tag>
            </h5>
          </template>
        <!-- Polish pass 2: one explanatory line at the card font size, then the smaller content. -->
        <p class="aux-intro" data-cy="structure-intro">{{ structureIntro }}</p>
        <div v-if="structure">
        <p v-if="structure.coverage.verified" class="is-size-7" data-cy="structure-verified">
          {{ $t('campaigns.review.structureVerifiedRecords', { records: recordsLine(structure.coverage.recordsUsed) }) }}
        </p>
        <div v-else class="is-size-7" data-cy="structure-unverified">
          <p class="structure-summary">
            <template v-if="!structure.coverage.plan.available">
              {{ $t('campaigns.review.structureUnavailable', { inputs: (structure.coverage.unavailable || []).join(', ') }) }}
            </template>
            <template v-else>
              {{ $t('campaigns.review.structureSummary', {
                n: structure.coverage.items.length,
                stage: structure.coverage.plan.next,
                count: structure.coverage.plan.count,
                total: structure.coverage.plan.total,
              }) }}
            </template>
          </p>

          <b-collapse :open="false" class="mt-2" data-cy="structure-detail">
            <template #trigger="props">
              <a class="is-clickable aux-head">
                <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
                {{ $t('campaigns.review.structureWhat') }}
              </a>
            </template>
            <b-table :data="structure.coverage.items" class="mt-2 structure-table" narrowed>
              <b-table-column v-slot="props" field="label" :label="$t('campaigns.review.structureItem')">
                {{ props.row.label }}
              </b-table-column>
              <b-table-column v-slot="props" field="reason" :label="$t('campaigns.review.structureReason')">
                {{ props.row.reason }}
              </b-table-column>
              <b-table-column v-slot="props" field="scope" :label="$t('campaigns.review.structureScope')">
                {{ props.row.scope === 'dark' ? $t('campaigns.review.structureScopeDark') : $t('campaigns.review.structureScopeFull') }}
              </b-table-column>
              <b-table-column v-slot="props" field="missing" :label="$t('campaigns.review.structureMissing')">
                {{ $t('campaigns.review.structureMissingStages', { s1: (props.row.missing['1'] || []).length, s2: (props.row.missing['2'] || []).length }) }}
              </b-table-column>
            </b-table>
            <div v-if="structure.coverage.plan.available" class="mt-2">
              <p>
                <strong>{{ $t('campaigns.review.structureStage1', { n: structure.coverage.plan.stages['1'].length }) }}</strong>
                {{ structure.coverage.plan.stages['1'].map(clientLabel).join(', ') || '—' }}
              </p>
              <p>
                <strong>{{ $t('campaigns.review.structureStage2', { n: structure.coverage.plan.stages['2'].length }) }}</strong>
              </p>
              <p>{{ $t('campaigns.review.structureCredits', { total: structure.coverage.plan.total }) }}</p>
            </div>
            <pre v-if="structureUi.showCli" class="structure-cmd" data-cy="structure-cli">{{ (structure.coverage.commands || []).join('\n') }}</pre>
          </b-collapse>

          <div class="buttons mt-2">
            <b-tooltip :label="structureUi.notify === 'claude' ? $t('campaigns.review.notifyClaudeTip') : $t('campaigns.review.notifyRobbieTip')"
              type="is-dark" multilined>
              <b-button size="is-small" data-cy="btn-structure-notify" @click="openStructureBrief">
                {{ structureUi.notify === 'claude' ? $t('campaigns.review.notifyClaude') : $t('campaigns.review.notifyRobbie') }}
              </b-button>
            </b-tooltip>
            <b-tooltip v-if="structureUi.acknowledge && structure.finding" :label="$t('campaigns.review.structureAcknowledgeTip')"
              type="is-dark" multilined>
              <b-button size="is-small" data-cy="btn-structure-acknowledge"
                :type="stagedAction(structure.finding.key) === 'accept' ? 'is-primary' : ''"
                @click="stage(structure.finding.key, 'D4.2', 'accept')">
                {{ $t('campaigns.review.acknowledge') }}
              </b-button>
            </b-tooltip>
            <span v-if="structure.finding && decided(structure.finding.key)" class="ml-2 has-text-grey">
              {{ decidedLine(decided(structure.finding.key)) }}
            </span>
          </div>
        </div>
        </div>

        <div v-else-if="report.structure">
        <p v-if="report.structure.verified" class="is-size-7">
          {{ $t('campaigns.review.structureVerified', { date: report.structure.verified.verified_at.slice(0, 10), id: report.structure.verified.test_id }) }}
        </p>
        <div v-else class="is-size-7">
          <p>{{ $t('campaigns.review.structureOffer', { differs: (report.structure.differs || []).join(', ') }) }}</p>
          <pre class="structure-cmd">{{ report.structure.command }}</pre>
        </div>
        </div>
        </b-collapse>

        <b-collapse v-else-if="s.key === 'provenance'" :key="s.key" :open="s.open" class="review-section" data-cy="section-provenance">
          <template #trigger="props">
            <h5 class="title is-5 is-clickable aux-head">
              <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
              {{ $t('campaigns.review.sectionProvenance') }} <b-tag :type="s.pillType" class="ml-2">{{ s.pill }}</b-tag>
            </h5>
          </template>
          <p class="aux-intro" data-cy="provenance-intro">{{ $t('campaigns.review.provenanceIntro') }}</p>
          <ul class="is-size-7 provenance">
            <li>rubric {{ report.rubricVersion }} · job {{ report.jobId }} · bundle {{ report.bundleHash.slice(0, 12) }}</li>
            <li v-if="report.provenance">
              fork {{ report.provenance.forkVersion }} · builder {{ (report.provenance.bundleSha256 || '').slice(0, 12) }}
              · SpamAssassin {{ report.provenance.saVersion }} rules {{ (report.provenance.saRulesSha256 || '').slice(0, 12) }}
            </li>
            <li v-for="(c, n) in ((report.ai && report.ai.calls) || [])" :key="c.call"
              :class="{ 'has-text-weight-bold has-background-warning-light': s.highlight.has(n) }">
              A-{{ c.call }}: {{ c.status }} · {{ c.modelId }} · prompt {{ (c.promptHash || '').slice(0, 12) }}
              · {{ c.tokensIn }}/{{ c.tokensOut }} tokens · {{ c.ms }} ms<span v-if="c.cachedFrom"> · cached from {{ c.cachedFrom }}</span>
            </li>
            <li v-for="(at, what) in ((report.provenance && report.provenance.lookups) || {})" :key="what">{{ what }} {{ at }}</li>
          </ul>
        </b-collapse>
      </template>

      <b-modal scroll="keep" :aria-modal="true" :active.sync="briefOpen" :width="820" data-cy="structure-brief-modal">
        <div class="modal-card" style="width: auto">
          <header class="modal-card-head">
            <p class="modal-card-title">{{ $t('campaigns.review.structureBriefTitle') }}</p>
          </header>
          <section class="modal-card-body">
            <p class="is-size-7 mb-2">
              {{ structureUi.notify === 'claude' ? $t('campaigns.review.notifyClaudeTip') : $t('campaigns.review.notifyRobbieTip') }}
            </p>
            <label for="structure-brief" class="is-sr-only">{{ $t('campaigns.review.structureBriefTitle') }}</label>
            <textarea id="structure-brief" ref="briefText" class="textarea structure-brief" readonly :value="brief" rows="18"
              @focus="$event.target.select()" />
          </section>
          <footer class="modal-card-foot">
            <b-button type="is-primary" data-cy="btn-structure-copy" @click="copyBrief">{{ $t('campaigns.review.structureCopy') }}</b-button>
            <b-button @click="briefOpen = false">{{ $t('globals.buttons.close') }}</b-button>
          </footer>
        </div>
      </b-modal>
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
import {
  buildStructureBrief, isStructureKey, structureButtons, structureOf, STRUCTURE_PERMISSION,
} from '../structureBrief.mjs'; // eslint-disable-line import/extensions
import {
  auxSections, checklist, earlyEntries, isContextItem, sectionCounts, sectionOf,
} from '../reviewChecklist.mjs'; // eslint-disable-line import/extensions
import { contextKeys, contextRows } from '../contextAdvice.mjs'; // eslint-disable-line import/extensions
import {
  ACK_TIMEOUT_MS, RESULT_TIMEOUT_MS, acceptAck, acceptOpener, afterAckWait, clickDecision, fallbackUrl,
  isFinalResult, newToken, referenceLine, resultToast, selectMessage, standardLinks,
} from '../reviewNavigate.mjs'; // eslint-disable-line import/extensions

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
      briefOpen: false,
      brief: '',
    };
  },

  created() {
    // REVIEW-NAVIGATION-SPEC §4.5: the last admin window that clicked Inspect (its
    // `lm-review:opener` handshake), else window.opener; and the one click in flight. Kept off
    // the reactive data on purpose: they hold other windows.
    this.currentOpener = null;
    this.nav = null;
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

    // REVIEW-NAVIGATION-SPEC §4.6: running (the newest row), complete (the report shown), or
    // halted (failed/stale: the checklist stays as at the last PATCH).
    checklistPhase() {
      if (this.isRunning) {
        return 'running';
      }
      if (this.row && (this.row.status === 'failed' || this.row.status === 'stale')) {
        return 'halted';
      }
      return this.report ? 'complete' : null;
    },

    // The progress the checklist reads: the running/halted row's, or the shown report's own row's.
    checklistProgress() {
      if (this.checklistPhase === 'complete') {
        const g = this.gate && this.gate.review;
        if (g && g.progress) {
          return g.progress;
        }
        return this.row && this.row.status === 'complete' ? (this.row.progress || null) : null;
      }
      return (this.row && this.row.progress) || null;
    },

    checklistRows() {
      if (!this.checklistPhase) {
        return [];
      }
      return checklist({
        progress: this.checklistProgress,
        report: this.report,
        phase: this.checklistPhase,
        sectionsOf: sectionCounts,
        t: (k, p) => this.$t(k, p),
      });
    },

    // §4.6: the D items of a running review, read-only, in the same sections.
    earlySections() {
      const early = earlyEntries({ progress: this.row && this.row.progress, phase: this.isRunning ? 'running' : 'done' });
      return this.sectionList(early);
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
        // Polish pass 2: the campaign-context rule (D7.1) is decided inside the Context section.
        if (item.verdict === 'pass' || item.verdict === 'n/a' || isContextItem(item)) {
          return;
        }
        // INSPECT-SCOPE-SPEC §2.5: the structure key (R#…) is decided ONLY in the Structure
        // section (Acknowledge, structure admins) -- never in the generic lists and their actions.
        (item.findings || []).filter((f) => !isStructureKey(f.key)).forEach((finding) => out.push({ key: finding.key, item, finding }));
      });
      return out;
    },

    // The section rule is reviewChecklist.mjs `sectionOf` -- the checklist's counts use the same one.
    sections() {
      return this.sectionList(this.entries);
    },

    passed() {
      return this.report ? this.report.items.filter((i) => i.verdict === 'pass' || i.verdict === 'n/a') : [];
    },

    // D4.2's coverage (INSPECT-SCOPE-SPEC §2.5), or null → the legacy Structure section.
    structure() {
      return structureOf(this.report);
    },

    // Polish pass 2: the Context lines with D7.1's finding per concern line and the page that helps
    // (contextAdvice.mjs).
    contextLines() {
      return contextRows(this.report, this.$router.options.base || '');
    },

    // The Structure section's explanatory line: with the plan's total when the coverage has one.
    structureIntro() {
      const plan = this.structure && this.structure.coverage && this.structure.coverage.plan;
      const total = plan && Number.isInteger(plan.total) && plan.total > 0 ? plan.total : null;
      return total !== null
        ? this.$t('campaigns.review.structureIntro', { total })
        : this.$t('campaigns.review.structureIntroNoTotal');
    },

    // Context, Structure, Provenance with their pills (reviewChecklist.mjs auxSections), ordered
    // fail, warn, ok -- with Passed placed first in the ok group.
    auxLayout() {
      const aux = auxSections({ report: this.report, structure: this.structure, t: (k, p) => this.$t(k, p) });
      const notOk = aux.filter((s) => s.status !== 'ok');
      const ok = aux.filter((s) => s.status === 'ok');
      return [...notOk, { key: 'passed' }, ...ok];
    },

    // Buttons by the SERVER profile's permission (never a role name).
    structureUi() {
      return structureButtons(this.$can(STRUCTURE_PERMISSION));
    },

    stagedFixes() {
      return Object.values(this.staged).filter((s) => s.action === 'fixed' && s.fix);
    },

    // Done when the verdict is pass, or nothing is left that THIS user can decide here: every open
    // entry has a decision (recorded or staged), and -- for a structure admin -- so does D4.2's
    // structure finding (excluded from `entries`; its Acknowledge lives in the Structure section).
    // A user without the permission cannot act on it, so it never holds their Done hostage.
    isDone() {
      if (this.isRunning || !this.report) {
        return false;
      }
      if (this.verdictName === 'pass') {
        return true;
      }
      const decided = (key) => !!(this.staged[key] || this.latest[key]);
      if (this.structureUi.acknowledge && this.structure && this.structure.finding
        && !decided(this.structure.finding.key)) {
        return false;
      }
      // Polish pass 2: D7.1's findings (decided in the Context section) count like any entry.
      return this.entries.every((e) => decided(e.key)) && contextKeys(this.report).every(decided);
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

    // An "I fixed it" item (action `fixme`) that is still in the report after a re-inspection.
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

    // Entries -> the three sections (reviewChecklist.mjs `sectionOf`), empty sections dropped.
    sectionList(entries) {
      const by = { blockers: [], acknowledge: [], advisory: [] };
      entries.forEach((e) => by[sectionOf(e.item, e.finding)].push(e));
      return [
        { key: 'blockers', label: this.$t('campaigns.review.sectionBlockers'), entries: by.blockers },
        { key: 'acknowledge', label: this.$t('campaigns.review.sectionAcknowledge'), entries: by.acknowledge },
        { key: 'advisory', label: this.$t('campaigns.review.sectionAdvisory'), entries: by.advisory },
      ].filter((s) => s.entries.length);
    },

    // REVIEW-NAVIGATION-SPEC §4.1-§4.3 (reviewNavigate.mjs): the reference (the thumbnail or badge
    // that clicks through -- only this host's uploads render as an image, I10) and the standards
    // links. The window never re-derives a name.
    reference(finding) {
      return referenceLine(finding, window.location.origin);
    },

    links(item) {
      return standardLinks(item);
    },

    // §4.5: the window's opener is the last admin window that clicked Inspect, else window.opener.
    openerWindow() {
      return this.currentOpener || window.opener || null;
    },

    // §4.5 click-through: post to a reachable opener and wait 1,500 ms for `received`; otherwise
    // open the campaign's content page in a new tab, selecting on load.
    gotoBlock(blockId) {
      const opener = this.openerWindow();
      const { origin } = window.location;
      const decision = clickDecision({
        opener, ourOrigin: origin, campaignId: this.id, blockId,
      });
      if (decision === 'ignore') {
        return;
      }
      this.clearNav();
      if (decision === 'newTab') {
        this.openFallback(blockId);
        return;
      }
      const token = newToken();
      const nav = {
        token, target: opener, blockId, received: false, ackTimer: null, resultTimer: null,
      };
      this.nav = nav;
      try {
        opener.postMessage(selectMessage(this.id, blockId, token), origin);
      } catch (e) {
        this.clearNav();
        this.openFallback(blockId);
        return;
      }
      nav.ackTimer = setTimeout(() => {
        if (this.nav === nav && afterAckWait(nav.received ? 'received' : null) === 'newTab') {
          this.clearNav();
          this.openFallback(blockId);
        }
      }, ACK_TIMEOUT_MS);
    },

    openFallback(blockId) {
      const url = fallbackUrl(this.$router.options.base || '', this.id, blockId);
      if (url) {
        window.open(url, '_blank');
      }
    },

    clearNav() {
      if (this.nav) {
        clearTimeout(this.nav.ackTimer);
        clearTimeout(this.nav.resultTimer);
      }
      this.nav = null;
    },

    finishNav(result) {
      const key = resultToast(result);
      this.clearNav();
      if (key) {
        this.$utils.toast(this.$t(key), result === 'selected' ? 'is-success' : 'is-warning');
      }
    },

    // The opener handshake and the acks of the click in flight; anything else is ignored.
    onMessage(ev) {
      const { origin } = window.location;
      if (acceptOpener({
        origin: ev.origin, ourOrigin: origin, source: ev.source, self: window, data: ev.data, campaignId: this.id,
      })) {
        this.currentOpener = ev.source;
        return;
      }
      const { nav } = this;
      if (!nav) {
        return;
      }
      const result = acceptAck({
        origin: ev.origin, ourOrigin: origin, source: ev.source, expectedSource: nav.target, data: ev.data, token: nav.token,
      });
      if (!result) {
        return;
      }
      if (result === 'received') {
        if (!nav.received) {
          nav.received = true;
          clearTimeout(nav.ackTimer);
          // The opener answers within RESULT_TIMEOUT_MS (20 s; its own cap answers `unknown`); this
          // window's cap is 1 s longer (21 s) so that answer arrives first. A reply after it is ignored.
          nav.resultTimer = setTimeout(() => {
            if (this.nav === nav) {
              this.finishNav('unknown');
            }
          }, RESULT_TIMEOUT_MS + 1000);
        }
        return;
      }
      if (nav.received && isFinalResult(result)) {
        this.finishNav(result);
      }
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

    recordsLine(records) {
      return (records || []).map((r) => `#${r.id} (${String(r.verified_at).slice(0, 10)}, test ${r.test_id})`).join('; ') || '—';
    },

    clientLabel(id) {
      const labels = (this.structure && this.structure.coverage.clientLabels) || {};
      return labels[id] || id;
    },

    // Both Notify buttons (S8): open the SAME modal with the brief. Nothing is sent.
    openStructureBrief() {
      this.brief = buildStructureBrief({
        campaign: { id: this.id, name: this.campaign.name },
        review: this.gate ? this.gate.review : this.row,
        report: this.report,
        origin: window.location.origin,
      });
      this.briefOpen = true;
    },

    copyBrief() {
      const done = () => this.$utils.toast(this.$t('campaigns.review.structureCopied'));
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(this.brief).then(done).catch(() => this.selectBrief());
        return;
      }
      this.selectBrief();
    },

    // Polish pass 2: a proposal's Copy -- the clipboard, else the execCommand fallback through a
    // throwaway textarea (the brief modal's fallback, without its modal).
    copyText(text) {
      const done = () => this.$utils.toast(this.$t('campaigns.review.structureCopied'));
      const fallback = () => {
        const el = document.createElement('textarea');
        el.value = text;
        el.setAttribute('readonly', '');
        el.style.position = 'absolute';
        el.style.left = '-9999px';
        document.body.appendChild(el);
        el.select();
        const ok = document.execCommand('copy');
        document.body.removeChild(el);
        if (ok) done();
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(fallback);
        return;
      }
      fallback();
    },

    // Polish pass 2: Fix for me is light green when a fix exists, green when staged, grey (disabled) without one.
    fixType(e) {
      if (this.stagedAction(e.key) === 'fixed') return 'is-success';
      return e.finding.fix ? 'is-success is-light' : '';
    },

    selectBrief() {
      const el = this.$refs.briefText;
      if (el) {
        el.focus();
        el.select();
        document.execCommand('copy');
      }
    },

    closeWindow() {
      window.close();
    },
  },

  mounted() {
    this.loadCampaign();
    this.load();
    window.addEventListener('message', this.onMessage);
  },

  beforeDestroy() {
    clearTimeout(this.pollID);
    window.removeEventListener('message', this.onMessage);
    this.clearNav();
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
/* REVIEW-NAVIGATION-SPEC §4.6: the live checklist, pinned at the top. */
.review-checklist {
  position: sticky;
  top: 0;
  z-index: 5;
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 4px;
  padding: 0.5rem 0.75rem;
  margin-bottom: 1rem;
  font-size: 0.9em;
}
.review-checklist li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.review-checklist li.is-pending {
  color: #888;
}
.review-checklist li.is-current {
  font-weight: bold;
}
.checklist-spinner {
  display: inline-block;
  width: 0.8em;
  height: 0.8em;
  border: 2px solid #ccc;
  border-top-color: #3273dc;
  border-radius: 50%;
  animation: checklist-spin 0.8s linear infinite;
}
@keyframes checklist-spin {
  to { transform: rotate(360deg); }
}
.review-item.is-early {
  opacity: 0.9;
}
/* Polish pass 2 (2026-10-06): a pending/current checklist row's empty grey circle. */
.checklist-circle {
  display: inline-block;
  box-sizing: border-box;
  width: 1rem;
  height: 1rem;
  border: 1px solid #b5b5b5; /* $grey-light */
  border-radius: 50%;
  flex: none;
}
/* The proposal box: the exact replacement text, with Copy at its top-right. */
.proposal {
  position: relative;
  background: #effaf3;
  border: 1px solid #b8e0c4;
  border-radius: 4px;
  padding: 0.5rem 4.5rem 0.5rem 0.75rem;
  margin: 0.4rem 0;
}
.proposal-text {
  white-space: pre-wrap;
  font-size: 1.05em;
  margin: 0;
}
.proposal-copy {
  position: absolute;
  top: 0.35rem;
  right: 0.35rem;
}
/* The Context section: concern lines bold with a warning band (never yellow text — contrast). */
.context-list {
  margin: 0.25rem 0;
}
.context-line {
  padding: 0.15rem 0.5rem;
  border-left: 3px solid transparent;
}
.context-line.is-concern {
  font-weight: bold;
  border-left-color: #ffe08a;
}
.context-advice {
  font-weight: normal;
  margin: 0.25rem 0 0.4rem 1.25rem;
  list-style: disc;
}
.aux-intro {
  margin-bottom: 1rem;
}
.passed-ai {
  vertical-align: middle;
  margin-right: 0.15rem;
}
/* The lists page's +/- expand glyph (style.scss scopes it to section.lists, so copied here). */
.aux-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}
.expand-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.15rem;
  height: 1.15rem;
  border: 1px solid #b5b5b5; /* $grey-light */
  border-radius: 3px;
  background: none;
  padding: 0;
  cursor: pointer;
  color: #7a7a7a; /* $grey */
  line-height: 1;
  vertical-align: middle;
  flex: none;
}
.expand-btn:hover {
  border-color: #0055d4; /* $primary */
  color: #0055d4;
}
/* The glyph inside the button is styled in the UNSCOPED block at the end of this file: the <i>
   belongs to the b-icon child component, which scoped CSS cannot reach. */
/* The reference: the thumbnail is the click-through, the editor's block-number badge on its
   top-right corner at the badge's FIXED size (EditorBlockWrapper.tsx) -- never scaled. */
.where-line {
  margin: 0.4rem 0;
}
.where-thumb {
  position: relative;
  display: inline-block;
  line-height: 0;
}
.where-thumb .evidence-thumb {
  display: block;
}
.where-thumb .lm-structure-number {
  position: absolute;
  top: 0;
  right: 0;
}
.where-badge {
  display: inline-block;
  vertical-align: middle;
  margin-right: 0.3rem;
}
.lm-structure-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  height: 18px;
  min-width: 18px;
  padding: 0 3px;
  font: 600 11px/1 sans-serif;
  border: 1px solid rgba(0, 121, 204, 1);
  border-radius: 0 0 0 4px;
  background: #ffffff;
  color: rgba(0, 121, 204, 1);
}
.structure-cmd {
  white-space: pre-wrap;
  font-size: 0.8em;
}
.structure-brief {
  font-family: monospace;
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

<!-- Unscoped on purpose (keyed on the page's root class): the +/- glyph is the <i> inside the
     b-icon CHILD component, which scoped CSS cannot reach. The icon box fills the button and the
     fontello pseudo-element's side margins and line-height are stripped, so the glyph is centred
     (it otherwise sat top-left). Same look as the lists page's global .expand-btn rules. -->
<style>
.campaign-review .expand-btn .icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  margin: 0;
  font-size: 0.7rem;
  line-height: 1;
}
.campaign-review .expand-btn .icon i {
  display: block;
  margin: 0;
  line-height: 1;
}
.campaign-review .expand-btn .icon i::before {
  margin: 0;
  width: auto;
  line-height: 1;
}
</style>
