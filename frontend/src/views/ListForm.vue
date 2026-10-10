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
        <!-- BRAND-PICKER-SPEC D4: the locked list (models.LockedListNames) is read-only here --
        every control disabled, no Save; the server refuses the write with 409 regardless. -->
        <b-notification v-if="locked" type="is-warning" :closable="false" data-cy="list-locked">
          {{ $t('lists.lockedList', { name: data.name }) }}
        </b-notification>
        <b-field :label="$t('globals.fields.name')" label-position="on-border">
          <b-input :maxlength="200" :ref="'focus'" v-model="form.name" name="name"
            :placeholder="$t('globals.fields.name')" required :disabled="locked" />
        </b-field>

        <b-field :label="$t('lists.type')" label-position="on-border" :message="$t('lists.typeHelp')">
          <b-select v-model="form.type" name="type" :placeholder="$t('lists.typeHelp')" required expanded :disabled="locked">
            <option value="private">
              {{ $t('lists.types.private') }}
            </option>
            <option value="public">
              {{ $t('lists.types.public') }}
            </option>
          </b-select>
        </b-field>

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

        <!-- BRAND-PICKER-SPEC D3: the brand is chosen, never typed; the server writes the
        brand:/from:/site: tags from the brands row. Required, no empty option. -->
        <b-field :label="$t('lists.brand')" label-position="on-border" :message="$t('lists.brandHelp')">
          <b-select v-model="brand" name="brand" :placeholder="$t('lists.brand')" :required="!locked" expanded
            :disabled="locked" data-cy="list-brand">
            <option v-for="b in brands" :key="b.slug" :value="b.slug">
              {{ brandLabel(b) }}
            </option>
          </b-select>
        </b-field>
        <p v-if="chosenBrand" class="is-size-7 has-text-grey mb-4" data-cy="list-brand-from">
          {{ $t('lists.brandFrom', { from: chosenBrand.from_email }) }}
        </p>

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

        <b-field :message="$t('lists.archivedHelp')" :label="$t('lists.archived')">
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
import {
  buildListRequest, brandLabel, isLockedList, isReservedTag, splitReservedTags,
} from '../listBrand.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'ListForm',

  components: {
    CopyText,
  },

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
    };
  },

  methods: {
    brandLabel,

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
      this.$api.createList(buildListRequest(this.form, this.brand)).then((data) => {
        this.$emit('finished');
        this.$parent.close();
        this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));
      });
    },

    updateList() {
      this.$api.updateList({ id: this.data.id, ...buildListRequest(this.form, this.brand) }).then((data) => {
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
    this.$api.getBrands().then((data) => {
      this.brands = Array.isArray(data) ? data : [];
    });

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });
  },
});
</script>
