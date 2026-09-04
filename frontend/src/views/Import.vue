<template>
  <section class="import">
    <h1 class="title is-4">
      {{ $t('import.title') }}
    </h1>
    <b-loading :active="isLoading" />

    <!-- Fork (import presets) -- disposable at v7 (SPA rewrite). One button per preset from
         /api/config; a panel with only a file drop and Preview; the preview summary and a
         Confirm. Everything else about the import is fixed server-side by the preset. -->
    <section v-if="isFree() && serverConfig.import_presets && serverConfig.import_presets.length" class="wrap">
      <div class="box import-preset" data-cy="import-presets">
        <template v-if="!preset.active">
          <div class="buttons">
            <b-button v-for="p in serverConfig.import_presets" :key="p.key" type="is-primary" outlined
              icon-left="file-upload-outline" :data-cy="`import-preset-${p.key}`" @click="openPreset(p)">
              {{ p.name }}
            </b-button>
          </div>
          <p class="is-size-7 has-text-grey">{{ $t('import.preset.help') }}</p>
        </template>

        <template v-else>
          <h4 class="title is-5">{{ preset.active.name }}</h4>

          <template v-if="!preset.preview">
            <p>{{ $t('import.preset.headersHelp') }}</p>
            <br />
            <blockquote class="csv-example">
              <code class="csv-headers">
                <span v-for="(h, i) in preset.active.headers" :key="h">{{ h }}{{ i < preset.active.headers.length - 1 ? ',' : '' }}</span>
              </code>
            </blockquote>
            <br />
            <b-field>
              <b-upload v-model="preset.file" drag-drop expanded accept=".csv">
                <div class="has-text-centered section">
                  <p><b-icon icon="file-upload-outline" size="is-large" /></p>
                  <p>{{ $t('import.preset.fileHelp') }}</p>
                </div>
              </b-upload>
            </b-field>
            <div class="tags" v-if="preset.file">
              <b-tag size="is-medium" closable @close="preset.file = null">{{ preset.file.name }}</b-tag>
            </div>
            <div class="buttons">
              <b-button type="is-primary" :disabled="!preset.file" :loading="isProcessing" @click="previewPreset">
                {{ $t('import.preset.preview') }}
              </b-button>
              <b-button @click="closePreset">{{ $t('import.preset.cancel') }}</b-button>
            </div>
          </template>

          <template v-else>
            <table class="table is-fullwidth is-narrow preset-preview">
              <tbody>
                <tr>
                  <th>{{ $t('import.preset.list') }}</th>
                  <td>
                    <strong>{{ preset.preview.list.name }}</strong>
                    <b-tag v-if="preset.preview.list.exists" type="is-light">
                      {{ $t('import.preset.listExists', { id: preset.preview.list.id, count: preset.preview.list.subscriber_count || 0 }) }}
                    </b-tag>
                    <b-tag v-else type="is-success is-light">{{ $t('import.preset.listCreate') }}</b-tag>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.encoding') }}</th>
                  <td>
                    <b-tag :type="preset.preview.encoding === 'utf-8' ? 'is-light' : 'is-warning is-light'">
                      {{ preset.preview.encoding }}
                    </b-tag>
                    <details v-if="preset.preview.non_ascii_names.length" class="preset-details">
                      <summary>{{ $t('import.preset.nonAsciiNames') }} ({{ preset.preview.non_ascii_names.length }})</summary>
                      <ul><li v-for="n in preset.preview.non_ascii_names" :key="n">{{ n }}</li></ul>
                    </details>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.rows') }}</th>
                  <td>{{ preset.preview.rows }}</td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.subscribers') }}</th>
                  <td>
                    <strong>{{ preset.preview.subscribers }}</strong>
                    &nbsp;({{ $t('import.preset.new') }}: {{ preset.preview.new }},
                    {{ $t('import.preset.existing') }}: {{ preset.preview.existing }})
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.willFillName') }}</th>
                  <td>
                    {{ preset.preview.will_fill_name.length }}
                    <details v-if="preset.preview.will_fill_name.length" class="preset-details">
                      <summary>&hellip;</summary>
                      <ul>
