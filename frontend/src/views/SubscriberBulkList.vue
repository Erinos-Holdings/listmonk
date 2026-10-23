<template>
  <form @submit.prevent="onSubmit">
    <div class="modal-card" style="width: auto">
      <header class="modal-card-head">
        <h4 class="title is-size-5">
          {{ $t('subscribers.manageLists') }}
        </h4>
      </header>

      <section expanded class="modal-card-body">
        <b-field label="Action">
          <div>
            <b-radio v-model="form.action" name="action" native-value="add" data-cy="check-list-add">
              {{ $t('globals.buttons.add') }}
            </b-radio>
            <b-radio v-model="form.action" name="action" native-value="remove" data-cy="check-list-remove">
              {{ $t('globals.buttons.remove') }}
            </b-radio>
            <b-radio v-model="form.action" name="action" native-value="unsubscribe" data-cy="check-list-unsubscribe">
              {{ $t('subscribers.markUnsubscribed') }}
            </b-radio>
          </div>
        </b-field>

        <!-- Fork (global brand, integrations GLOBAL-BRAND-SPEC D8) -- the option set is the
             caller's, by mode (Subscribers.vue bulkListOptions). JOURNEY-UX-SPEC A1.2: under Add
             the no_purchase step lists are left out (the server refuses a session's add); Remove
             and Unsubscribe offer every list, so cleanup keeps its only bulk UI. -->
        <list-selector label="Target lists" placeholder="Lists to apply to" v-model="form.lists" :selected="form.lists"
          :all="pickerOptions" />

        <!-- Fork: enabled for every list, not only double opt-in. Sending status=confirmed is the
             only way the ids path's ON CONFLICT re-subscribes a row a subscriber unsubscribed
             (add-subscribers-to-lists); on single opt-in lists that is the checkbox's sole effect. -->
        <b-field :message="$t('subscribers.preconfirmHelp')">
          <b-checkbox v-model="form.preconfirm" data-cy="preconfirm" :native-value="true"
            :disabled="form.action !== 'add' || form.lists.length === 0">
            {{ $t('subscribers.preconfirm') }}
          </b-checkbox>
        </b-field>

        <!-- Fork (evergreen) -->
        <b-field v-if="form.action === 'add'" :message="$t('subscribers.backfillHelp')">
          <b-checkbox v-model="form.backfill" data-cy="backfill" :native-value="true">
            {{ $t('subscribers.backfill') }}
          </b-checkbox>
        </b-field>
      </section>

      <footer class="modal-card-foot has-text-right">
        <b-button @click="$parent.close()">
          {{ $t('globals.buttons.close') }}
        </b-button>
        <b-button native-type="submit" type="is-primary" :disabled="form.lists.length === 0">
          {{ $t('globals.buttons.save') }}
        </b-button>
      </footer>
    </div>
  </form>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import ListSelector from '../components/ListSelector.vue';
import { manageListOptions } from '../campaignListUx.mjs'; // eslint-disable-line import/extensions

export default Vue.extend({
  components: {
    ListSelector,
  },

  props: {
    numSubscribers: { type: Number, default: 0 },
    // Fork (global brand, D8) -- the target lists offered: the scope united with the affected
    // rows' lists (every list under All brands). The modal no longer reads the store.
    options: { type: Array, default: () => [] },
  },

  data() {
    return {
      // Binds form input values.
      form: {
        action: 'add',
        lists: [],
        preconfirm: false,
        backfill: false,
      },
    };
  },

  methods: {
    onSubmit() {
      this.$emit('finished', this.form.action, this.form.preconfirm, this.form.lists, this.form.backfill);
      this.$parent.close();
    },
  },

  computed: {
    ...mapState(['loading']),

    pickerOptions() {
      return manageListOptions(this.options, this.form.action);
    },
  },

  watch: {
    // A step list chosen under Remove/Unsubscribe is dropped when the action becomes Add.
    'form.action': function onAction(action) {
      this.form.lists = manageListOptions(this.form.lists, action);
    },
  },
});
</script>
