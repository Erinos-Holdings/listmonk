<template>
  <!-- One Brand Usage row (moved out of BrandUsage.vue unchanged 2026-11-02, so the parent brand's
       band above the header and the sorted body rows render from this one markup). Every cell,
       circle, link and badge rule is brandUsage.mjs. -->
  <tr class="bu-row" :class="{ 'bu-zma': row.zma }">
    <td class="has-text-centered bu-score" :class="buTd('score')">
      <span :title="cell('score').tooltip || null" :data-cy="`usage-score-${row.brand}`">
        {{ cell('score').text }}</span>
    </td>
    <td class="bu-pad-r" :class="buTd('brand')">
      <span :data-cy="`usage-brand-${row.brand}`">{{ row.label }}</span>
      <b-tag v-if="row.zma" size="is-small" class="ml-1" :title="$t('dashboard.usage.zmaHelp')">
        {{ $t('dashboard.usage.zma') }}
      </b-tag>
    </td>
    <td v-for="c in columns" :key="c.field" class="has-text-centered"
      :class="[buTd(c.field), `bu-band-${c.band}`, padClass(c), { 'bu-start': c.first, 'bu-substart': c.divide, 'bu-sk': isSkipped(c.field) }]">
      <!-- V2 D3: one circle per state, the stored count inside. 2026-10-30: EACH circle links
           what it counts (brandUsage.mjs::circleLink) -- green the running campaigns / clean
           steps, amber the rest; red never links; each carries its own accessible name. -->
      <span v-if="cell(c.field).circles.length" class="bu-circles" :data-cy="`usage-${c.field}-${row.brand}`">
        <component v-for="ci in cell(c.field).circles" :key="ci.color"
          :is="circleTo(c.field, ci.color) ? 'router-link' : 'span'"
          :to="circleTo(c.field, ci.color) ? { name: 'campaigns', query: circleTo(c.field, ci.color) } : null"
          :class="['bu-circle', `bu-${ci.color}`]" :title="cell(c.field).tooltip"
          :aria-label="circleAria(ci)" :data-cy="`usage-${c.field}-${row.brand}-${ci.color}`">{{ ci.n }}</component>
      </span>
      <!-- DASHBOARDS-POLISH2-SPEC D2: the Cadence / Last broadcast text links the brand's
           lists in the Broadcasts scope (brandUsage.mjs::cellLink); '—' never links.
           POLISH3 D2/D5: the broadcast links carry status=finished; a Skipped count > 0
           is red and links its skipping lists in the All scope. MATRIX-POLISH-SPEC D10.3:
           Last broadcast links the last send's campaigns (?id=…, All) when the doc carries
           broadcasts.lastIds. -->
      <component v-else :is="link(c.field) ? 'router-link' : 'span'"
        :to="link(c.field) ? { name: 'campaigns', query: link(c.field) } : null"
        :class="cell(c.field).cls" :title="title(c.field)"
        :data-cy="`usage-${c.field}-${row.brand}`">
        <span>{{ cell(c.field).text }}</span>
      </component>
      <!-- LIFECYCLE-SERIES-SPEC S9 -- after a series group's last leaf: one grey "Nd legacy"
           chip per off-standard step (does not count toward the score), and one amber ⚠
           per step with no parseable days (POLISH3 stage 2 F5); the hover carries the
           legacy description or the parser error (brandUsage.mjs::cellBadges). -->
      <!-- USAGE-DECISIONS-SPEC D7: a legacy chip with stored campaigns opens them
           (b.link, the Day 0 circle's { id, evergreen: 'all' } shape). -->
      <component v-for="(b, bi) in badges(c.field)" :key="`w-${bi}`"
        :is="b.link ? 'router-link' : 'span'"
        :to="b.link ? { name: 'campaigns', query: b.link } : null"
        :class="b.kind === 'legacy' ? 'tag is-light is-small bu-legacy ml-1' : 'bu-warn has-text-warning-dark ml-1'"
        :title="b.tooltip" :aria-label="b.tooltip"
        :data-cy="b.kind === 'legacy' ? `usage-legacy-${row.brand}` : `usage-npwarn-${row.brand}`">
        <span>{{ b.glyph }}</span>
      </component>
    </td>
  </tr>
</template>

<script>
import Vue from 'vue';
import {
  SKIPPED_FIELDS, cell as cellOf, cellBadges, cellLink, circleLink, circleName, linkTip,
} from '../brandUsage.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'BrandUsageRow',

  props: {
    // A shapeRows row, the body columns (bodyColumns()) and the sorted field.
    row: { type: Object, required: true },
    columns: { type: Array, required: true },
    sortField: { type: String, default: '' },
  },

  methods: {
    t(k, p) {
      return this.$t(k, p);
    },

    cell(field) {
      return cellOf(this.row, field, this.t);
    },

    badges(field) {
      return cellBadges(this.row, field, this.t);
    },

    link(field) {
      return cellLink(this.row, field);
    },

    circleTo(field, color) {
      return circleLink(this.row, field, color);
    },

    // Each circle's own accessible name (its part, not the whole cell's tooltip). A Re-engagement
    // circle carries its own (brandUsage.mjs::reCircleName, USAGE-REENGAGEMENT-SPEC D3).
    circleAria(ci) {
      return ci.name || circleName(ci.color, ci.n, this.t);
    },

    title(field) {
      const parts = [this.cell(field).tooltip, linkTip(this.row, field, this.t)];
      return parts.filter(Boolean).join('\n') || null;
    },

    padClass(c) {
      return { 'bu-pad-l': !!c.padLeft, 'bu-pad-r': !!c.padRight };
    },

    buTd(field) {
      return { 'bu-sorted': this.sortField === field };
    },

    // 2026-11-02 (user): the Skipped cells share one minimum width (three digits or a circle), so
    // the Loyalty circles above them sit evenly spaced.
    isSkipped(field) {
      return SKIPPED_FIELDS.includes(field);
    },
  },
});
</script>
