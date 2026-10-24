<template>
  <form @submit.prevent="onSubmit">
    <div class="modal-card content" style="width: auto">
      <header class="modal-card-head">
        <p v-if="isEditing" class="has-text-grey-light is-size-7">
          {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
          {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
        </p>
        <b-tag v-if="isEditing" :class="[data.type, 'is-pulled-right']">
          {{ $t(`lists.types.${data.type}`) }}
        </b-tag>
        <h4 v-if="isEditing">
          {{ data.name }}
        </h4>
        <h4 v-else>
          {{ $t('lists.newList') }}
        </h4>
      </header>
      <section expanded class="modal-card-body">
        <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D11) -- a list outside the selected
             brand opens with the notice; the form is otherwise unchanged. -->
        <brand-context-notice v-if="isEditing" :record-brand="data.brand || ''"
          :kind="$tc('globals.terms.list', 1).toLowerCase()" />
        <!-- BRAND-PICKER-SPEC D4: the locked list (models.LockedListNames) is read-only here --
        every control disabled, no Save; the server refuses the write with 409 regardless. -->
        <b-notification v-if="locked" type="is-warning" :closable="false" data-cy="list-locked">
          {{ $t('lists.lockedList', { name: data.name }) }}
        </b-notification>
        <b-field :label="$t('globals.fields.name')" label-position="on-border">
          <b-input :maxlength="200" :ref="'focus'" v-model="form.name" name="name"
            :placeholder="$t('globals.fields.name')" required :disabled="locked" />
        </b-field>

        <!-- BRAND-PICKER-SPEC D3: the brand is chosen, never typed; the server writes the
        brand:/from:/site: tags from the brands row. Required, no empty option. GLOBAL-BRAND-SPEC
        D10: under a selected brand it is preset to that brand and disabled (a new list, or a list
        already of that brand); a list of another brand keeps it enabled, with the notice. -->
        <!-- JOURNEY-UX-SPEC A1.5: the From line sits directly under the help (no field margin
        between them while it shows). -->
        <b-field :label="$t('lists.brand')" label-position="on-border" :message="$t('lists.brandHelp')"
          :class="{ 'mb-0': !!chosenBrand }">
          <b-select v-model="brand" name="brand" :placeholder="$t('lists.brand')" :required="!locked" expanded
            :disabled="locked || brandLocked" data-cy="list-brand">
            <option v-for="b in brands" :key="b.slug" :value="b.slug">
              {{ brandLabel(b) }}
            </option>
          </b-select>
        </b-field>
        <p v-if="chosenBrand" class="is-size-7 has-text-grey mb-4" data-cy="list-brand-from">
          {{ $t('lists.brandFrom', { from: chosenBrand.from_email }) }}
        </p>

        <!-- Fork (list join triggers, LIST-JOIN-TRIGGER-SPEC D7; shown on NEW lists too since the
        G10 pass -- the form creates, then PUTs the trigger onto the new id, because POST ignores
        the fields by design). Every control follows the locked flag. The badge reads the STORED
        trigger_sync, written by the reconciler (listTrigger.mjs). JOURNEY-UX-SPEC D8: a Broadcast |
        Journey selector over the same join_trigger -- Broadcast saves null (the retired None), the
        Journey pane always carries a kind (F4). -->
        <div class="list-join-trigger mt-5" data-cy="list-join-trigger">
          <p class="has-text-weight-semibold mb-2">
            {{ $t('lists.campaignType') }}
            <b-tag v-if="triggerBadge" :type="triggerBadgeType" class="ml-2" data-cy="list-trigger-badge">
              {{ triggerBadgeText }}
            </b-tag>
          </p>
          <!-- JOURNEY-UX-SPEC A1.4: the help follows the selection (listTypeHelpKey). -->
          <b-field :message="$t(listTypeHelpKey(listType))" data-cy="list-campaign-type">
            <b-radio-button v-model="listType" native-value="broadcast" type="is-primary" :disabled="locked">
              {{ $t('lists.campaignTypeBroadcast') }}
            </b-radio-button>
            <b-radio-button v-model="listType" native-value="journey" type="is-primary" :disabled="locked">
              {{ $t('lists.campaignTypeJourney') }}
            </b-radio-button>
          </b-field>
          <!-- JOURNEY-UX-SPEC A1.3: a radio's help is a bold "Sends ..." lead, then the prose
          (triggerHelpKeys), shown under that radio ONLY while it is selected, every time it is
          (user, 2026-10-24). A plain v-if element, never Buefy's #message slot: b-field does not
          re-read a slot that appears or disappears after mount, which is why the slot version
          showed only the first time and never for SKUs (the G-H2 finding). -->
          <div v-if="listType === 'journey'" class="list-journey-pane" data-cy="list-journey-pane">
            <div class="trigger-option">
              <b-field>
                <b-radio v-model="triggerOption" native-value="first_order" name="join_trigger" :disabled="locked">
                  {{ $t('lists.joinTriggerFirstOrder') }}
                </b-radio>
              </b-field>
              <p v-if="triggerOption === 'first_order'" class="help trigger-help" data-cy="trigger-help-first_order">
                <strong>{{ $t(helpFor('first_order').lead) }}</strong> {{ $t(helpFor('first_order').body) }}
              </p>
            </div>
            <div class="trigger-option">
              <b-field>
                <b-radio v-model="triggerOption" native-value="skus" name="join_trigger" :disabled="locked">
                  {{ $t('lists.joinTriggerSkus') }}
                </b-radio>
              </b-field>
              <p v-if="triggerOption === 'skus'" class="help trigger-help" data-cy="trigger-help-skus">
                <strong>{{ $t(helpFor('skus').lead) }}</strong> {{ $t(helpFor('skus').body) }}
              </p>
            </div>
            <b-field v-if="triggerOption === 'skus'" :message="$t('lists.joinTriggerSkusHelp')">
              <b-input v-model="skusText" type="textarea" name="join_trigger_skus" :disabled="locked"
                data-cy="list-trigger-skus" />
            </b-field>
            <!-- NO-PURCHASE-STEPS-SPEC D7: a "still deciding" step -- {kind: no_purchase, days}. -->
            <div class="trigger-option">
              <b-field>
                <b-radio v-model="triggerOption" native-value="no_purchase" name="join_trigger" :disabled="locked"
                  data-cy="list-trigger-no-purchase">
                  {{ $t('lists.joinTriggerNoPurchase') }}
                </b-radio>
              </b-field>
              <p v-if="triggerOption === 'no_purchase'" class="help trigger-help" data-cy="trigger-help-no_purchase">
                <strong>{{ $t(helpFor('no_purchase').lead) }}</strong> {{ $t(helpFor('no_purchase').body) }}
              </p>
            </div>
            <b-field v-if="triggerOption === 'no_purchase'" :label="$t('lists.joinTriggerDays')"
              :type="daysText === '' || daysOk ? '' : 'is-danger'">
              <b-input v-model="daysText" type="number" min="1" max="365" step="1" name="join_trigger_days"
                :disabled="locked" data-cy="list-trigger-days" />
            </b-field>
          </div>
          <!-- JOURNEY-UX-SPEC F8: in both selector states -- Broadcast lists host automations too. -->
          <p v-if="isEditing" class="is-size-7 has-text-grey">
            <router-link :to="{ path: '/campaigns/new', query: { list_id: data.id } }" @click.native="$parent.close()">
              {{ $t('lists.joinTriggerAutomation') }}
            </router-link>
          </p>
        </div>

        <!-- JOURNEY-UX-SPEC D10/D11: Transactional apart from the journey section; then Tags,
        Description; Type and Opt-in demoted (defaults private/single), side by side. -->
        <hr class="form-divider" />
        <b-field v-if="canTransactional" :message="$t('lists.transactionalHelp')">
          <b-checkbox v-model="transactional" name="transactional" :disabled="locked" data-cy="list-transactional">
            {{ $t('lists.transactional') }}
          </b-checkbox>
        </b-field>

        <b-field :label="$t('globals.terms.tags')" label-position="on-border" :message="$t('lists.tagsHelp')"
          :type="tagError ? 'is-danger' : ''">
          <b-taginput v-model="form.tags" name="tags" ellipsis icon="tag-outline"
            :placeholder="$t('globals.terms.tags')" :before-adding="beforeAddingTag" :disabled="locked" />
        </b-field>
        <p v-if="tagError" class="help is-danger">{{ tagError }}</p>

        <b-field :label="$t('globals.fields.description')" label-position="on-border">
          <b-input :maxlength="2000" v-model="form.description" name="description" type="textarea"
            :placeholder="$t('globals.fields.description')" :disabled="locked" />
        </b-field>

        <hr class="form-divider" />
        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('lists.type')" label-position="on-border" :message="$t('lists.typeHelp')">
              <b-select v-model="form.type" name="type" :placeholder="$t('lists.typeHelp')" required expanded
                :disabled="locked">
                <option value="private">
                  {{ $t('lists.types.private') }}
                </option>
                <option value="public">
                  {{ $t('lists.types.public') }}
                </option>
              </b-select>
            </b-field>
          </div>
          <div class="column is-6">
            <b-field :label="$t('lists.optin')" label-position="on-border" :message="$t('lists.optinHelp')">
              <b-select v-model="form.optin" name="optin" placeholder="Opt-in type" required expanded :disabled="locked">
                <option value="single">
                  {{ $t('lists.optins.single') }}
                </option>
                <option value="double">
                  {{ $t('lists.optins.double') }}
                </option>
              </b-select>
            </b-field>
          </div>
        </div>

        <b-field :message="$t('lists.archivedHelp')" :label="$t('lists.archived')" class="mt-5">
          <b-switch v-model="isArchived" name="status" :disabled="locked" />
        </b-field>
      </section>
      <footer class="modal-card-foot has-text-right">
        <b-button @click="$parent.close()">
          {{ $t('globals.buttons.close') }}
        </b-button>
        <b-button v-if="!locked && ($can('lists:manage_all') || $canList(data.id, 'list:manage'))" native-type="submit"
          type="is-primary" :loading="loading.lists" data-cy="btn-save">
          {{ $t('globals.buttons.save') }}
        </b-button>
      </footer>
    </div>
  </form>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import CopyText from '../components/CopyText.vue';
import BrandContextNotice from '../components/BrandContextNotice.vue';
import {
  brandLabel, isLockedList, isReservedTag, splitReservedTags,
} from '../listBrand.mjs'; // eslint-disable-line import/extensions
import {
  buildListFormRequest, buildListTypeTrigger, daysValid, journeyOption, listTypeHelpKey, listTypeOf, skusToText,
  storedDays, storedTrigger, storedTriggerSync, triggerBadge, triggerHelpKeys, LIST_TYPE_BROADCAST,
} from '../listTrigger.mjs'; // eslint-disable-line import/extensions
import { formBrandDefault } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import brandScopeMixin from '../brandScopeMixin';

export default Vue.extend({
  name: 'ListForm',

  components: {
    CopyText,
    BrandContextNotice,
  },

  // Fork (global brand, integrations GLOBAL-BRAND-SPEC D10).
  mixins: [brandScopeMixin],

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      // Binds form input values.
      form: {
        name: '',
        type: 'private',
        optin: 'single',
        status: 'active',
        tags: [],
      },

      // BRAND-PICKER-SPEC D3: the brands rows (GET /api/brands) and the chosen slug.
      brands: [],
      brand: null,
      tagError: '',

      // JOURNEY-UX-SPEC D8: Broadcast | Journey (Broadcast on a new list).
      listType: LIST_TYPE_BROADCAST,
      // LIST-JOIN-TRIGGER-SPEC D7: the Journey pane's radio (always a kind, F4), the SKU textarea
      // and the checkbox.
      triggerOption: 'first_order',
      skusText: '',
      // NO-PURCHASE-STEPS-SPEC D7: the no_purchase step's days input.
      daysText: '',
      transactional: false,
    };
  },

  methods: {
    brandLabel,
    listTypeHelpKey,

    // JOURNEY-UX-SPEC A1.3: a radio's own Sends lead and prose keys, selection-independent.
    helpFor(option) {
      return triggerHelpKeys(option) || { lead: '', body: '' };
    },

    // The server refuses a reserved tag (lists.reservedTag); refuse it here first, with the same
    // message.
    beforeAddingTag(tag) {
      if (isReservedTag(tag)) {
        this.tagError = this.$t('lists.reservedTag', { tag: String(tag).trim() });
        return false;
      }
      this.tagError = '';
      return true;
    },

    onSubmit() {
      if (this.locked) {
        return;
      }
      if (this.isEditing) {
        this.updateList();
        return;
      }

      this.createList();
    },

    createList() {
      // The server's POST ignores join_trigger/transactional (LIST-JOIN-TRIGGER-SPEC D3), so a
      // trigger chosen on a new list is applied with a second call onto the created id. The
      // list exists either way; a refused trigger surfaces as the PUT's error toast.
      const joinTrigger = buildListTypeTrigger(this.listType, this.triggerOption, this.skusText, this.daysText);
      const wantsTrigger = joinTrigger !== null || (this.canTransactional && this.transactional);
      this.$api.createList(buildListFormRequest(this.form, this.brand)).then((data) => {
        const done = () => {
          this.$emit('finished');
          this.$parent.close();
          this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));
        };
        if (!wantsTrigger) {
          done();
          return;
        }
        const req = buildListFormRequest(this.form, this.brand, {
          joinTrigger,
          canTransactional: this.canTransactional,
          transactional: this.transactional,
        });
        // On a refused trigger the list still exists: close and refresh anyway (the API layer
        // has toasted the reason), so a retry edits the created list instead of creating twice.
        this.$api.updateList({ id: data.id, ...req }).then(done, () => {
          this.$emit('finished');
          this.$parent.close();
        });
      });
    },

    updateList() {
      // D7/F3: an allow-listed body; transactional only when the checkbox is rendered (D5).
      const req = buildListFormRequest(this.form, this.brand, {
        joinTrigger: buildListTypeTrigger(this.listType, this.triggerOption, this.skusText, this.daysText),
        canTransactional: this.canTransactional,
        transactional: this.transactional,
      });
      this.$api.updateList({ id: this.data.id, ...req }).then((data) => {
        this.$emit('finished');
        this.$parent.close();
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
      });
    },
  },

  computed: {
    ...mapState(['loading', 'profile']),

    chosenBrand() {
      return this.brands.find((b) => b.slug === this.brand) || null;
    },

    // Fork (global brand, D10) -- the selection's preset for this form ('list': none presets
    // nothing and locks nothing -- a brandless list is never created by the form).
    brandDefault() {
      return formBrandDefault(this.brandSelection, 'list');
    },

    // The Brand dropdown is disabled under a selected brand for a new list and for a list already
    // of that brand; a list of another brand keeps it enabled (S4 -- the notice says why).
    brandLocked() {
      if (!this.brandDefault.locked) {
        return false;
      }
      return !this.isEditing || (this.$props.data.brand || '') === this.brandDefault.value;
    },

    // LIST-JOIN-TRIGGER-SPEC D5/D7: the Transactional checkbox is rendered only for holders of
    // lists:transactional.
    canTransactional() {
      return this.$can('lists:transactional');
    },

    // NO-PURCHASE-STEPS-SPEC D7: the days field's inline state (the server is the control).
    daysOk() {
      return daysValid(this.daysText);
    },

    // The badge from the STORED trigger and trigger_sync (null = no trigger, no badge).
    triggerBadge() {
      return triggerBadge(storedTrigger(this.data), storedTriggerSync(this.data));
    },

    triggerBadgeText() {
      const b = this.triggerBadge;
      if (!b) return '';
      if (b.state === 'live') return this.$t(b.key, { time: this.$utils.niceDate(b.params.time, true) });
      return this.$t(b.key, b.params);
    },

    triggerBadgeType() {
      const b = this.triggerBadge;
      if (!b) return '';
      return { live: 'is-success', invalid: 'is-danger', pending: 'is-warning' }[b.state];
    },

    // The render catalog list: read-only in the form (BRAND-PICKER-SPEC D4).
    locked() {
      return this.isEditing && isLockedList(this.data);
    },

    isArchived: {
      get() {
        return this.form.status === 'archived';
      },
      set(v) {
        this.form.status = v ? 'archived' : 'active';
      },
    },
  },

  mounted() {
    this.form = { ...this.form, ...this.$props.data };
    // The Tags field shows the free tags only; the reserved ones are the brand's projection. On
    // edit the select is preset from the row's brand; an untagged list shows it empty and required.
    this.form.tags = splitReservedTags(this.form.tags).free;
    this.brand = this.$props.data.brand || null;
    // LIST-JOIN-TRIGGER-SPEC D7: the section starts from the stored trigger (read under either
    // key spelling -- the row arrives camel-cased, storedTrigger).
    const jt = storedTrigger(this.$props.data);
    this.listType = listTypeOf(jt);
    this.triggerOption = journeyOption(jt);
    this.skusText = skusToText(jt && jt.skus);
    this.daysText = storedDays(jt);
    this.transactional = !!this.$props.data.transactional;
    // Fork (global brand, D10) -- a new list under a selected brand is that brand's.
    if (!this.isEditing && this.brandDefault.locked) {
      this.brand = this.brandDefault.value;
    }
    this.$api.getBrands().then((data) => {
      this.brands = Array.isArray(data) ? data : [];
    });

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });
  },
});
</script>
