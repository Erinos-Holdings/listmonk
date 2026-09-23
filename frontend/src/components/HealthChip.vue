<template>
  <!-- Fork (brand health, integrations BRAND-HEALTH-SPEC D3/D4/D12). Renders a status the
  BrandHealth Lambda computed; it computes nothing. Four states plus "no row for this tag".
  unknown is neutral grey and never carries a warning glyph. -->
  <span class="health-chip">
    <b-tooltip v-if="status" :label="tooltip" :active="!!tooltip" type="is-dark" multilined>
      <router-link v-if="to" :to="to" :data-cy="`health-${status}`">
        <b-tag :class="['health-tag', `health-${status}`]">{{ label }}</b-tag>
      </router-link>
      <b-tag v-else :class="['health-tag', `health-${status}`]" :data-cy="`health-${status}`">{{ label }}</b-tag>
    </b-tooltip>
    <b-tag v-else-if="missingTag" class="health-tag health-missing" data-cy="health-missing">
      {{ $t('lists.health.noRow') }}
    </b-tag>
    <!-- No row at all and no tag (an untagged list with no default-sender row): the unknown
    state. There are exactly four chip states; an absent row is never a fifth. -->
    <b-tag v-else class="health-tag health-unknown" data-cy="health-unknown">{{ $t('brands.status.unknown') }}</b-tag>
    <span v-if="hint" class="is-size-7 has-text-grey health-hint">{{ hint }}</span>
  </span>
</template>

<script>
const STATUSES = ['ok', 'warn', 'issues', 'unknown'];

export default {
  name: 'HealthChip',

  props: {
    // ok | warn | issues | unknown, or empty when there is no row.
    status: { type: String, default: '' },
    note: { type: String, default: '' },
    // A router location; the chip links there when set.
    to: { type: [Object, String], default: null },
    // The list's brand: tag value when it has no health row ("No health row for this tag").
    missingTag: { type: String, default: '' },
    // Small text after the chip, e.g. "(default sender)".
    hint: { type: String, default: '' },
    // Extra tooltip text (e.g. the as-of date).
    detail: { type: String, default: '' },
  },

  computed: {
    label() {
      const s = STATUSES.includes(this.status) ? this.status : 'unknown';
      return this.$t(`brands.status.${s}`);
    },

    tooltip() {
      return [this.note, this.detail].filter((x) => !!x).join(' · ');
    },
  },
};
</script>

<style lang="scss">
.health-chip {
  white-space: nowrap;

  .health-tag.tag {
    font-weight: 600;
  }
  .health-ok.tag {
    background-color: #e6f6ec;
    color: #1d6b3a;
  }
  .health-warn.tag {
    background-color: #fff4db;
    color: #8a5a00;
  }
  .health-issues.tag {
    background-color: #fde8e8;
    color: #a61b1b;
  }
  .health-unknown.tag,
  .health-missing.tag {
    background-color: #eeeeee;
    color: #666666;
  }
  .health-hint {
    margin-left: 0.35rem;
  }
}
</style>
