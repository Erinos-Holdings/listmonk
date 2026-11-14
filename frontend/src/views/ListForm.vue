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
        <!-- Fork (journey lists, integrations JOURNEY-LISTS-SPEC D12): a standard journey list
        (journey_locked, the server's one definition) is read-only without lists:journeys; the
        Active switch below stays Marketing's. The server refuses the edit with 403 regardless. -->
        <b-notification v-if="journeyReadOnly" type="is-info" :closable="false" data-cy="list-journey-locked">
          {{ $t('lists.journeyLockedNotice') }}
        </b-notification>
        <!-- Fork (naming standard, NAMING-STANDARD-SPEC D3/D6) -- the limit is 30 after the brand
             prefix (`<SLUG> ` private, `<display name> ` public), so maxlength is 30 + the prefix;
             the hint names the rule.
             Fork (name guardrails, NAME-GUARDRAILS-SPEC §4 D6/D8) -- live naming warnings
             (naming.mjs::nameWarnings, warnings only) and the grey sample; a private list's brand
             prefix is a fixed addon before a required HANDLE input (listPrefixAddon), and the saved
             name is joinListName(addon, handle). Without the addon, the plain input. -->
        <!-- Keyed (review S4-1): Buefy caches its addon layout per instance, so the two branches
             must be distinct instances. pattern (S4-3): a whitespace-only handle is not a handle. -->
        <b-field v-if="nameAddon" key="name-addon" :label="$t('globals.fields.name')" label-position="on-border"
          :message="nameMessage" :type="nameMessageType" data-cy="list-name-addon">
          <p class="control">
            <span class="button is-static">{{ nameAddon }}</span>
          </p>
          <b-input :maxlength="handleLimit.maxlength || 200" :ref="'focus'" v-model="nameHandle" name="name"
            :placeholder="nameSample" required pattern=".*\S.*" :disabled="readOnly" expanded @blur="nameTouched = true" />
        </b-field>
        <b-field v-else key="name-plain" :label="$t('globals.fields.name')" label-position="on-border" :message="nameMessage"
          :type="nameMessageType">
          <b-input :maxlength="nameLimit.maxlength || 200" :ref="'focus'" v-model="form.name" name="name"
            :placeholder="nameSample" required :disabled="readOnly" @blur="nameTouched = true" />
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
            :disabled="readOnly || brandLocked" data-cy="list-brand">
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
          <!-- Fork (journey lists, JOURNEY-LISTS-SPEC D2/D12): the Active switch on any list with a
          STORED trigger, for a user with the list's manage permission, locked or not. It saves on
          its own (PUT /api/lists/:id/active), never with the form. Turning on needs a running
          automation (the server refuses otherwise). -->
          <div v-if="activeState.show" class="list-active-switch mb-3" data-cy="list-active">
            <b-field :message="$t('lists.activeHelp')">
              <b-switch :value="activeState.active" :disabled="activeBusy || (!activeState.active && !activeState.canTurnOn)"
                type="is-success" data-cy="list-active-switch" @input="onActiveToggle">
                {{ $t('lists.activeSwitch') }}
              </b-switch>
            </b-field>
            <!-- CAMPAIGN-LISTS-SPEC D9: red, not the unreadable yellow is-warning. -->
            <p v-if="activeState.hintKey" class="help is-danger" data-cy="list-active-hint">{{ $t(activeState.hintKey) }}</p>
            <p v-if="activeState.warningKey" class="help is-danger" data-cy="list-active-warning">{{ $t(activeState.warningKey) }}</p>
          </div>
          <!-- JOURNEY-UX-SPEC A1.4: the help follows the selection (listTypeHelpKey). -->
          <b-field :message="$t(listTypeHelpKey(listType))" data-cy="list-campaign-type">
            <b-radio-button v-model="listType" native-value="broadcast" type="is-primary" :disabled="readOnly">
              {{ $t('lists.campaignTypeBroadcast') }}
            </b-radio-button>
            <b-radio-button v-model="listType" native-value="journey" type="is-primary" :disabled="readOnly">
              {{ $t('lists.campaignTypeJourney') }}
            </b-radio-button>
          </b-field>
          <!-- JOURNEY-UX-SPEC A1.3: a radio's help is a bold "Sends ..." lead, then the prose
          (triggerHelpKeys), shown under that radio ONLY while it is selected, every time it is
          (user, 2026-10-24). A plain v-if element, never Buefy's #message slot: b-field does not
          re-read a slot that appears or disappears after mount, which is why the slot version
          showed only the first time and never for SKUs (the G-H2 finding). -->
          <div v-if="listType === 'journey'" class="list-journey-pane" data-cy="list-journey-pane">
            <!-- JOURNEY-LISTS-SPEC D12: the standard options are admin-only (lists:journeys). -->
            <p v-if="!canJourneys" class="help mb-2" data-cy="list-journey-admin-only">
              {{ $t('lists.journeyStandardAdminOnly') }}
            </p>
            <div class="trigger-option">
              <b-field>
                <b-radio v-model="triggerOption" native-value="first_order" name="join_trigger"
                  :disabled="readOnly || optionDisabled('first_order')">
                  {{ $t('lists.joinTriggerFirstOrder') }}
                </b-radio>
              </b-field>
              <p v-if="triggerOption === 'first_order'" class="help trigger-help" data-cy="trigger-help-first_order">
                <strong>{{ $t(helpFor('first_order').lead) }}</strong> {{ $t(helpFor('first_order').body) }}
              </p>
            </div>
            <div class="trigger-option">
              <b-field>
                <b-radio v-model="triggerOption" native-value="skus" name="join_trigger" :disabled="readOnly">
                  {{ $t('lists.joinTriggerSkus') }}
                </b-radio>
              </b-field>
              <p v-if="triggerOption === 'skus'" class="help trigger-help" data-cy="trigger-help-skus">
                <strong>{{ $t(helpFor('skus').lead) }}</strong> {{ $t(helpFor('skus').body) }}
              </p>
            </div>
            <b-field v-if="triggerOption === 'skus'" :message="$t('lists.joinTriggerSkusHelp')">
              <b-input v-model="skusText" type="textarea" name="join_trigger_skus" :disabled="readOnly"
                data-cy="list-trigger-skus" />
            </b-field>
            <!-- The two series step kinds (NO-PURCHASE-STEPS-SPEC D7: a No-purchase series step,
            {kind: no_purchase, days}; LIFECYCLE-SERIES-SPEC S2/S7: a Post-purchase series step,
            {kind: post_purchase, days}). Each option carries its own days field directly beneath it
            (2026-10-30), so the days sit under the selected step, never below the other one. -->
            <template v-for="k in stepKinds">
              <div :key="`opt-${k.value}`" class="trigger-option">
                <b-field>
                  <b-radio v-model="triggerOption" :native-value="k.value" name="join_trigger"
                    :disabled="readOnly || optionDisabled(k.value)" :data-cy="k.cy">
                    {{ $t(k.label) }}
                  </b-radio>
                </b-field>
                <p v-if="triggerOption === k.value" class="help trigger-help" :data-cy="`trigger-help-${k.value}`">
                  <strong>{{ $t(helpFor(k.value).lead) }}</strong> {{ $t(helpFor(k.value).body) }}
                </p>
              </div>
              <!-- LIFECYCLE-SERIES-SPEC S7/D5: the step's days -- 2/5 (No-purchase series) or 5/14/30
              (Post-purchase series), a stored off-pattern value as a "N days (legacy)" radio; each
              radio hovers with its D5 description (listTrigger.mjs::daysRadios). -->
              <b-field v-if="triggerOption === k.value" :key="`days-${k.value}`" class="trigger-days" :label="$t(k.daysLabel)"
                :type="daysText === '' || daysOk ? '' : 'is-danger'" data-cy="list-trigger-days">
                <b-radio v-for="r in daysOptions" :key="r.days" v-model="daysText" :native-value="String(r.days)"
                  name="join_trigger_days" :disabled="readOnly" :title="$t(r.help)" :data-cy="`list-trigger-days-${r.days}`">
                  {{ r.legacy ? $t('lists.joinTriggerDaysLegacy', { n: r.days }) : $tc('lists.joinTriggerDaysOption', r.days, { n: r.days }) }}
                </b-radio>
              </b-field>
            </template>
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
          <b-checkbox v-model="transactional" name="transactional" :disabled="readOnly" data-cy="list-transactional">
            {{ $t('lists.transactional') }}
          </b-checkbox>
        </b-field>

        <b-field :label="$t('globals.terms.tags')" label-position="on-border" :message="$t('lists.tagsHelp')"
          :type="tagError ? 'is-danger' : ''">
          <b-taginput v-model="form.tags" name="tags" ellipsis icon="tag-outline"
            :placeholder="$t('globals.terms.tags')" :before-adding="beforeAddingTag" :disabled="readOnly" />
        </b-field>
        <p v-if="tagError" class="help is-danger">{{ tagError }}</p>

        <b-field :label="$t('globals.fields.description')" label-position="on-border">
          <b-input :maxlength="2000" v-model="form.description" name="description" type="textarea"
            :placeholder="$t('globals.fields.description')" :disabled="readOnly" />
        </b-field>

        <hr class="form-divider" />
        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('lists.type')" label-position="on-border" :message="$t('lists.typeHelp')">
              <b-select v-model="form.type" name="type" :placeholder="$t('lists.typeHelp')" required expanded
                :disabled="readOnly">
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
              <b-select v-model="form.optin" name="optin" placeholder="Opt-in type" required expanded :disabled="readOnly">
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
          <b-switch v-model="isArchived" name="status" :disabled="readOnly" />
        </b-field>
      </section>
      <footer class="modal-card-foot has-text-right">
        <!-- JOURNEY-LISTS-SPEC D3/D12: saving a changed trigger, brand or transactional on an
        active list turns it off -- said before Save. -->
        <p v-if="turnsOffNotice" class="help is-warning mr-3" data-cy="list-active-turns-off">
          {{ $t('lists.activeTurnsOff') }}
        </p>
        <b-button @click="$parent.close()">
          {{ $t('globals.buttons.close') }}
        </b-button>
        <b-button v-if="!readOnly && ($can('lists:manage_all') || $canList(data.id, 'list:manage'))" native-type="submit"
          type="is-primary" :loading="loading.lists" :disabled="saveBlocked" data-cy="btn-save">
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
  buildListFormRequest, buildListTypeTrigger, daysRadios, daysValid, journeyOption, listTypeHelpKey, listTypeOf,
  skusToText, storedDays, storedTrigger, storedTriggerSync, triggerBadge, triggerHelpKeys, triggerOption as triggerOptionOf,
  LIST_TYPE_BROADCAST, activeSwitchState, formReadOnly, journeyOptionDisabled, journeySaveBlocked,
  triggerChangeTurnsOff,
} from '../listTrigger.mjs'; // eslint-disable-line import/extensions
import { formBrandDefault } from '../brandScope.mjs'; // eslint-disable-line import/extensions
import {
  joinListName, listPrefixAddon, nameCounter, namePlaceholder, nameWarnings, splitListName,
} from '../naming.mjs'; // eslint-disable-line import/extensions
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
      // LIST-JOIN-TRIGGER-SPEC D7: the Journey pane's radio, the SKU textarea and the checkbox.
      // JOURNEY-LISTS-SPEC D12: no radio is preselected ('') on an untriggered list.
      triggerOption: '',
      skusText: '',
      // NO-PURCHASE-STEPS-SPEC D7: the no_purchase step's days input.
      daysText: '',
      transactional: false,

      // JOURNEY-LISTS-SPEC D2a/D12: the list as GET /api/lists/:id returns it (running_automation
      // is on that response only), refreshed after an Active switch; null until read.
      fresh: null,
      activeBusy: false,

      // Fork (name guardrails, NAME-GUARDRAILS-SPEC D6/D8) -- the handle typed after the fixed
      // addon (kept as typed; form.name is its trimmed join), and whether the name field has been
      // left or a save tried (handleMissing shows only then).
      nameHandle: '',
      nameTouched: false,
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

    // JOURNEY-LISTS-SPEC D12: a standard radio is disabled without lists:journeys.
    optionDisabled(option) {
      return journeyOptionDisabled(option, this.canJourneys);
    },

    // JOURNEY-LISTS-SPEC D2: the switch saves on its own route; the form re-reads the list.
    onActiveToggle(on) {
      this.activeBusy = true;
      this.$api.putListActive(this.data.id, !!on).then((row) => {
        this.fresh = row;
        this.$emit('finished');
      }).finally(() => {
        this.activeBusy = false;
      });
    },

    onSubmit() {
      // Fork (name guardrails, D6) -- a save attempt counts as touched.
      this.nameTouched = true;
      if (this.readOnly || this.saveBlocked) {
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

    // Fork (naming standard, NAMING-STANDARD-SPEC D6) -- the brand prefix this name is measured
    // after (the same exact-match rule as models.ListNameRemainder), and the input's limit.
    namePrefix() {
      const b = this.chosenBrand;
      if (!b || !b.slug) return '';
      const p = this.form.type === 'public' ? `${b.display_name || ''} ` : `${b.slug.toUpperCase()} `;
      return p.trim() !== '' && (this.form.name || '').startsWith(p) ? p : '';
    },

    nameLimit() {
      return nameCounter('list', this.form.name, this.namePrefix);
    },

    // Fork (name guardrails, NAME-GUARDRAILS-SPEC D8) -- the fixed `<SLUG>` addon ('' = the plain
    // input): a private list with a brand, new or stored with that prefix.
    nameAddon() {
      return listPrefixAddon({
        brandSlug: this.brand || '',
        listType: this.form.type,
        isNew: !this.isEditing,
        storedName: this.$props.data.name || '',
      });
    },

    // The handle input's limit: the 30 applies to the handle itself.
    handleLimit() {
      return nameCounter('list', this.nameHandle);
    },

    // The shown warning codes (§4): none on a read-only form; handleMissing only once touched.
    nameWarningCodes() {
      if (this.readOnly) return [];
      return nameWarnings('list', this.form.name, { brands: this.brands, brandSlug: this.brand || '', listType: this.form.type })
        .filter((c) => c !== 'handleMissing' || this.nameTouched);
    },

    nameMessage() {
      const lines = this.nameWarningCodes.map((c) => this.$t(`naming.warn.${c}`));
      return lines.length ? lines : this.$t('naming.hint');
    },

    nameMessageType() {
      return this.nameWarningCodes.length ? 'is-warning' : null;
    },

    // The grey sample (D3): `Product Launch` in the handle input, else the whole sample name.
    nameSample() {
      if (this.nameAddon) return 'Product Launch';
      const b = this.chosenBrand;
      return namePlaceholder('list', {
        brandSlug: b ? b.slug : '', brandDisplay: b ? b.display_name : '', listType: this.form.type,
      });
    },

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

    // LIFECYCLE-SERIES-SPEC S7: the selected step option's radios (2/5 or 5/14/30), plus the
    // STORED value outside them when the stored trigger is that kind (not the selection, or picking
    // a preset would drop the stored choice) -- labeled legacy (D2).
    daysOptions() {
      const jt = storedTrigger(this.data);
      return daysRadios(this.triggerOption, triggerOptionOf(jt) === this.triggerOption ? storedDays(jt) : '');
    },

    // The two series step options, in form order, each with its days field's label.
    stepKinds() {
      return [
        {
          value: 'no_purchase', label: 'lists.joinTriggerNoPurchase', daysLabel: 'lists.joinTriggerDays', cy: 'list-trigger-no-purchase',
        },
        {
          value: 'post_purchase', label: 'lists.joinTriggerPostPurchase', daysLabel: 'lists.joinTriggerDaysPostPurchase', cy: 'list-trigger-post-purchase',
        },
      ];
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

    // JOURNEY-LISTS-SPEC D6/D12 -- lists:journeys; the stored row (the fresh GET when read).
    canJourneys() {
      return this.$can('lists:journeys');
    },

    row() {
      return this.fresh || this.$props.data;
    },

    // A standard journey list without lists:journeys: read-only, no Save.
    journeyReadOnly() {
      return this.isEditing && formReadOnly(this.row, this.canJourneys);
    },

    // Every form control follows readOnly (the render-catalog lock or the journey lock).
    readOnly() {
      return this.locked || this.journeyReadOnly;
    },

    // D12: Save stays disabled while Journey has no radio chosen.
    saveBlocked() {
      return journeySaveBlocked(this.listType, this.triggerOption);
    },

    // D12: the Active switch (edit only -- a new list has no stored trigger).
    activeState() {
      const canManage = this.$can('lists:manage_all') || this.$canList(this.data.id, 'list:manage');
      return activeSwitchState(this.isEditing ? this.row : {}, canManage);
    },

    // D3/D12: "Saving turns Active off" when the form's trigger, brand or transactional differ.
    turnsOffNotice() {
      if (!this.isEditing || this.readOnly || this.saveBlocked) return false;
      return triggerChangeTurnsOff(this.row, {
        joinTrigger: buildListTypeTrigger(this.listType, this.triggerOption, this.skusText, this.daysText),
        brand: this.brand || '',
        transactional: this.canTransactional ? this.transactional : undefined,
      });
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

  watch: {
    // Fork (name guardrails, NAME-GUARDRAILS-SPEC D8) -- form.name is always the addon joined with
    // the handle. When the addon appears (the brand is known, or the type turns private) the name
    // is re-split into it; an already-prefixed stored name is left byte-for-byte as stored.
    // When it goes away the plain input simply shows form.name, already the join.
    nameAddon(now, was) {
      if (!now) {
        // S4-5: nothing typed after the addon -> the plain input starts empty (its grey sample
        // shows), not with a bare prefix the user never typed.
        if (was && !this.nameHandle.trim()) this.form.name = '';
        return;
      }
      this.nameHandle = splitListName(this.form.name, now);
      if (!(this.form.name || '').startsWith(`${now} `)) {
        this.form.name = joinListName(now, this.nameHandle);
      }
    },

    nameHandle(h) {
      if (!this.nameAddon || splitListName(this.form.name, this.nameAddon) === h) return;
      this.form.name = joinListName(this.nameAddon, h);
    },

    // LIFECYCLE-SERIES-SPEC S7: switching between the two step kinds keeps the chosen days only
    // when the new kind offers it as a radio; otherwise the field empties (no hidden value is sent).
    triggerOption() {
      if (this.daysText !== '' && !this.daysOptions.some((r) => String(r.days) === this.daysText)) {
        this.daysText = '';
      }
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
    // JOURNEY-LISTS-SPEC D2a: running_automation is on the single-list GET only.
    if (this.isEditing && this.$props.data.id) {
      this.$api.getList(this.$props.data.id).then((row) => {
        this.fresh = row;
      });
    }

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });
  },
});
</script>