<li v-for="f in preset.preview.will_fill_name" :key="f.email">
                        {{ f.email }}: <em>{{ f.from || '(empty)' }}</em> &rarr; <strong>{{ f.to }}</strong>
                      </li>
</ul>
                    </details>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.willFillLang') }}</th>
                  <td>
                    {{ preset.preview.will_fill_lang.length }}
                    <details v-if="preset.preview.will_fill_lang.length" class="preset-details">
                      <summary>&hellip;</summary>
                      <ul><li v-for="f in preset.preview.will_fill_lang" :key="f.email">{{ f.email }}: {{ f.lang }}</li></ul>
                    </details>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.langLess') }}</th>
                  <td>
                    {{ preset.preview.lang_less }}
                    <details v-if="preset.preview.unmapped_locales.length" class="preset-details">
                      <summary>{{ $t('import.preset.unmappedLocales') }} ({{ preset.preview.unmapped_locales.length }})</summary>
                      <ul><li v-for="u in preset.preview.unmapped_locales" :key="u.email">{{ u.email }}: {{ u.locale }}</li></ul>
                    </details>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.duplicates') }}</th>
                  <td>
                    {{ preset.preview.duplicates.length }}
                    <details v-if="preset.preview.duplicates.length" class="preset-details">
                      <summary>&hellip;</summary>
                      <ul><li v-for="d in preset.preview.duplicates" :key="d.email">{{ d.email }} (rows {{ d.rows.join(', ') }})</li></ul>
                    </details>
                  </td>
                </tr>
                <tr>
                  <th>{{ $t('import.preset.skipped') }}</th>
                  <td>
                    {{ preset.preview.skipped.length }}
                    <details v-if="preset.preview.skipped.length" class="preset-details">
                      <summary>&hellip;</summary>
                      <ul><li v-for="s in preset.preview.skipped" :key="s.row">row {{ s.row }}: {{ s.email }} &mdash; {{ s.reason }}</li></ul>
                    </details>
                  </td>
                </tr>
                <tr v-if="preset.preview.warnings.length">
                  <th>{{ $t('import.preset.warnings') }}</th>
                  <td class="has-text-danger">
                    <ul><li v-for="w in preset.preview.warnings" :key="w">{{ w }}</li></ul>
                  </td>
                </tr>
              </tbody>
            </table>
            <div class="buttons">
              <b-button type="is-primary" :disabled="preset.preview.subscribers === 0" :loading="isProcessing"
                data-cy="import-preset-confirm" @click="confirmPreset">
                {{ $t('import.preset.confirm') }}
              </b-button>
              <b-button @click="closePreset">{{ $t('import.preset.cancel') }}</b-button>
            </div>
          </template>
        </template>
      </div>
    </section>

    <section v-if="isFree()" class="wrap">
      <form @submit.prevent="onUpload" class="box">
        <div>
          <div class="columns">
            <div class="column">
              <b-field :label="$t('import.mode')" :addons="false">
                <div>
                  <b-radio v-model="form.mode" name="mode" native-value="subscribe" data-cy="check-subscribe">
                    {{ $t('import.subscribe') }}
                  </b-radio>
                  <br />
                  <b-radio v-model="form.mode" name="mode" native-value="blocklist" data-cy="check-blocklist">
                    {{ $t('import.blocklist') }}
                  </b-radio>
                </div>
              </b-field>
            </div>
            <div class="column">
              <b-field :label="$t('globals.fields.status')" :addons="false">
                <template v-if="form.mode === 'subscribe'">
                  <b-radio v-model="form.subStatus" name="subStatus" native-value="unconfirmed"
                    data-cy="check-unconfirmed">
                    {{ $t('subscribers.status.unconfirmed') }}
                  </b-radio>
                  <b-radio v-model="form.subStatus" name="subStatus" native-value="confirmed" data-cy="check-confirmed">
                    {{ $t('subscribers.status.confirmed') }}
                  </b-radio>
                </template>

                <b-radio v-else v-model="form.subStatus" name="subStatus" native-value="unsubscribed"
                  data-cy="check-unsubscribed">
                  {{ $t('subscribers.status.unsubscribed') }}
                </b-radio>
              </b-field>
            </div>

            <div class="column">
              <b-field :label="$t('import.csvDelim')" :message="$t('import.csvDelimHelp')" class="delimiter">
                <b-input v-model="form.delim" name="delim" placeholder="," maxlength="1" required />
              </b-field>
            </div>
          </div>

          <div class="columns">
            <div class="column is-4">
              <b-field v-if="form.mode === 'subscribe'" :label="$t('import.overwriteUserInfo')"
                :message="$t('import.overwriteUserInfoHelp')">
                <div>
                  <b-switch v-model="form.overwriteUserInfo" name="overwriteUserInfo" data-cy="overwrite-user-info" />
                </div>
              </b-field>
            </div>

            <div class="column">
              <b-field v-if="form.mode === 'subscribe'" :label="$t('import.overwriteSubStatus')"
                :message="$t('import.overwriteSubStatusHelp')">
                <div>
                  <b-switch v-model="form.overwriteSubStatus" name="overwriteSubStatus"
                    data-cy="overwrite-sub-status" />
                </div>
              </b-field>
            </div>
          </div>

          <!-- Fork (evergreen) -- imported people are not new; keep automatic campaigns off them. -->
          <div class="columns" v-if="form.mode === 'subscribe'">
            <div class="column">
              <b-field :label="$t('subscribers.backfill')" :message="$t('subscribers.backfillHelp')">
                <div>
                  <b-switch v-model="form.backfill" name="backfill" data-cy="backfill" />
                </div>
              </b-field>
            </div>
          </div>

          <list-selector v-if="form.mode === 'subscribe'" :label="$t('globals.terms.lists')"
            :placeholder="$t('import.listSubHelp')" :message="$t('import.listSubHelp')" v-model="form.lists"
            :selected="form.lists" :all="lists.results" />
          <hr />

          <b-field :label="$t('import.csvFile')" label-position="on-border">
            <b-upload v-model="form.file" drag-drop expanded>
              <div class="has-text-centered section">
                <p>
                  <b-icon icon="file-upload-outline" size="is-large" />
                </p>
                <p>{{ $t('import.csvFileHelp') }}</p>
              </div>
            </b-upload>
          </b-field>
          <div class="tags" v-if="form.file">
            <b-tag size="is-medium" closable @close="clearFile">
              {{ form.file.name }}
            </b-tag>
          </div>
          <div class="buttons">
            <b-button native-type="submit" type="is-primary"
              :disabled="!form.file || (form.mode === 'subscribe' && form.lists.length === 0)" :loading="isProcessing">
              {{ $t('import.upload') }}
            </b-button>
          </div>
        </div>
      </form>
      <br /><br />

      <div class="import-help">
        <h5 class="title is-size-6">
          {{ $t('import.instructions') }}
        </h5>
        <p>{{ $t('import.instructionsHelp') }}</p>
        <br />
        <blockquote class="csv-example">
          <code class="csv-headers"> <span>email,</span> <span>name,</span> <span>attributes</span></code>
        </blockquote>

        <hr />

        <h5 class="title is-size-6">
          {{ $t('import.csvExample') }}
        </h5>

        <pre class="csv-example" v-text="example" />
      </div>
    </section><!-- upload //-->

    <section v-if="isRunning() || isDone()" class="wrap status box has-text-centered">
      <b-progress :value="progress" show-value type="is-success" />
      <br />
      <p
        :class="['is-size-5', 'is-capitalized', { 'has-text-success': status.status === 'finished' }, { 'has-text-danger': (status.status === 'failed' || status.status === 'stopped') }]">
        {{ status.status }}
      </p>

      <p>{{ $t('import.recordsCount', { num: status.imported, total: status.total }) }}</p>
      <br />

      <p>
        <b-button @click="stopImport" :loading="isProcessing" icon-left="file-upload-outline" type="is-primary">
          {{ isDone() ? $t('import.importDone') : $t('import.stopImport') }}
        </b-button>
      </p>
      <br />

      <div class="import-logs">
        <log-view :lines="logs" :loading="false" />
      </div>
    </section>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import ListSelector from '../components/ListSelector.vue';
