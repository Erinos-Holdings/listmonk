<template>
  <!-- Fork (blocklist composition, integrations BLOCKLIST-COMPOSITION-SPEC D5/D6). DISPOSABLE Vue
  (upstream deletes this SPA at v7). Renders a stored `composition` field verbatim -- the SES
  pane's account-wide one or a brand document's scoped one. The integrations BrandHealth Lambda
  computes every bucket, count, domain and code (lib/blocklist-composition.ts owns the rules and
  the vocabulary); the only arithmetic here is each row's share of the stored total
  (compositionRows.mjs). Nothing at all for an absent field. -->
  <div v-if="state !== 'absent'" class="box blocklist-composition" data-cy="blocklist-composition">
    <h3 class="title is-6 mb-1">
      {{ $t('composition.title') }}
      <span v-if="composition.asOf" class="is-size-7 has-text-grey">{{ $t('composition.asOf', { date: composition.asOf }) }}</span>
    </h3>
    <p class="is-size-7 has-text-grey mb-2">
      {{ $t(scope === 'brand' ? 'composition.brandScope' : 'composition.accountScope') }}
    </p>
    <p class="is-size-7 mb-2" data-cy="composition-total">
      <strong>{{ $t('composition.total', { n: fmt(composition.total) }) }}</strong>
    </p>

    <p v-if="state === 'skipped'" class="is-size-7" data-cy="composition-skipped">{{ $t('composition.skippedVolume') }}</p>

    <table v-else class="table is-narrow is-fullwidth is-size-7">
      <thead>
        <tr>
          <th>{{ $t('composition.cause') }}</th>
          <th class="has-text-right">{{ $t('composition.count') }}</th>
          <th class="has-text-right">{{ $t('composition.share') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.key" :data-cy="`composition-${r.key}`">
          <td>
            <b-tooltip v-if="r.helpKey" :label="$t(r.helpKey)" type="is-dark" multilined append-to-body>
              <span class="composition-label">{{ $t(r.labelKey) }}</span>
            </b-tooltip>
            <code v-else>{{ r.label }}</code>
            <p v-if="r.key === 'hard:block'" class="has-text-grey">{{ $t('composition.bucket.hardBlockNote') }}</p>
            <!-- The pane's collapsed-detail pattern: a native <details> (as Import.vue's preset
            previews), closed by default. -->
            <details v-if="r.hasDetail" class="composition-detail">
              <summary class="has-text-grey">{{ $t('composition.details') }}</summary>
              <div class="columns is-gapless mt-1">
                <div v-if="r.domains.length" class="column">
                  <p class="has-text-grey">{{ $t('composition.domains') }}</p>
                  <ul><li v-for="d in r.domains" :key="d.domain">{{ d.domain }} · {{ d.nText }}</li></ul>
                </div>
                <div v-if="r.codes.length" class="column">
                  <p class="has-text-grey">{{ $t('composition.codes') }}</p>
                  <ul><li v-for="c in r.codes" :key="c.code">{{ c.code }} · {{ c.nText }}</li></ul>
                </div>
              </div>
            </details>
          </td>
          <td class="has-text-right">{{ r.countText }}</td>
          <td class="has-text-right">{{ r.share || '—' }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script>
import { compositionRows, compositionState } from '../compositionRows.mjs'; // eslint-disable-line import/extensions

export default {
  name: 'BlocklistComposition',

  props: {
    composition: { type: Object, default: null },
    // 'account' (the SES pane) or 'brand' (the Brands detail) -- selects the scope line only.
    scope: { type: String, default: 'account' },
  },

  computed: {
    state() {
      return compositionState(this.composition);
    },

    rows() {
      return compositionRows(this.composition, (n) => this.fmt(n));
    },
  },

  methods: {
    fmt(n) {
      return Number.isFinite(n) ? this.$utils.formatNumber(n) : '—';
    },
  },
};
</script>

<style lang="scss">
.blocklist-composition {
  .composition-label {
    border-bottom: 1px dotted #999;
    cursor: help;
  }
  .composition-detail summary {
    cursor: pointer;
  }
  .composition-detail ul {
    margin: 0;
  }
}
</style>
