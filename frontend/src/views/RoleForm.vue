<template>
  <form @submit.prevent="onSubmit">
    <div class="modal-card content" style="width: auto">
      <header class="modal-card-head">
        <p v-if="isEditing" class="has-text-grey-light is-size-7">
          {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
        </p>
        <h4 v-if="isEditing">
          {{ data.name }}
        </h4>
        <h4 v-else>
          {{ type === 'user' ? $t('users.newUserRole') : $t('users.newListRole') }}
        </h4>
      </header>

      <section expanded class="modal-card-body">
        <b-field :label="$t('globals.fields.name')" label-position="on-border">
          <b-input autofocus :disabled="disabled" :maxlength="200" v-model="form.name" name="name" ref="focus"
            required />
        </b-field>

        <!-- Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D7) -- a brand role grants by
             brand, never by list: Read/write brands and Read-only (analytics) brands, whose first
             option is All brands. A brand picked at one level is not offered at the other; All brands
             clears the other read-only picks (brandRoleForm.mjs). The per-list editor is gone. -->
        <div v-if="type === 'list'" class="box">
          <p class="is-size-7 has-text-grey mb-4">{{ $t('users.brandsHelp') }}</p>
          <div class="columns">
            <div class="column">
              <b-field :label="$t('users.brandsManage')" label-position="on-border">
                <b-select multiple native-size="8" :value="form.manage" :disabled="disabled" expanded
                  data-cy="brands-manage" @input="onManage">
                  <option v-for="s in manageOpts" :key="s" :value="s">{{ brandLabel(s) }}</option>
                </b-select>
              </b-field>
            </div>
            <div class="column">
              <b-field :label="$t('users.brandsAnalytics')" label-position="on-border">
                <b-select multiple native-size="8" :value="form.read" :disabled="disabled" expanded
                  data-cy="brands-analytics" @input="onRead">
                  <option v-for="s in readOpts" :key="s" :value="s">{{ brandLabel(s) }}</option>
                </b-select>
              </b-field>
            </div>
          </div>
        </div>

        <template v-if="type === 'user'">
          <div class="columns">
            <div class="column is-7">
              <h5 class="mb-0">
                {{ $t('users.perms') }}
              </h5>
            </div>
            <div class="column has-text-right" v-if="!disabled">
              <a href="#" @click.prevent="onToggleSelect">{{ $t('globals.buttons.toggleSelect') }}</a>
            </div>
          </div>

          <b-table :data="serverConfig.permissions">
            <b-table-column v-slot="props" field="group" :label="$t('users.roleGroup')">
              {{ $tc(`globals.terms.${props.row.group}`) }}
            </b-table-column>

            <b-table-column v-slot="props" field="permissions" label="Permissions">
              <div v-for="p in props.row.permissions" :key="p">
                <b-checkbox v-model="form.permissions" :native-value="p" :disabled="disabled">
                  {{ p }}
                  <a v-if="p === 'subscribers:sql_query'"
                    href="https://listmonk.app/docs/roles-and-permissions/#subscriberssql_query" target="_blank"
                    rel="noopener noreferrer" aria-label="Warning: high risk permission">
                    <b-icon icon="warning-empty" type="is-danger" size="is-small" />
                  </a>
                </b-checkbox>
              </div>
            </b-table-column>
          </b-table>
        </template>
        <a href="https://listmonk.app/docs/roles-and-permissions" target="_blank" rel="noopener noreferrer">
          <b-icon icon="link-variant" /> {{ $t('globals.buttons.learnMore') }}
        </a>
      </section>

      <footer class="modal-card-foot has-text-right">
        <b-button @click="$parent.close()">
          {{ $t('globals.buttons.close') }}
        </b-button>
        <b-button v-if="!disabled" native-type="submit" type="is-primary" :loading="loading.roles" data-cy="btn-save">
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
import stepUp, { stepUpError } from '../stepUp';
import {
  ALL_BRANDS, brandRolePayload, manageOptions, normalizeReadPicks, picksFromRole, readOptions,
} from '../brandRoleForm.mjs'; // eslint-disable-line import/extensions
import { brandLabelOf } from '../brandScope.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  name: 'RoleForm',

  components: {
    CopyText,
  },

  props: {
    data: { type: Object, default: () => ({}) },
    isEditing: { type: Boolean, default: false },
    type: { type: String, default: 'user' },
  },

  data() {
    return {
      // Binds form input values.
      form: {
        name: null,
        permissions: {},
        // Fork (brand roles, D7) -- the two pick arrays (slugs; ALL_BRANDS in read for All brands).
        manage: [],
        read: [],
      },
      hasToggle: false,
      disabled: false,
    };
  },

  methods: {
    // Fork (brand roles, D7).
    onManage(v) {
      this.form.manage = Array.isArray(v) ? v.filter((s) => s !== ALL_BRANDS) : [];
    },

    onRead(v) {
      this.form.read = normalizeReadPicks(v);
    },

    brandLabel(s) {
      return s === ALL_BRANDS ? this.$t('users.brandsAll') : brandLabelOf(s, this.brandRows);
    },

    onSubmit() {
      if (this.isEditing) {
        this.updateRole();
        return;
      }

      this.createRole();
    },

    onToggleSelect() {
      if (this.hasToggle) {
        this.form.permissions = [];
      } else {
        this.form.permissions = this.serverConfig.permissions.reduce((acc, item) => {
          item.permissions.forEach((p) => {
            acc.push(p);
          });
          return acc;
        }, []);
      }

      this.hasToggle = !this.hasToggle;
    },

    createRole() {
      let fn;
      const form = { name: this.form.name };

      if (this.$props.type === 'user') {
        fn = this.$api.createUserRole;
        form.permissions = this.form.permissions;
      } else {
        fn = this.$api.createListRole;
        form.brands = brandRolePayload(this.form.name, this.form.manage, this.form.read).brands;
      }

      // Fork (integrations STEPUP-ADMIN-SPEC D4) -- every role write steps up first.
      stepUp(this).then(() => fn(form)).then((data) => {
        this.$emit('finished');
        this.$utils.toast(this.$t('globals.messages.created', { name: data.name }));
        this.$parent.close();
      }).catch(stepUpError(this));
    },

    updateRole() {
      let fn;
      const form = { id: this.$props.data.id, name: this.form.name };

      if (this.$props.type === 'user') {
        fn = this.$api.updateUserRole;
        form.permissions = this.form.permissions;
      } else {
        fn = this.$api.updateListRole;
        form.brands = brandRolePayload(this.form.name, this.form.manage, this.form.read).brands;
      }

      stepUp(this).then(() => fn(form)).then((data) => {
        this.$emit('finished');
        this.$utils.toast(this.$t('globals.messages.updated', { name: data.name }));
        this.$parent.close();
      }).catch(stepUpError(this));
    },
  },

  computed: {
    ...mapState(['loading', 'serverConfig', 'brandRows']),

    // Fork (brand roles, D7) -- the brands rows (App.vue loads them for the navbar roster).
    manageOpts() {
      return manageOptions(this.brandRows, this.form.read);
    },

    readOpts() {
      return readOptions(this.brandRows, this.form.manage);
    },
  },

  mounted() {
    if (this.isEditing) {
      this.form = { ...this.form, ...this.$props.data };
      if (this.type === 'list') {
        const p = picksFromRole(this.$props.data);
        this.form.manage = p.manage;
        this.form.read = p.read;
      }

      // It's the superadmin role. Disable the form.
      if (this.$props.data.id === 1 || !this.$can('roles:manage')) {
        this.disabled = true;
      }
    } else {
      const skip = ['admin', 'users'];
      this.form.permissions = this.serverConfig.permissions.reduce((acc, item) => {
        if (skip.includes(item.group)) {
          return acc;
        }
        item.permissions.forEach((p) => {
          if (p !== 'subscribers:sql_query' && !p.startsWith('lists:') && !p.startsWith('settings:')) {
            acc.push(p);
          }
        });
        return acc;
      }, []);
    }

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });
  },
});
</script>
