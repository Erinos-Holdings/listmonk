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
      <div class="verdict-banner" :class="`is-${runStarting ? 'none' : verdictName}`" data-cy="review-verdict">
        <template v-if="isRunning || runStarting">{{ $t('campaigns.review.running') }}</template>
        <template v-else-if="verdictName === 'pass'">{{ $t('campaigns.review.verdictPass') }}</template>
        <template v-else-if="verdictName === 'blocked'">
          {{ $t('campaigns.review.verdictBlocked', { n: blockers.length }) }}
        </template>
        <template v-else>{{ $t('campaigns.review.verdictNone') }}</template>
      </div>
    </header>

    <!-- integrations REVIEW-NAVIGATION-SPEC §4.6: the live checklist, pinned above the sections
         while the review runs and after it completes (reviewChecklist.mjs). ALT-AUTOGEN-SPEC A1.1:
         its FIRST row is the alt step (altsStep) -- the generation the campaign page runs before the
         inspection; before the job exists it is the only row. -->
    <ul v-if="checklistRows.length" class="review-checklist" data-cy="review-checklist">
      <li v-for="r in checklistRows" :key="r.key" :class="`is-${r.state}`" :data-cy="`checklist-${r.key}`">
        <!-- A done row: the green check circle; the alt step's warning state: the warning sign;
             otherwise an empty grey circle (circle before and after). -->
        <b-icon v-if="r.checked" icon="check-circle-outline" size="is-small" class="has-text-success" />
        <b-icon v-else-if="r.state === 'warn'" icon="warning-empty" size="is-small" class="has-text-warning-dark" />
        <span v-else class="checklist-circle" aria-hidden="true" />
        <span>{{ r.label }}</span>
        <span v-if="r.spinner" class="checklist-spinner" aria-hidden="true" />
      </li>
    </ul>

    <!-- integrations REVIEW-TRIM-2-SPEC D4: a run that started no job ends on the latest report with
         this line (reviewNavigate.mjs runView). -->
    <b-message v-if="runLine" type="is-warning" data-cy="review-run-line">{{ $t(runLine) }}</b-message>

    <!-- The starting state shows no banner of the previous job (runStarting). -->
    <b-message v-if="!runStarting && row && row.status === 'failed'" type="is-danger" data-cy="review-failed">
      {{ $t('campaigns.review.failed') }}
      <b-button size="is-small" class="ml-2" @click="reinspect" :loading="busy">{{ $t('campaigns.review.retry') }}</b-button>
    </b-message>
    <b-message v-else-if="!runStarting && row && row.status === 'stale'" type="is-warning">
      {{ $t('campaigns.review.stale') }}
      <b-button size="is-small" class="ml-2" @click="reinspect" :loading="busy">{{ $t('campaigns.review.reinspect') }}</b-button>
    </b-message>
    <b-message v-else-if="!runStarting && isEdited" type="is-warning" data-cy="review-edited">
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
            <button type="button" class="proposal-copy" data-cy="btn-copy-proposal" :aria-label="$t('campaigns.review.copyProposal')"
              :title="$t('campaigns.review.copyProposal')" @click="copyText(e.finding.proposal)">
              <b-icon icon="file-multiple-outline" size="is-small" />
            </button>
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

    <template v-if="report && !isRunning && !runStarting">
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
            <button type="button" class="proposal-copy" data-cy="btn-copy-proposal" :aria-label="$t('campaigns.review.copyProposal')"
              :title="$t('campaigns.review.copyProposal')" @click="copyText(e.finding.proposal)">
              <b-icon icon="file-multiple-outline" size="is-small" />
            </button>
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
            <!-- integrations REVIEW-TRIM-2-SPEC D9: a finding with nothing to replace keeps the button
                 disabled; its tooltip says why in one line (the planner's reason, else the todo). -->
            <b-tooltip :active="!e.finding.fix" :label="fixTooltip(e)" type="is-dark" multilined>
              <b-button size="is-small" :disabled="!e.finding.fix" data-cy="btn-fix-for-me"
                :type="fixType(e)" @click="stage(e.key, e.item.id, 'fixed', e.finding.fix)">
                {{ $t('campaigns.review.fixForMe') }}
              </b-button>
            </b-tooltip>
            <b-button size="is-small" data-cy="btn-ill-fix-it" :type="stagedAction(e.key) === 'fixme' ? 'is-primary' : ''"
              @click="stage(e.key, e.item.id, 'fixme')">
              {{ $t('campaigns.review.illFixIt') }}
            </b-button>
            <b-tooltip v-if="acceptStateOf(e.item) === 'cannot'" :label="$t('campaigns.review.cannotAccept')" type="is-dark">
              <b-button size="is-small" disabled>{{ $t('campaigns.review.acceptRisk') }}</b-button>
            </b-tooltip>
            <!-- REVIEW-TRIM-SPEC D7: an item naming an override permission the user lacks (D2.15) is
                 never staged here; the server refuses it with 403 anyway. -->
            <b-tooltip v-else-if="acceptStateOf(e.item) === 'override'" :label="$t('campaigns.review.overrideAdminOnly')" type="is-dark" multilined>
              <b-button size="is-small" disabled data-cy="btn-accept-override">{{ $t('campaigns.review.acceptRisk') }}</b-button>
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
        <!-- ALT-AUTOGEN-SPEC A1.2: the "Alt tags" section, built like Passed and placed directly before
             it (reviewChecklist.mjs auxLayout / altsSection): Approved, Modified (old → new), Generated. -->
        <b-collapse v-if="s.key === 'alts'" :key="s.key" :open="false" class="review-section" data-cy="section-alts">
          <template #trigger="props">
            <h5 class="title is-5 is-clickable aux-head">
              <button type="button" class="expand-btn" :aria-expanded="props.open ? 'true' : 'false'"><b-icon :icon="props.open ? 'minus' : 'plus'" size="is-small" /></button>
              {{ $t('campaigns.review.sectionAltTags') }} <b-tag type="is-success">{{ altTags.total }}</b-tag>
            </h5>
          </template>
          <div v-for="g in altTags.groups" :key="g.key" class="alt-group" :data-cy="`alts-${g.key}`">
            <p class="alt-group-head">{{ $t(`campaigns.review.altTags.${g.key}`) }} <b-tag type="is-success" size="is-small">{{ g.count }}</b-tag></p>
            <ul class="alt-tags-list is-size-7">
              <li v-for="a in g.rows" :key="a.key">
                <strong>{{ a.file }}</strong> —
                <template v-if="g.key === 'modified'">{{ a.from }} → {{ a.alt }}</template>
                <template v-else>{{ a.alt }}</template>
              </li>
            </ul>
          </div>
        </b-collapse>

        <b-collapse v-else-if="s.key === 'passed'" :key="s.key" :open="false" class="review-section" data-cy="section-passed">
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
               yellow text). REVIEW-TRIM-SPEC D3: under a line, the Lambda's advice `note` in plain
               text (no buttons -- context is never decided) and, on a concern line, the page that helps. -->
          <ul class="context-list" data-cy="context-list">
            <li v-for="c in contextLines" :key="c.index" class="context-line"
              :class="{ 'is-concern has-background-warning-light': s.highlight.has(c.index) }">
              <strong>{{ c.label }}</strong>: {{ c.value }}
              <span v-if="c.asOf" class="has-text-grey">({{ c.asOf }})</span>
              <a v-if="c.concern && c.href" :href="c.href" target="_blank" rel="noopener" class="ml-1" data-cy="context-open">
                {{ $t('campaigns.review.contextOpen', { page: $t(`campaigns.review.contextPage.${c.page}`) }) }}</a>
              <p v-if="c.note" class="context-note" data-cy="context-note">{{ c.note }}</p>
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
                {{ $t(structureAckLabel) }}
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

    <!-- Submit bar. integrations REVIEW-TRIM-2-SPEC Stage 4 F4: none in the starting state (a
         Re-inspect there would start a job before the Inspect click's alt-generation save). -->
    <footer v-if="!runStarting" class="review-submit">
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
import {
  applyFixes, campaignDigests, compileCampaignBody, dispositionsAfterFixes, loadFrameBuilder, reviewPayload,
} from '../reviewFixes.mjs'; // eslint-disable-line import/extensions
import { acceptAlts, altsRowState } from '../altGenerate.mjs'; // eslint-disable-line import/extensions
import {
  buildStructureBrief, isStructureKey, structureButtons, structureOf, STRUCTURE_PERMISSION,
} from '../structureBrief.mjs'; // eslint-disable-line import/extensions
import {
  acceptState, altsSection, altsStep, auxLayout, auxSections, checklist, earlyEntries, isReviewDone, sectionCounts, sectionOf,
  structureAcknowledgeLabel, withAltsStep,
} from '../reviewChecklist.mjs'; // eslint-disable-line import/extensions
import { contextRows } from '../contextAdvice.mjs'; // eslint-disable-line import/extensions
import {
  ACK_TIMEOUT_MS, RESULT_TIMEOUT_MS, RUN_FALLBACK_MS, acceptAck, acceptOpener, acceptRun, afterAckWait, clickDecision, fallbackUrl,
  fallbackWindowName, isFinalResult, newToken, referenceLine, resultToast, runFromQuery, runView, selectMessage, standardLinks,
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
      // integrations ALT-AUTOGEN-SPEC A1.1: the live generation state (altGenerate.mjs altsRowState)
      // that drives the checklist's alt step (reviewChecklist.mjs altsStep).
      altsRow: null,
      // integrations REVIEW-TRIM-2-SPEC D4 (reviewNavigate.mjs runView): the Inspect click's `?run=`
      // token (null: not opened by Inspect), the latest job id at the first load (undefined until
      // then), the run's `failed` message, and the time since mount once the 90 s bound passes.
      run: runFromQuery(this.$route.query),
      runBaseJob: undefined,
      runFailed: false,
      runElapsed: 0,
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

    // REVIEW-TRIM-2-SPEC D4 / I12: the run's view -- `starting` (no report, no banners), `latest`, or
    // `failed` / `timeout` (the latest report with `line` above it).
    runState() {
      return runView({
        run: this.run, baseJob: this.runBaseJob, row: this.row, failed: this.runFailed, elapsedMs: this.runElapsed,
      });
    },

    runStarting() {
      return this.runState.mode === 'starting';
    },

    runLine() {
      return this.runState.line || null;
    },

    // ALT-AUTOGEN-SPEC A1.1: the checklist's first row (informational; a missed message hides nothing).
    // REVIEW-TRIM-2-SPEC D4: in the starting state only the live run drives it (never the old report).
    altsStepRow() {
      return altsStep({
        live: this.altsRow,
        report: this.runStarting ? null : this.report,
        reportJob: this.runStarting || !this.report ? null : this.report.jobId,
        running: this.runStarting ? false : this.isRunning,
        t: (k, p) => this.$t(k, p),
      });
    },

    checklistRows() {
      // REVIEW-TRIM-2-SPEC D4: the starting state -- "Starting inspection" first, then the live alts row.
      if (this.runStarting) {
        const starting = {
          key: 'starting', state: 'current', checked: false, spinner: true, label: this.$t('campaigns.review.checklist.starting'),
        };
        return [starting, ...(this.altsStepRow ? [this.altsStepRow] : [])];
      }
      const rows = this.checklistPhase ? checklist({
        progress: this.checklistProgress,
        report: this.report,
        phase: this.checklistPhase,
        sectionsOf: sectionCounts,
        t: (k, p) => this.$t(k, p),
      }) : [];
      return withAltsStep(this.altsStepRow, rows);
    },

    // ALT-AUTOGEN-SPEC A1.2: the "Alt tags" section (null without report.alts or images in scope).
    altTags() {
      return altsSection(this.report, (k, p) => this.$t(k, p));
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
        if (item.verdict === 'pass' || item.verdict === 'n/a') {
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

    // The Context lines with each line's advice note and the page that helps (contextAdvice.mjs).
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
    // fail, warn, ok -- with Passed placed first in the ok group and the "Alt tags" section directly
    // before it (A1.2, reviewChecklist.mjs auxLayout).
    auxLayout() {
      const aux = auxSections({ report: this.report, structure: this.structure, t: (k, p) => this.$t(k, p) });
      return auxLayout(aux, !!this.altTags);
    },

    // Buttons by the SERVER profile's permission (never a role name).
    structureUi() {
      return structureButtons(this.$can(STRUCTURE_PERMISSION));
    },

    stagedFixes() {
      return Object.values(this.staged).filter((s) => s.action === 'fixed' && s.fix);
    },

    // REVIEW-TRIM-SPEC D6: "Acknowledge (author fix expected)" when nothing can be rendered.
    structureAckLabel() {
      return structureAcknowledgeLabel(this.structure && this.structure.coverage);
    },

    // REVIEW-TRIM-SPEC D8: the Done rule is reviewChecklist.mjs `isReviewDone` -- the blockers and
    // acknowledgements decided (recorded or staged), and for a structure admin D4.2's structure
    // finding; advisory entries never gate Done. A user without the permission cannot act on the
    // structure finding, so it never holds their Done hostage.
    isDone() {
      const decided = (key) => !!(this.staged[key] || this.latest[key]);
      // REVIEW-TRIM-2-SPEC Stage 4 F4: never Done while a run is starting (the shown report is the old one).
      return isReviewDone({
        running: this.isRunning || this.runStarting,
        report: this.report,
        verdict: this.verdictName,
        entries: this.entries,
        decided,
        structureAcknowledge: this.structureUi.acknowledge,
        structureKey: this.structure && this.structure.finding ? this.structure.finding.key : null,
      });
    },
  },

  methods: {
    load() {
      return this.$api.getCampaignReview(this.id, true).then((d) => {
        const first = !this.state;
        this.state = d;
        // A1.1: a run whose first message came before the first load takes the report shown now.
        if (first && this.altsRow && this.altsRow.baseJob === undefined) {
          this.altsRow = { ...this.altsRow, baseJob: this.report ? this.report.jobId : null };
        }
        // REVIEW-TRIM-2-SPEC D4: the run's base -- the latest job id at the first load.
        if (first && this.run && this.runBaseJob === undefined) {
          this.runBaseJob = (d && d.review && d.review.job_id) || null;
        }
        this.schedulePoll();
      }).catch(() => this.schedulePoll());
    },

    // 2 s while running (or while a run is starting), 5 s otherwise (D11).
    schedulePoll() {
      clearTimeout(this.pollID);
      this.pollID = setTimeout(this.load, this.isRunning || this.runStarting ? 2000 : 5000);
    },

    // REVIEW-TRIM-2-SPEC D9: the disabled Fix-for-me button's one line -- the planner's reason, else the todo.
    fixTooltip(e) {
      return (e.finding && (e.finding.fixNote || e.finding.todo)) || '';
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

    // REVIEW-TRIM-SPEC D7 (reviewChecklist.mjs acceptState): cannot | override | ok, by the SERVER
    // profile's permission.
    acceptStateOf(item) {
      return acceptState(item, (perm) => this.$can(perm));
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
        // A named window: a second fallback navigates the first tab rather than opening another.
        // The page it mounts hands itself back as this window's editor (Campaign.vue mounted).
        const win = window.open(url, fallbackWindowName(this.id));
        if (win) {
          win.focus();
        }
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
      // REVIEW-TRIM-2-SPEC D4: the campaign page's `failed` -- no job follows this run.
      if (acceptRun({
        origin: ev.origin, ourOrigin: origin, source: ev.source, opener: this.openerWindow(), data: ev.data, campaignId: this.id, run: this.run,
      })) {
        this.runFailed = true;
        return;
      }
      // ALT-AUTOGEN-SPEC D9 / A1.1: the campaign page's generation messages -- from this window's
      // opener only, de-duplicated by token (the page re-posts each message on the opener schedule).
      // A new run records the shown report's job (undefined until the review state has loaded).
      if (acceptAlts({
        origin: ev.origin, ourOrigin: origin, source: ev.source, opener: this.openerWindow(), data: ev.data, campaignId: this.id,
      })) {
        let base;
        if (this.state) {
          base = this.report ? this.report.jobId : null;
        }
        this.altsRow = altsRowState(this.altsRow, ev.data, base);
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
      return loadFrameBuilder(this.$refs.builderFrame, BUILDER_CACHE_BUST);
    },

    // Apply the staged fixes to the SAVED campaign, recompile a visual body, PUT it. Returns the fix
    // objects that were applied AND saved (empty when nothing could be).
    async applyStagedFixes() {
      const raw = await this.$api.getCampaignRaw(this.id);
      // REVIEW-TRIM-2-SPEC D9: campaign.resave / footer.official apply only while the stored body /
      // design is the inspected one (sha256 equality).
      const digests = await campaignDigests(raw).catch(() => ({}));
      const {
        campaign, applied, skipped, recompile,
      } = applyFixes(raw, this.stagedFixes.map((s) => s.fix), digests);
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
        // ALT-AUTOGEN-SPEC D5 step 6: the one compile, shared with the campaign page's alt generation.
        body = compileCampaignBody(em, campaign, tpls, lists || []);
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

  watch: {
    // REVIEW-TRIM-2-SPEC D4: once the run is over (its job is shown, or it ended with a line), the
    // `run` token leaves the URL -- through history.replaceState, never the router (the root
    // <router-view> is keyed on fullPath) -- so a reload shows the latest report as today.
    'runState.mode': function onRunMode(mode) {
      if (this.run && mode !== 'starting') {
        const { pathname, hash } = window.location;
        window.history.replaceState(window.history.state, '', `${pathname}${hash}`);
      }
    },
  },

  mounted() {
    this.loadCampaign();
    this.load();
    window.addEventListener('message', this.onMessage);
    // REVIEW-TRIM-2-SPEC D4: absent any message, the 90 s bound ends the starting state.
    if (this.run) {
      this.runTimer = setTimeout(() => { this.runElapsed = RUN_FALLBACK_MS; }, RUN_FALLBACK_MS);
    }
  },

  beforeDestroy() {
    clearTimeout(this.pollID);
    clearTimeout(this.runTimer);
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
  padding: 0.5rem 2.25rem 0.5rem 0.75rem;
  margin: 0.4rem 0;
}
.proposal-text {
  white-space: pre-wrap;
  font-size: 1.05em;
  margin: 0;
}
/* The icon-only Copy at the proposal box's top-right. */
.proposal-copy {
  position: absolute;
  top: 0.35rem;
  right: 0.35rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.6rem;
  height: 1.6rem;
  padding: 0;
  border: 1px solid #b8e0c4;
  border-radius: 3px;
  background: #fff;
  color: #4a4a4a;
  cursor: pointer;
}
.proposal-copy:hover {
  border-color: #0055d4;
  color: #0055d4;
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
.context-note {
  font-weight: normal;
  margin: 0.25rem 0 0.4rem 0;
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
/* The .icon box (BIcon's root — reachable by scoped CSS) fills the button; the +/- glyph INSIDE
   it (i::before) is centred by the global .expand-btn rule in style.scss, which scoped CSS cannot
   reach (the <i> is nested in the child component). */
.expand-btn .icon {
  width: 100%;
  height: 100%;
  font-size: 0.7rem;
}
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
/* ALT-AUTOGEN-SPEC A1.2: the "Alt tags" section's groups (Approved, Modified, Generated). */
.alt-group + .alt-group {
  margin-top: 0.5rem;
}
.alt-group-head {
  font-weight: bold;
  font-size: 0.875rem;
}
.builder-frame {
  position: absolute;
  width: 0;
  height: 0;
  border: 0;
  visibility: hidden;
}
</style>