import LogView from '../components/LogView.vue';

export default Vue.extend({
  components: {
    ListSelector,
    LogView,
  },

  props: {
    data: { type: Object, default: () => { } },
    isEditing: { type: Boolean, default: false },
  },

  data() {
    return {
      form: {
        mode: 'subscribe',
        subStatus: 'unconfirmed',
        delim: ',',
        lists: [],
        overwriteUserInfo: false,
        overwriteSubStatus: false,
        // Fork (evergreen) -- imported subscribers are not new; default ON.
        backfill: true,
        file: null,
        example: '',
      },

      // Fork (import presets) -- the active preset panel: which preset, the dropped file,
      // and the server's preview (null until Preview is clicked).
      preset: { active: null, file: null, preview: null },

      // Initial page load still has to wait for the status API to return
      // to either show the form or the status box.
      isLoading: true,

      isProcessing: false,
      status: { status: '' },
      logs: [],
      pollID: null,
    };
  },

  watch: {
    'form.mode': function formMode() {
      // Select the appropriate status radio whenever mode changes.
      this.$nextTick(() => {
        if (this.form.mode === 'subscribe') {
          this.form.subStatus = 'unconfirmed';
        } else {
          this.form.subStatus = 'unsubscribed';
        }
      });
    },
  },

  methods: {
    clearFile() {
      this.form.file = null;
    },

    // Fork (import presets).
    openPreset(p) {
      this.preset = { active: p, file: null, preview: null };
    },

    closePreset() {
      this.preset = { active: null, file: null, preview: null };
      this.isProcessing = false;
    },

    previewPreset() {
      this.isProcessing = true;
      const params = new FormData();
      params.set('file', this.preset.file, this.preset.file.name);
      this.$api.previewImportPreset(this.preset.active.key, params).then((data) => {
        this.isProcessing = false;
        this.preset.preview = data;
      }, () => {
        this.isProcessing = false;
      });
    },

    confirmPreset() {
      this.isProcessing = true;
      const params = new FormData();
      params.set('file', this.preset.file, this.preset.file.name);
      params.set('content_hash', this.preset.preview.content_hash);
      this.$api.importPreset(this.preset.active.key, params).then(() => {
        this.$utils.toast(this.$t('import.importStarted'));
        this.closePreset();
        this.pollStatus();
      }, () => {
        this.isProcessing = false;
      });
    },

    // Returns true if we're free to do an upload.
    isFree() {
      if (this.status.status === 'none') {
        return true;
      }
      return false;
    },

    // Returns true if an import is running.
    isRunning() {
      if (this.status.status === 'importing'
        || this.status.status === 'stopping') {
        return true;
      }
      return false;
    },

    isSuccessful() {
      return this.status.status === 'finished';
    },

    isFailed() {
      return (
        this.status.status === 'stopped'
        || this.status.status === 'failed'
      );
    },

    // Returns true if an import has finished (failed or successful).
    isDone() {
      if (this.status.status === 'finished'
        || this.status.status === 'stopped'
        || this.status.status === 'failed'
      ) {
        return true;
      }
      return false;
    },

    pollStatus() {
      // Clear any running status polls.
      clearInterval(this.pollID);

      // Poll for the status as long as the import is running.
      this.pollID = setInterval(() => {
        this.$api.getImportStatus().then((data) => {
          this.isProcessing = false;
          this.isLoading = false;
          this.status = data;
          this.getLogs();

          if (!this.isRunning()) {
            clearInterval(this.pollID);
          }
        }, () => {
          this.isProcessing = false;
          this.isLoading = false;
          this.status = { status: 'none' };
          clearInterval(this.pollID);
        });
        return true;
      }, 250);
    },

    getLogs() {
      this.$api.getImportLogs().then((data) => {
        this.logs = data.split('\n').map((line) => line.replace(/\s+importer\.go:\d+:\s*/, ' *: '));
        Vue.nextTick(() => {
          // vue.$refs doesn't work as the logs textarea is rendered dynamically.
          const ref = document.getElementById('import-log');
          if (ref) {
            ref.scrollTop = ref.scrollHeight;
          }
        });
      });
    },

    // Cancel a running import or clears a finished import.
    stopImport() {
      this.isProcessing = true;
      this.$api.stopImport().then(() => {
        this.pollStatus();
        this.form.file = null;
      });
    },

    renderExample() {
      const h = 'email,name,attributes\n'
        + 'user1@mail.com,"User One","{""age"": 42, ""planet"": ""Mars""}"\n'
        + 'user2@mail.com,"User Two","{""age"": 24, ""job"": ""Time Traveller""}"';

      this.example = h;
    },

    resetForm() {
      this.form.mode = 'subscribe';
      this.form.overwriteUserInfo = false;
      this.form.overwriteSubStatus = false;
      this.form.backfill = true;
      this.form.file = null;
      this.form.lists = [];
      this.form.subStatus = 'unconfirmed';
      this.form.delim = ',';
    },

    onUpload() {
      if (this.form.mode === 'subscribe' && this.form.overwriteSubStatus) {
        this.$utils.confirm(this.$t('import.subscribeWarning'), this.onSubmit, this.resetForm);
        return;
      }

      this.onSubmit();
    },

    onSubmit() {
      this.isProcessing = true;

      // Prepare the upload payload.
      const params = new FormData();
      params.set('params', JSON.stringify({
        mode: this.form.mode,
        subscription_status: this.form.subStatus,
        delim: this.form.delim,
        lists: this.form.lists.map((l) => l.id),
        overwrite_userinfo: this.form.overwriteUserInfo,
        overwrite_subscription_status: this.form.overwriteSubStatus,
        backfill: this.form.backfill,
      }));
      params.set('file', this.form.file);

      // Post.
      this.$api.importSubscribers(params).then(() => {
        // On file upload, show a confirmation.
        this.$utils.toast(this.$t('import.importStarted'));

        // Start polling status.
        this.pollStatus();
      }, () => {
        this.isProcessing = false;
        this.form.file = null;
      });
    },
  },

  computed: {
    ...mapState(['lists', 'serverConfig']),

    // Import progress bar value.
    progress() {
      if (!this.status || !this.status.total > 0) {
        return 0;
      }
      return Math.ceil((this.status.imported / this.status.total) * 100);
    },
  },

  mounted() {
    this.renderExample();
    this.pollStatus();

    const ids = this.$utils.parseQueryIDs(this.$route.query.list_id);
    if (ids.length > 0 && this.lists.results) {
      this.$nextTick(() => {
        this.form.lists = this.lists.results.filter((l) => ids.indexOf(l.id) > -1);
      });
    }
  },
});
</script>

<style lang="scss" scoped>
/* Fork (import presets) */
.preset-preview th {
  white-space: nowrap;
  width: 1%;
}
.preset-details {
  margin-top: 0.25rem;
  summary {
    cursor: pointer;
  }
  ul {
    margin: 0.25rem 0 0.5rem 1rem;
    list-style: disc;
  }
}
</style>
