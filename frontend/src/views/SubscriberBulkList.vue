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

        <list-selector label="Target lists" placeholder="Lists to apply to" v-model="form.lists" :selected="form.lists"
          :all="lists.results" />

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

export default Vue.extend({
  components: {
    ListSelector,
  },

  props: {
    numSubscribers: { type: Number, default: 0 },
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
    ...mapState(['lists', 'loading']),
  },
});
</script>
