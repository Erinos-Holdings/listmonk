<template>
  <section class="campaign">
    <header class="columns page-header">
      <div class="column is-6">
        <p v-if="isEditing && data.status" class="tags">
          <b-tag v-if="isEditing" :class="data.status">
            {{ $t(`campaigns.status.${data.status}`) }}
          </b-tag>
          <b-tag v-if="data.type === 'optin'" :class="data.type">
            {{ $t('lists.optin') }}
          </b-tag>
          <span v-if="isEditing" class="has-text-grey-light is-size-7" :data-campaign-id="data.id">
            {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
            {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
          </span>
        </p>
        <h4 v-if="isEditing" class="title is-4">
          {{ data.name }}
        </h4>
        <h4 v-else class="title is-4">
          {{ $t('campaigns.newCampaign') }}
        </h4>
      </div>

      <div class="column is-6">
        <div v-if="canManage || canSend" class="buttons">
          <!-- Fork (evergreen) -- a running evergreen is edited by pausing it first. -->
          <b-field grouped v-if="isEditing && data.evergreen && data.status === 'running' && canSend">
            <b-field expanded>
              <b-button expanded @click="$utils.confirm(null, pauseCampaign)" :loading="loading.campaigns"
                type="is-primary" icon-left="pause-circle-outline" data-cy="btn-pause-evergreen">
                {{ $t('campaigns.pause') }}
              </b-button>
            </b-field>
          </b-field>
          <b-field grouped v-if="isEditing && canEdit">
            <b-field v-if="canManage" expanded>
              <b-button expanded @click="() => onSubmit('update')" :loading="loading.campaigns" type="is-primary"
                :disabled="isBrandBlocked" icon-left="content-save-outline" data-cy="btn-save"
                aria-keyshortcuts="ctrl+s">
                <span class="has-kbd">{{ $t('globals.buttons.saveChanges') }} <span class="kbd">Ctrl+S</span></span>
              </b-button>
            </b-field>
            <b-field expanded v-if="canSend && canStart">
              <b-button expanded @click="startCampaign" :loading="loading.campaigns" type="is-primary"
                :disabled="isBrandBlocked" icon-left="rocket-launch-outline" data-cy="btn-start">
                {{ $t('campaigns.start') }}
              </b-button>
            </b-field>
            <b-field expanded v-if="canSend && canSchedule">
              <b-button expanded @click="startCampaign" :loading="loading.campaigns" type="is-primary"
                :disabled="isBrandBlocked" icon-left="clock-start" data-cy="btn-schedule">
                {{ $t('campaigns.schedule') }}
              </b-button>
            </b-field>
            <b-field expanded v-if="canSend && canUnSchedule">
              <b-button expanded @click="$utils.confirm(null, unscheduleCampaign)" :loading="loading.campaigns"
                type="is-primary" icon-left="clock-start" data-cy="btn-unschedule">
                {{ $t('campaigns.unSchedule') }}
              </b-button>
            </b-field>
          </b-field>
        </div>
      </div>
    </header>

    <b-loading :active="loading.campaigns" />

    <b-message v-if="isEditing && data.evergreen && data.status === 'running'" type="is-info" class="mb-4">
      {{ $t('campaigns.evergreenPauseToEdit') }}
    </b-message>

    <b-tabs type="is-boxed" :animated="false" v-model="activeTab" @input="onTab">
      <b-tab-item :label="$tc('globals.terms.campaign')" label-position="on-border" value="campaign"
        icon="rocket-launch-outline">
        <section class="wrap">
          <div class="columns">
            <div class="column is-7">
              <form @submit.prevent="() => onSubmit(isNew ? 'create' : 'update')">
                <b-field :label="$t('globals.fields.name')" label-position="on-border">
                  <b-input :maxlength="200" :ref="'focus'" v-model="form.name" name="name" :disabled="!canEdit"
                    :placeholder="$t('globals.fields.name')" required autofocus />
                </b-field>

                <b-field :label="$t('campaigns.subject')" label-position="on-border">
                  <b-input :maxlength="5000" v-model="form.subject" name="subject" :disabled="!canEdit"
                    :placeholder="$t('campaigns.subject')" required />
                </b-field>

                <b-field :label="$t('campaigns.preheader')" label-position="on-border"
                  :message="$t('campaigns.preheaderHelp')">
                  <b-input :maxlength="500" v-model="form.preheader" name="preheader" :disabled="!canEdit"
                    :placeholder="$t('campaigns.preheader')" />
                </b-field>

                <!-- READ-ONLY BY DESIGN: the From address is a property of the brand, derived from
                     the selected lists' `from:` tag. The escape hatch is editing the list, not the
                     campaign. `readonly` rather than `disabled` on purpose -- a disabled input is
                     skipped by browser validation, and `required` is the backstop for a derivation
                     that ever yields empty. -->
                <b-field :label="$t('campaigns.fromAddress')" label-position="on-border"
                  :type="brandDerivation.error ? 'is-danger' : ''" :message="brandFromMessage">
                  <!-- custom-class, NOT class: in Vue 2 a `class` on a component lands on its ROOT
                       element, which for b-input is the wrapping div.control -- so the styling
                       would never reach the <input>. Buefy's customClass prop is applied directly
                       to the input alongside its own classes. -->
                  <b-input :maxlength="200" v-model="form.fromEmail" name="from_email" :disabled="!canEdit" readonly
                    custom-class="is-derived" :placeholder="$t('campaigns.fromAddressPlaceholder')" required />
                </b-field>

                <list-selector v-model="form.lists" :selected="form.lists" :all="lists.results" :disabled="!canEdit"
                  :label="$t('globals.terms.lists')" :placeholder="$t('campaigns.sendToLists')" />

                <div class="columns">
                  <div class="column is-6">
                    <b-field :label="$tc('globals.terms.messenger')" label-position="on-border">
                      <b-select :placeholder="$tc('globals.terms.messenger')" v-model="form.messenger" name="messenger"
                        :disabled="!canEdit" required expanded>
                        <template v-if="emailMessengers.length > 1">
                          <optgroup label="email">
                            <option v-for="m in emailMessengers" :value="m" :key="m">
                              {{ m }}
                            </option>
                          </optgroup>
                        </template>
                        <template v-else>
                          <option value="email">email</option>
                        </template>
                        <option v-for="m in otherMessengers" :value="m" :key="m">{{ m }}</option>
                      </b-select>
                    </b-field>
                  </div>
                  <div class="column is-6">
                    <b-field :label="$t('campaigns.format')" label-position="on-border" class="mr-4 mb-0">
                      <b-select v-model="form.content.contentType" :disabled="!canEdit || isEditing" value="richtext"
                        expanded>
                        <option v-for="(name, f) in contentTypes" :key="f" name="format" :value="f"
                          :data-cy="`check-${f}`">
                          {{ name }}
                        </option>
                      </b-select>
                    </b-field>
                  </div>
                </div>

                <b-field :label="$t('globals.terms.tags')" label-position="on-border">
                  <b-taginput v-model="form.tags" name="tags" :disabled="!canEdit" ellipsis icon="tag-outline"
                    :placeholder="$t('globals.terms.tags')" />
                </b-field>
                <hr />

                <div class="columns">
                  <div class="column is-4">
                    <b-field :label="$t('campaigns.sendLater')" data-cy="btn-send-later">
                      <b-switch v-model="form.sendLater" :disabled="!canEdit || form.evergreen" />
                    </b-field>
                  </div>
                  <div class="column">
                    <br />
                    <b-field v-if="form.sendLater" data-cy="send_at"
                      :message="form.sendAtDate ? $utils.duration(Date(), form.sendAtDate) : ''">
                      <b-datetimepicker v-model="form.sendAtDate" :disabled="!canEdit" required editable mobile-native
                        position="is-top-right" :placeholder="$t('campaigns.dateAndTime')" icon="calendar-clock"
                        :timepicker="{ hourFormat: '24' }" :datetime-formatter="formatDateTime"
                        horizontal-time-picker />
                    </b-field>
                  </div>
                </div>

                <!-- Fork (evergreen) -- an evergreen campaign never finishes; it keeps sending to
                     subscribers who join its (single) list after it is started. -->
                <div class="columns" v-if="serverConfig.evergreen_enabled || form.evergreen">
                  <div class="column is-6">
                    <b-field :label="$t('campaigns.evergreen')" data-cy="btn-evergreen" class="evergreen-field"
                      :message="form.evergreen ? $t('campaigns.evergreenHelp') : ''">
                      <b-switch v-model="form.evergreen" :disabled="!canEdit || form.sendLater" />
                    </b-field>
                  </div>
                  <div class="column">
                    <b-field v-if="form.evergreen" :label="$t('campaigns.evergreenDelay')" class="evergreen-delay"
                      data-cy="evergreen-delay">
                      <b-numberinput v-model="form.sendDelayDays" :min="0" :max="365" :disabled="!canEdit"
                        controls-position="compact" />
                    </b-field>
                  </div>
                </div>

                <div>
                  <p class="has-text-right">
                    <a href="#" @click.prevent="onShowHeaders" data-cy="btn-headers">
                      <b-icon icon="plus" />{{ $t('settings.smtp.setCustomHeaders') }}
                    </a>
                  </p>
                  <b-field v-if="form.headersStr !== '[]' || isHeadersVisible" label-position="on-border"
                    :message="$t('campaigns.customHeadersHelp')">
                    <b-input v-model="form.headersStr" name="headers" type="textarea"
                      placeholder="[{&quot;X-Custom&quot;: &quot;value&quot;}, {&quot;X-Custom2&quot;: &quot;value&quot;}]"
                      :disabled="!canEdit" />
                  </b-field>
                </div>
                <hr />

                <b-field v-if="isNew">
                  <b-button native-type="submit" type="is-primary" :loading="loading.campaigns"
                    :disabled="isBrandBlocked" data-cy="btn-continue">
                    {{ $t('campaigns.continue') }}
                  </b-button>
                </b-field>
              </form>
            </div>
            <div v-if="canManage" class="column is-4 is-offset-1">
              <br />
              <div class="box">
                <h3 class="title is-size-6">
                  {{ $t('campaigns.sendTest') }}
                </h3>
                <b-field :message="$t('campaigns.sendTestHelp')">
                  <b-taginput ref="testEmails" v-model="form.testEmails" :before-adding="$utils.validateEmail"
                    :disabled="isNew" ellipsis icon="email-outline" :placeholder="$t('campaigns.testEmails')" />
                </b-field>
                <b-field>
                  <!-- SEND MUST MEAN SEND. Without the mousedown guard the first click is lost:
                       clicking blurs the taginput, Buefy's customOnBlur commits the pending text
                       as a chip SYNCHRONOUSLY, the field grows taller, this button moves down
                       between mousedown and mouseup, and the browser never dispatches a click. The
                       send silently does not happen, with no request and so no server-side trace.
                       Preventing the default on mousedown stops the blur, so nothing reflows and
                       the click lands. Keyboard users are unaffected -- Enter/Space fires click
                       with no mousedown. -->
                  <b-button @mousedown.native.prevent @click="() => onSubmit('test')"
                    :loading="loading.campaigns" :disabled="isNew" type="is-primary" icon-left="email-outline">
                    {{ $t('campaigns.send') }}
                  </b-button>
                </b-field>
              </div>
            </div>
          </div>
        </section>
      </b-tab-item><!-- campaign -->

      <b-tab-item :label="$t('campaigns.content')" icon="text" :disabled="isNew" value="content">
        <editor v-if="data.id" ref="editor" :key="editorKey" v-model="form.content" :id="data.id" :title="data.name"
          :disabled="!canEdit" :templates="templates" :content-types="contentTypes" :brand-palettes="brandPalettes" />

        <div class="columns">
          <div class="column is-6">
            <p v-if="!isAttachFieldVisible" class="is-size-6 has-text-grey">
              <a href="#" @click.prevent="onShowAttachField()" data-cy="btn-attach">
                <b-icon icon="file-upload-outline" size="is-small" />
                {{ $t('campaigns.addAttachments') }}
              </a>
            </p>

            <b-field v-if="isAttachFieldVisible" :label="$t('campaigns.attachments')" label-position="on-border"
              expanded data-cy="media">
              <b-taginput v-model="form.media" name="media" ellipsis icon="tag-outline" ref="media" field="filename"
                @focus="onOpenAttach" :disabled="!canEdit" />
            </b-field>
          </div>
          <div class="column has-text-right">
            <a href="https://listmonk.app/docs/templating/#template-expressions" target="_blank"
              rel="noopener noreferer">
              <b-icon icon="code" /> {{ $t('campaigns.templatingRef') }}</a>
            <span v-if="canEdit && form.content.contentType !== 'plain'" class="is-size-6 has-text-grey ml-6">
              <a v-if="form.altbody === null" href="#" @click.prevent="onAddAltBody">
                <b-icon icon="text" size="is-small" /> {{ $t('campaigns.addAltText') }}
              </a>
              <a v-else href="#" @click.prevent="$utils.confirm(null, onRemoveAltBody)">
                <b-icon icon="trash-can-outline" size="is-small" />
                {{ $t('campaigns.removeAltText') }}
              </a>
            </span>
          </div>
        </div>

        <div v-if="canEdit && form.content.contentType !== 'plain'" class="alt-body">
          <b-input v-if="form.altbody !== null" v-model="form.altbody" type="textarea" :disabled="!canEdit" />
        </div>
      </b-tab-item><!-- content -->

      <b-tab-item :label="$t('globals.terms.attribs')" icon="code" value="attribs" :disabled="isNew">
        <section class="wrap">
          <b-field :label="$t('globals.terms.attribs')" :message="$t('campaigns.attribsHelp')"
            label-position="on-border">
            <b-input v-model="form.attribsStr" type="textarea" :disabled="!canEdit" rows="15" />
          </b-field>
        </section>
      </b-tab-item><!-- attribs -->

      <b-tab-item :label="$t('campaigns.archive')" icon="newspaper-variant-outline" value="archive" :disabled="isNew">
        <section class="wrap">
          <div class="columns">
            <div class="column is-4">
              <b-field :label="$t('campaigns.archiveEnable')" data-cy="btn-archive"
                :message="$t('campaigns.archiveHelp')">
                <div class="columns">
                  <div class="column">
                    <b-switch data-cy="btn-archive" v-model="form.archive" :disabled="!canArchive" />
                  </div>
                  <div class="column is-12">
                    <a :href="`${serverConfig.root_url}/archive/${data.uuid}`" target="_blank" rel="noopener noreferer"
                      :class="{ 'has-text-grey-light': !form.archive }" aria-label="$t('campaigns.archive')">
                      <b-icon icon="link-variant" />
                    </a>
                  </div>
                </div>
              </b-field>
            </div>
            <div class="column is-8">
              <b-field grouped position="is-right">
                <b-field v-if="!canEdit && canArchive">
                  <b-button @click="onUpdateCampaignArchive" :loading="loading.campaigns" type="is-primary"
                    icon-left="content-save-outline" data-cy="btn-save">
                    {{ $t('globals.buttons.saveChanges') }}
                  </b-button>
                </b-field>
              </b-field>
            </div>
          </div>

          <div class="columns">
            <div class="column is-6">
              <b-field :label="$tc('globals.terms.template')" label-position="on-border">
                <b-select :placeholder="$tc('globals.terms.template')" v-model="form.archiveTemplateId" name="template"
                  :disabled="!canArchive || !form.archive || form.content.contentType === 'visual'" required>
                  <template v-for="t in templates">
                    <option v-if="t.type === 'campaign'" :value="t.id" :key="t.id">
                      {{ t.name }}
                    </option>
                  </template>
                </b-select>
              </b-field>
            </div>

            <div class="column is-6">
              <b-field grouped position="is-right">
                <b-field v-if="form.archive && (!this.form.archiveMetaStr || this.form.archiveMetaStr === '{}')">
                  <a class="button is-primary" href="#" @click.prevent="onFillArchiveMeta" aria-label="{}"><b-icon
                      icon="code" /></a>
                </b-field>
                <b-field v-if="form.archive">
                  <b-button @click="onToggleArchivePreview" type="is-primary" icon-left="file-find-outline"
                    data-cy="btn-preview">
                    {{ $t('campaigns.preview') }}
                  </b-button>
                </b-field>
              </b-field>
            </div>
          </div>
          <b-field>
            <b-field :label="$t('campaigns.archiveSlug')" label-position="on-border"
              :message="$t('campaigns.archiveSlugHelp')">
              <b-input :maxlength="200" :ref="'focus'" v-model="form.archiveSlug" name="archive_slug"
                data-cy="archive-slug" :disabled="!canArchive || !form.archive" />
            </b-field>
          </b-field>
          <b-field :label="$t('campaigns.archiveMeta')" :message="$t('campaigns.archiveMetaHelp')"
            label-position="on-border">
            <b-input v-model="form.archiveMetaStr" name="archive_meta" type="textarea" data-cy="archive-meta"
              :disabled="!canArchive || !form.archive" rows="20" />
          </b-field>
        </section>
      </b-tab-item><!-- archive -->
    </b-tabs>

    <b-modal scroll="keep" :aria-modal="true" :active.sync="isAttachModalOpen" :width="900">
      <div class="modal-card content" style="width: auto">
        <section expanded class="modal-card-body">
          <media is-modal @selected="onAttachSelect" />
        </section>
      </div>
    </b-modal>

    <campaign-preview v-if="isPreviewingArchive" @close="onToggleArchivePreview" type="campaign" :id="data.id"
      :archive-meta="form.archiveMetaStr" :title="data.title" :content-type="data.contentType"
      :template-id="form.archiveTemplateId" is-post is-archive />
  </section>
</template>

<script>
import dayjs from 'dayjs';
import htmlToPlainText from 'textversionjs';
import Vue from 'vue';
import { mapState } from 'vuex';
import {
  readDraft, writeDraft, deleteDraft, DRAFT_MAX_AGE_MS,
} from '../drafts';

import CampaignPreview from '../components/CampaignPreview.vue';
import CopyText from '../components/CopyText.vue';
import Editor from '../components/Editor.vue';
import ListSelector from '../components/ListSelector.vue';
import Media from './Media.vue';
import {
  BRAND_TAG_PREFIX, FROM_TAG_PREFIX, brandThemePalette, reBrandSlug,
} from '../brand';

// Canonical casing, folded at the comparison — mirrors `sesTagHeader` in cmd/campaigns_brand.go,
// which is the half that actually enforces this. The header name should exist exactly once per
// implementation; the two implementations have to agree, which is the whole reason both exist.
const SES_TAG_HEADER = 'X-SES-MESSAGE-TAGS';
const BRAND_TAG_KEY = 'brand';

// The mapping for a list carrying neither tag. `curated` pairs with app.from_email — an unmapped
// campaign gets the DEFAULT slug, never no tag at all, so the `unattributed` CloudWatch dimension
// stays reserved for genuine mistakes and the alarm on it stays trustworthy.
//
// MUST MATCH `defaultBrandSlug` in cmd/campaigns_brand.go, which is what actually enforces it.
// Nothing checks that these agree; if they diverge, the editor shows one brand and the backend
// stores another.
const DEFAULT_BRAND = 'curated';

// Rewrite the `brand=` pair inside an "a=b, c=d" SES message-tag value, leaving any other pair
// alone. Replacing the whole value would silently drop a second tag someone added by hand.
const setBrandInTagValue = (value, slug) => {
  const out = [];
  let replaced = false;

  String(value || '').split(',').map((p) => p.trim()).filter((p) => p !== '')
    .forEach((pair) => {
      const eq = pair.indexOf('=');
      const key = (eq === -1 ? pair : pair.slice(0, eq)).trim().toLowerCase();
      if (key === BRAND_TAG_KEY) {
        if (!replaced) {
          out.push(`${BRAND_TAG_KEY}=${slug}`);
          replaced = true;
        }
        return;
      }
      out.push(pair);
    });

  if (!replaced) {
    out.unshift(`${BRAND_TAG_KEY}=${slug}`);
  }

  return out.join(', ');
};

export default Vue.extend({
  components: {
    ListSelector,
    Editor,
    Media,
    CopyText,
    CampaignPreview,
  },

  data() {
    return {
      contentTypes: Object.freeze({
        richtext: this.$t('campaigns.richText'),
        html: this.$t('campaigns.rawHTML'),
        markdown: this.$t('campaigns.markdown'),
        plain: this.$t('campaigns.plainText'),
        visual: this.$t('campaigns.visual'),
      }),

      isNew: false,
      isEditing: false,
      isHeadersVisible: false,
      isAttachFieldVisible: false,
      isAttachModalOpen: false,
      isPreviewingArchive: false,
      activeTab: 'campaign',

      data: {},

      // IDs from ?list_id query param.
      selListIDs: [],

      // True between loading an existing campaign and the first settled derivation, so the
      // repoint TOAST fires once per load rather than on every subsequent list edit. The
      // persistent inline notice is the `brandFromRepointed` computed, which is self-clearing.
      brandFromLoadPending: false,

      // The brand swatch row(s) currently pushed into the visual editor's color picker, and
      // the slug of the latest theme request. The slug doubles as the stale-response guard:
      // rapid list changes leave several theme fetches in flight and an older response can
      // resolve last, so a response is discarded unless it answers the latest request.
      brandPalettes: [],
      brandThemeSlug: null,

      // The latest fetched palette (or null), cached so a sweep deferred by an error-transit
      // derivation can be re-evaluated without refetching.
      brandThemePalette: null,

      // DOCUMENT COLOR PROVENANCE — which brand's palette this design's colors are presumed
      // to carry; the rebrand sweep's mapping source. While no document exists it follows
      // each clean found:true derivation silently; once one does, it updates in exactly two
      // places: the first clean found:true derivation (initial load), and an Apply that
      // actually rewrote colors. On Keep it deliberately stays put — the doc still carries
      // the old hexes, so
      // a later swap to a third brand must map from the palette actually in the document.
      // Session-scoped by design (nothing is persisted); the zero-match Apply toast is the
      // mitigation for a reload mislabeling it.
      heldBrandPalette: null,

      // The brand slug the sweep prompt was last offered for, so the derivation watcher
      // (which refires on every unrelated list edit) offers once per brand arrival, while a
      // deliberate re-swap back and forth offers again.
      brandSweepOffered: null,

      // Fork (session expiry): when the campaign finished loading, so a stash written by
      // another tab after that is not clobbered by this one.
      loadedAt: 0,
      // Bumped by applyDraft to re-mount the editor on the restored content.
      editorKey: 0,

      // Binds form input values.
      form: {
        archiveSlug: null,
        name: '',
        subject: '',
        preheader: '',
        fromEmail: '',
        headersStr: '[]',
        headers: [],
        attribsStr: '{}',
        messenger: 'email',
        lists: [],
        tags: [],
        sendAt: null,
        content: {
          // We only send visual campaigns; a null bodySource makes the builder
          // start from its empty document (Outlook compatibility ON).
          contentType: 'visual',
          body: '',
          bodySource: null,
          templateId: null,
        },
        altbody: null,
        media: [],

        // Parsed Date() version of send_at from the API.
        sendAtDate: null,
        sendLater: false,
        // Fork (evergreen) -- "send automatically to new subscribers", delay in days.
        evergreen: false,
        sendDelayDays: 0,
        // Raw seconds as loaded; sent back unchanged when the days field is untouched so an
        // API-set sub-day delay is not silently rounded to 0 by a UI save.
        sendDelaySecs: 0,
        archive: false,
        archiveMetaStr: '{}',
        archiveMeta: {},
        testEmails: [],
      },
    };
  },

  methods: {
    formatDateTime(s) {
      return dayjs(s).format('YYYY-MM-DD HH:mm');
    },

    onToggleArchivePreview() {
      this.isPreviewingArchive = !this.isPreviewingArchive;
    },

    onAddAltBody() {
      this.form.altbody = htmlToPlainText(this.form.content.body);
    },

    onRemoveAltBody() {
      this.form.altbody = null;
    },

    onShowHeaders() {
      this.isHeadersVisible = !this.isHeadersVisible;
    },

    onShowAttachField() {
      this.isAttachFieldVisible = true;
      this.$nextTick(() => {
        this.$refs.media.focus();
      });
    },

    onOpenAttach() {
      this.isAttachModalOpen = true;
    },

    onAttachSelect(o) {
      if (this.form.media.some((m) => m.id === o.id)) {
        return;
      }

      this.form.media.push(o);
    },

    // Fork (session expiry): upstream compared body/contentType only, so a changed subject
    // or list never armed the leave guard. Now a real diff over the stashable fields, so
    // beforeRouteLeave / onbeforeunload and the draft stash all agree on "dirty".
    isUnsaved() {
      if (!this.isEditing || !this.data.id) {
        return false;
      }
      return JSON.stringify(this.draftFromForm()) !== JSON.stringify(this.draftFromCampaign(this.data));
    },

    // The whitelisted, JSON-safe view of the editor state, from the form ...
    draftFromForm() {
      const f = this.form;
      return this.draftShape({
        name: f.name,
        subject: f.subject,
        preheader: f.preheader,
        altbody: f.altbody,
        tags: f.tags,
        headersStr: f.headersStr,
        attribsStr: f.attribsStr,
        archive: f.archive,
        archiveSlug: f.archiveSlug,
        archiveMetaStr: f.archiveMetaStr,
        archiveTemplateId: f.archiveTemplateId,
        lists: f.lists,
        sendAtDate: f.sendAtDate,
        evergreen: f.evergreen,
        sendDelayDays: f.sendDelayDays,
        content: f.content,
      });
    },

    // ... and the same shape from the loaded campaign, so the two compare as equals.
    draftFromCampaign(c) {
      return this.draftShape({
        name: c.name,
        subject: c.subject,
        preheader: (c.attribs && c.attribs.preheader) || '',
        altbody: c.altbody,
        tags: c.tags,
        headersStr: JSON.stringify(c.headers, null, 4),
        attribsStr: c.attribs ? JSON.stringify(c.attribs, null, 4) : '{}',
        archive: c.archive,
        archiveSlug: c.archiveSlug,
        archiveMetaStr: c.archiveMeta ? JSON.stringify(c.archiveMeta, null, 4) : '{}',
        archiveTemplateId: c.archiveTemplateId,
        lists: c.lists,
        sendAtDate: c.sendAt ? dayjs(c.sendAt).toDate() : null,
        evergreen: c.evergreen,
        sendDelayDays: Math.round((c.sendDelaySecs || 0) / 86400),
        content: {
          body: c.body,
          bodySource: c.bodySource,
          contentType: c.contentType,
          templateId: c.templateId,
        },
      });
    },

    draftShape(f) {
      const d = f.sendAtDate;
      const validDate = d instanceof Date && !Number.isNaN(d.getTime());
      return {
        name: f.name || '',
        subject: f.subject || '',
        preheader: f.preheader || '',
        altbody: f.altbody || null,
        tags: f.tags || [],
        headersStr: f.headersStr,
        attribsStr: f.attribsStr,
        archive: !!f.archive,
        archiveSlug: f.archiveSlug || null,
        archiveMetaStr: f.archiveMetaStr,
        archiveTemplateId: f.archiveTemplateId || null,
        listIds: (f.lists || []).map((l) => l.id).sort((a, b) => a - b),
        sendAtDate: validDate ? d.toISOString() : null,
        evergreen: !!f.evergreen,
        sendDelayDays: Number(f.sendDelayDays) || 0,
        content: {
          // Visual content: the builder re-renders `body` from `bodySource` on load and the
          // HTML it emits need not byte-match what was stored, which made every untouched
          // visual campaign read as dirty ("Discard changes?" on navigation). The document
          // is the source of truth there; body is compared only for hand-written content.
          body: f.content.contentType === 'visual' ? '' : (f.content.body || ''),
          bodySource: f.content.bodySource || null,
          contentType: f.content.contentType,
          templateId: f.content.templateId || null,
        },
      };
    },

    // Stash the editor state before the session-expiry redirect. Only when dirty, and never
    // over another tab's stash for the same campaign unless it predates this tab's load.
    stashDraft(e) {
      if (!this.isEditing || !this.data.id || !this.isUnsaved()) {
        return;
      }
      const existing = readDraft(this.data.id);
      if (existing && existing.savedAt > this.loadedAt) {
        // Another tab already kept edits for this campaign; say so rather than "none".
        if (e && e.detail) {
          e.detail.skipped = true;
        }
        return;
      }
      writeDraft(this.data.id, {
        savedAt: Date.now(),
        userId: this.profile.id,
        campaignUpdatedAt: this.data.updatedAt,
        ...this.draftFromForm(),
      });
      if (e && e.detail) {
        e.detail.stashed = true;
      }
    },

    // Offer a stash back after the campaign has loaded and the editor has mounted.
    offerDraftRestore() {
      const { id } = this.data;
      const d = readDraft(id);
      if (!d) {
        return;
      }
      if (Date.now() - d.savedAt > DRAFT_MAX_AGE_MS || d.userId !== this.profile.id || !this.canEdit) {
        deleteDraft(id);
        return;
      }

      const at = dayjs(d.savedAt).format('HH:mm');
      const msg = d.campaignUpdatedAt === this.data.updatedAt
        ? this.$t('campaigns.draftRestore', { at })
        : this.$t('campaigns.draftRestoreChanged', { at, changedAt: dayjs(this.data.updatedAt).format('HH:mm') });

      // Buefy directly rather than $utils.confirm, which HTML-escapes the message; the
      // strings carry markup and no user-supplied text ({at} is a formatted time).
      this.$buefy.dialog.confirm({
        scroll: 'keep',
        message: msg,
        confirmText: this.$t('globals.buttons.ok'),
        cancelText: this.$t('globals.buttons.cancel'),
        onConfirm: () => {
          deleteDraft(id);
          this.applyDraft(d);
        },
        onCancel: () => deleteDraft(id),
      });
    },

    applyDraft(d) {
      this.form = {
        ...this.form,
        name: d.name,
        subject: d.subject,
        preheader: d.preheader,
        altbody: d.altbody,
        tags: d.tags,
        headersStr: d.headersStr,
        attribsStr: d.attribsStr,
        archive: d.archive,
        archiveSlug: d.archiveSlug,
        archiveMetaStr: d.archiveMetaStr,
        archiveTemplateId: d.archiveTemplateId,
        lists: (this.lists.results || []).filter((l) => d.listIds.indexOf(l.id) > -1),
        sendAtDate: d.sendAtDate ? new Date(d.sendAtDate) : null,
        sendLater: !!d.sendAtDate,
        evergreen: !!d.evergreen,
        sendDelayDays: Number(d.sendDelayDays) || 0,
        content: { ...d.content },
      };

      // The editor clones its value ONCE (Editor.vue `self`, no watcher on value) and the
      // visual builder reads its source once at iframe load, so a form.content assignment
      // alone leaves both showing the server copy while the form holds the restore — the
      // next input event would silently overwrite it. Re-mount the editor instead: it
      // re-clones from the restored content, and the iframe reloads with it as its source.
      this.editorKey += 1;
    },

    // Push the derived From address and `brand` message tag into the form.
    //
    // THE DERIVED VALUE IS FORM STATE ONLY UNTIL THE USER SAVES, and it is deliberately allowed to
    // mark the form dirty. Do NOT suppress that by rebasing `this.data` to match: the campaign goes
    // on sending with its STORED From until a save lands, so a "clean" form displaying a derived
    // address would show one address while another goes out -- the exact wrong-brand failure this
    // feature exists to prevent, wearing a fix's clothing.
    syncBrandDerivation() {
      // Never rewrite a campaign that cannot be edited anyway (canEditCampaign makes `finished`
      // terminal server-side); and never act before the lists store has loaded, or every campaign
      // would momentarily derive the default mapping and falsely announce a repoint.
      if (!this.canEdit || !this.lists.results || this.lists.results.length === 0) {
        return;
      }

      // Wait for a SETTLED derivation. An empty fromEmail means serverConfig has not loaded yet
      // (App.vue fetches it independently of the lists, so either can win the race) -- acting on
      // it would blank the field AND fire a spurious "updated to undefined" toast, which would
      // also consume the one-shot brandFromLoadPending flag so the REAL repoint never announces
      // itself. Not assigning a falsy value also keeps `required` a genuine backstop rather than
      // something this code trips by itself.
      const d = this.brandDerivation;
      if (d.error || !d.fromEmail) {
        return;
      }

      const stored = this.data.fromEmail || '';
      if (this.brandFromLoadPending) {
        this.brandFromLoadPending = false;

        if (stored !== '' && stored !== d.fromEmail) {
          this.$utils.toast(this.$t('campaigns.brandFromRepointed', { from: stored, to: d.fromEmail }), 'is-warning', 6000);
        }
      }

      if (this.form.fromEmail !== d.fromEmail) {
        this.form.fromEmail = d.fromEmail;
      }

      // Headers is a free-text JSON array carrying other headers too, so replace ONLY the
      // X-SES-MESSAGE-TAGS entry. Malformed JSON is left alone -- onSubmit already reports it.
      const merged = this.mergeBrandHeader(this.form.headersStr, d.brand);
      if (merged !== null && merged !== this.form.headersStr) {
        this.form.headersStr = merged;
      }
    },

    // Keep the visual editor's brand swatch row in sync with the derived brand. Display-only:
    // the row follows every CLEAN derivation (mapped or the unmapped default — if `curated`
    // ever publishes a catalog page, unmapped campaigns showing its swatches is intended),
    // while error transits (the add-then-remove list swap passes through a conflict error)
    // leave the current row alone; the next clean derivation refreshes it. Held provenance
    // for the rebrand sweep is separate state with its own rules.
    syncBrandTheme() {
      const d = this.brandDerivation;
      if (d.error || !d.brand) {
        return;
      }

      // Lowercase-fold before fetch and compare: the proxy folds anyway (catalog slugs are
      // canonically lowercase and its API is case-sensitive), and folding here keeps the
      // same-slug guard and the row label consistent for a merely mis-cased `brand:` tag.
      const slug = d.brand.toLowerCase();
      if (slug === this.brandThemeSlug) {
        // Same brand, no refetch — but a sweep may have been deferred: a theme response that
        // arrived while the derivation was transiting an error offers nothing, so re-evaluate
        // against the cached palette on every clean derivation.
        this.maybeOfferBrandSweep(this.brandThemePalette);
        return;
      }
      this.brandThemeSlug = slug;
      // A newly arriving brand may offer the sweep again — the once-only latch is per
      // arrival, not per campaign (a deliberate re-swap back and forth should re-prompt).
      this.brandSweepOffered = null;

      this.$api.getBrandTheme(slug).then((data) => {
        if (slug !== this.brandThemeSlug) {
          return;
        }

        const p = data.found ? brandThemePalette(slug, data.theme) : null;
        this.brandThemePalette = p;
        this.brandPalettes = p ? [p] : [];
        this.maybeOfferBrandSweep(p);
      }).catch(() => {
        // Soft-fail by design: a brand with no page (or a catalog outage past the proxy's
        // stale-serving) means no row, never an editor error. Clear rather than leave a
        // previous brand's row lingering.
        if (slug === this.brandThemeSlug) {
          this.brandThemePalette = null;
          this.brandPalettes = [];
        }
      });
    },

    // Rebrand sweep (confirm-prompted). Seeds held provenance on the first clean found:true
    // derivation; prompts when a later clean derivation's palette differs from it. The fetch's
    // stale guard covers the REQUEST — the derivation is re-read here because it may have
    // changed (to an error, or unmapped) while the response was in flight, and error/unmapped
    // states must neither seed, nor clear, nor prompt.
    maybeOfferBrandSweep(palette) {
      const d = this.brandDerivation;
      if (!palette || d.error || d.unmapped || !d.brand
        || d.brand.toLowerCase() !== palette.label) {
        return;
      }

      // No document yet (an unsaved campaign — the editor is v-if="data.id" — or a campaign
      // whose design was never built): there is nothing to sweep and nothing whose provenance
      // could be pinned, so held provenance silently follows the current brand — the design
      // built from here will carry these colors. Prompting here would offer to re-map a
      // design that does not exist, and Apply's null path would misdiagnose it as "editor
      // isn't ready".
      if (!this.data.id || !this.form.content.bodySource) {
        this.heldBrandPalette = palette;
        return;
      }

      if (!this.heldBrandPalette) {
        this.heldBrandPalette = palette;
        return;
      }
      if (this.heldBrandPalette.label === palette.label) {
        return;
      }

      // The sweep only means anything on a visual campaign, and is offered once per brand
      // arrival (the derivation watcher refires on every unrelated list edit).
      if (this.form.content.contentType !== 'visual' || this.brandSweepOffered === palette.label) {
        return;
      }
      this.brandSweepOffered = palette.label;

      this.$buefy.dialog.confirm({
        scroll: 'keep',
        message: this.$utils.escapeHTML(this.$t('campaigns.brandSweepPrompt', {
          old: this.heldBrandPalette.label, new: palette.label,
        })),
        confirmText: this.$t('campaigns.brandSweepApply'),
        cancelText: this.$t('campaigns.brandSweepKeep'),
        onConfirm: () => this.applyBrandSweep(palette),
        // Keep (cancel/dismiss) is deliberately a no-op: provenance stays with the palette
        // actually in the document.
      });
    },

    applyBrandSweep(newPalette) {
      const replaced = this.$refs.editor
        ? this.$refs.editor.remapBrandColors(this.form.content.bodySource, this.heldBrandPalette, newPalette)
        : null;

      if (replaced === null) {
        this.$utils.toast(this.$t('campaigns.brandSweepUnavailable'), 'is-warning');
        return;
      }
      if (replaced === 0) {
        // Session-scoped provenance can mislabel a design after a Keep-then-save-then-reload;
        // a mute no-op Apply would hide exactly that. Report it, and leave provenance where
        // it was — nothing was rewritten, so nothing changed hands.
        this.$utils.toast(this.$t('campaigns.brandSweepNoMatches'), 'is-warning');
        return;
      }
      this.heldBrandPalette = newPalette;
    },

    // Merge `brand=<slug>` into the campaign's headers array, preserving every other entry and
    // every other key within the X-SES-MESSAGE-TAGS entry itself. Returns null if the textarea
    // does not currently hold a JSON array.
    mergeBrandHeader(headersStr, slug) {
      let arr = null;
      try {
        arr = JSON.parse(headersStr && headersStr.trim() !== '' ? headersStr : '[]');
      } catch (e) {
        return null;
      }

      if (!Array.isArray(arr)) {
        return null;
      }

      let found = false;
      const next = arr.map((entry) => {
        if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
          return entry;
        }

        const out = {};
        Object.keys(entry).forEach((k) => {
          if (k.toLowerCase() === SES_TAG_HEADER.toLowerCase()) {
            found = true;
            out[k] = setBrandInTagValue(entry[k], slug);
          } else {
            out[k] = entry[k];
          }
        });
        return out;
      });

      if (!found) {
        next.push({ [SES_TAG_HEADER]: `${BRAND_TAG_KEY}=${slug}` });
      }

      return JSON.stringify(next, null, 4);
    },

    onTab(tab) {
      if (tab === 'content' && window.tinymce && window.tinymce.editors.length > 0) {
        this.$nextTick(() => {
          window.tinymce.editors[0].focus();
        });
      }

      // this.$router.replace({ hash: `#${tab}` });
      window.history.replaceState({}, '', `#${tab}`);
    },

    onFillArchiveMeta() {
      const archiveStr = `{"email": "email@domain.com", "name": "${this.$t('globals.fields.name')}", "attribs": {}}`;
      this.form.archiveMetaStr = this.$utils.getPref('campaign.archiveMetaStr') || JSON.stringify(JSON.parse(archiveStr), null, 4);
    },

    onSubmit(typ) {
      // The controls are disabled while a derivation is blocked, but Ctrl+S and a native form
      // submit reach here regardless.
      if (this.isBrandBlocked) {
        this.$utils.toast(this.brandDerivation.error, 'is-danger');
        return;
      }

      // Validate custom JSON headers.
      if (this.form.headersStr && this.form.headersStr !== '[]') {
        try {
          this.form.headers = JSON.parse(this.form.headersStr);
        } catch (e) {
          this.$utils.toast(e.toString(), 'is-danger');
          return;
        }
      } else {
        this.form.headers = [];
      }

      // Validate archive JSON body.
      if (this.form.archive && this.form.archiveMetaStr) {
        try {
          this.form.archiveMeta = JSON.parse(this.form.archiveMetaStr);
        } catch (e) {
          this.$utils.toast(e.toString(), 'is-danger');
          return;
        }
      }

      // Validate custom JSON attribs.
      let attribs = null;
      if (this.form.attribsStr && this.form.attribsStr.trim()) {
        try {
          attribs = JSON.parse(this.form.attribsStr);
        } catch (e) {
          this.$utils.toast(
            `${this.$t('subscribers.invalidJSON')}: ${e.toString()}`,
            'is-danger',

            3000,
          );
          return;
        }
      }

      // Fold the preheader field into attribs — it lives under attribs.preheader rather
      // than a dedicated column (no fork schema change). A non-empty field wins over any
      // preheader key typed into the raw attribs JSON. An empty field deletes the key only
      // when the server had one (the user actually cleared the field) — so a key managed
      // directly in the JSON tab survives saves where the field was simply left untouched.
      const preheader = (this.form.preheader || '').trim();
      if (preheader) {
        attribs = { ...(attribs || {}), preheader };
      } else if (attribs && this.data.attribs && this.data.attribs.preheader) {
        delete attribs.preheader;
      }
      this.form.attribs = attribs;

      switch (typ) {
        case 'create':
          this.createCampaign();
          break;
        case 'test':
          this.sendTest();
          break;
        default:
          this.updateCampaign();
          break;
      }
    },

    // Fork (evergreen) -- days field untouched: send the raw seconds back unchanged.
    sendDelaySecsPayload() {
      if (!this.form.evergreen) {
        return 0;
      }
      const days = Math.max(0, Number(this.form.sendDelayDays) || 0);
      const raw = Number(this.form.sendDelaySecs) || 0;
      if (Math.round(raw / 86400) === days) {
        return raw;
      }
      return days * 86400;
    },

    // Fork (evergreen) -- pause from the editor so a running welcome can be edited.
    pauseCampaign() {
      return this.$api.changeCampaignStatus(this.data.id, 'paused').then(() => {
        this.$utils.toast(this.$t('campaigns.statusChanged', { name: this.data.name, status: 'paused' }));
        return this.getCampaign(this.data.id);
      });
    },

    getCampaign(id) {
      return this.$api.getCampaign(id).then((data) => {
        this.data = data;
        this.form = {
          ...this.form,
          ...data,
          headersStr: JSON.stringify(data.headers, null, 4),
          archiveMetaStr: data.archiveMeta ? JSON.stringify(data.archiveMeta, null, 4) : '{}',
          attribsStr: data.attribs ? JSON.stringify(data.attribs, null, 4) : '{}',
          preheader: (data.attribs && data.attribs.preheader) || '',
          evergreen: !!data.evergreen,
          sendDelayDays: Math.round((data.sendDelaySecs || 0) / 86400),
          sendDelaySecs: data.sendDelaySecs || 0,

          // The structure that is populated by editor input event.
          content: {
            contentType: data.contentType,
            body: data.body,
            bodySource: data.bodySource,
            templateId: data.templateId,
          },
        };
        this.isAttachFieldVisible = this.form.media.length > 0;

        // Re-derive against the loaded campaign, announcing a repoint if the stored From
        // disagrees with what its lists now say.
        this.brandFromLoadPending = true;
        this.syncBrandDerivation();

        this.form.media = this.form.media.map((f) => {
          if (!f.id) {
            return { ...f, filename: `❌ ${f.filename}` };
          }
          return f;
        });
      });
    },

    sendTest() {
      // Commit whatever is still sitting uncommitted in the test-address field. A b-taginput only
      // moves text into the bound array on Enter/comma/Tab/blur, so "type one address, click Send"
      // would otherwise post an empty recipient list. addTag() emits `input` synchronously, so
      // form.testEmails is up to date by the time the payload is built below, and it still honours
      // :before-adding (validateEmail) -- a malformed address is dropped and the server's
      // "no subscribers to target" error is shown rather than nothing happening.
      const ti = this.$refs.testEmails;
      if (ti && ti.newTag && ti.newTag.trim() !== '') {
        ti.addTag();
      }

      const data = {
        id: this.data.id,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        from_email: this.form.fromEmail,
        messenger: this.form.messenger,
        type: 'regular',
        headers: this.form.headers,
        tags: this.form.tags,
        // Always an object, never null: the server only overrides the stored attribs (and
        // so the preheader) when the request carries a non-null value, and a test send
        // should reflect the editor's current state — including a cleared preheader.
        attribs: this.form.attribs || {},
        template_id: this.form.content.templateId,
        content_type: this.form.content.contentType,
        body: this.form.content.body,
        altbody: this.form.content.contentType !== 'plain' ? this.form.altbody : null,
        subscribers: this.form.testEmails,
        media: this.form.media.map((m) => m.id),
      };

      this.$api.testCampaign(data).then((d) => {
        this.$utils.toast(this.$t('campaigns.testSent'));
        this.$utils.showWarnings(d.warnings);
      });
      return false;
    },

    createCampaign() {
      const data = {
        archiveSlug: this.form.subject,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        from_email: this.form.fromEmail,
        content_type: this.form.content.contentType,
        messenger: this.form.messenger,
        type: 'regular',
        tags: this.form.tags,
        send_at: this.form.sendLater ? this.form.sendAtDate : null,
        headers: this.form.headers,
        attribs: this.form.attribs,
        media: this.form.media.map((m) => m.id),
        evergreen: !!this.form.evergreen,
        send_delay_secs: this.sendDelaySecsPayload(),
      };

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', hash: '#content', params: { id: d.id } });
      });
      return false;
    },

    // suppressWarnings: the editor Start flow saves and then changes status, and BOTH
    // responses carry the same computed render warnings — the save leg stays quiet there
    // so each Start surfaces them exactly once (from the status response, which also
    // carries the preheader nudge).
    async updateCampaign(typ, suppressWarnings = false) {
      const data = {
        archive_slug: this.form.archiveSlug,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        from_email: this.form.fromEmail,
        messenger: this.form.messenger,
        type: 'regular',
        tags: this.form.tags,
        send_at: this.form.sendLater ? this.form.sendAtDate : null,
        headers: this.form.headers,
        attribs: this.form.attribs,
        template_id: this.form.content.templateId,
        content_type: this.form.content.contentType,
        body: this.form.content.body,
        body_source: this.form.content.bodySource,
        altbody: this.form.content.contentType !== 'plain' ? this.form.altbody : null,
        archive: this.form.archive,
        archive_template_id: this.form.archiveTemplateId,
        archive_meta: this.form.archiveMeta,
        media: this.form.media.map((m) => m.id),
        evergreen: !!this.form.evergreen,
        send_delay_secs: this.sendDelaySecsPayload(),
      };

      let typMsg = 'globals.messages.updated';
      if (typ === 'start') {
        typMsg = 'campaigns.started';
      }

      if (!this.form.sendAtDate) {
        this.form.sendLater = false;
      }

      // This promise is used by startCampaign to first save before starting.
      return new Promise((resolve) => {
        this.$api.updateCampaign(this.data.id, data).then((d) => {
          this.data = d;
          this.form.archiveSlug = d.archiveSlug;
          this.form.attribsStr = d.attribs ? JSON.stringify(d.attribs, null, 4) : '{}';
          // Keep the field mirroring stored state so the cleared-field detection in
          // onSubmit compares against the right baseline on the next save.
          this.form.preheader = (d.attribs && d.attribs.preheader) || '';

          this.$utils.toast(this.$t(typMsg, { name: d.name }));
          if (!suppressWarnings) {
            this.$utils.showWarnings(d.warnings);
          }
          resolve();
        });
      });
    },

    onUpdateCampaignArchive() {
      if (this.isEditing && this.canEdit) {
        return;
      }

      const data = {
        archive: this.form.archive,
        archive_template_id: this.form.archiveTemplateId,
        archive_meta: JSON.parse(this.form.archiveMetaStr),
        archive_slug: this.form.archiveSlug,
      };

      this.$api.updateCampaignArchive(this.data.id, data).then((d) => {
        this.form.archiveSlug = d.archiveSlug;
      });
    },

    // Starts or schedule a campaign.
    startCampaign() {
      if (!this.canStart && !this.canSchedule) {
        return;
      }

      this.$utils.confirm(
        null,
        () => {
          // First save the campaign. Warnings are suppressed on this save leg —
          // the status response below re-computes and shows the same set once.
          this.updateCampaign(null, true).then(() => {
            // Then start/schedule it.
            let status = '';
            if (this.canStart) {
              status = 'running';
            } else if (this.canSchedule) {
              status = 'scheduled';
            } else {
              return;
            }

            this.$api.changeCampaignStatus(this.data.id, status).then((d) => {
              // Toasts are global, so they survive the route change below.
              this.$utils.showWarnings(d.warnings);
              this.$router.push({ name: 'campaigns' });
            });
          });
        },
      );
    },

    unscheduleCampaign() {
      this.$api.changeCampaignStatus(this.data.id, 'draft').then((d) => {
        this.data = d;
      });
    },
  },

  computed: {
    ...mapState(['serverConfig', 'loading', 'lists', 'templates', 'profile']),

    canManage() {
      return this.$can('campaigns:manage_all', 'campaigns:manage');
    },

    canSend() {
      return this.$can('campaigns:send');
    },

    canEdit() {
      return this.isNew
        || this.data.status === 'draft' || this.data.status === 'scheduled' || this.data.status === 'paused';
    },

    canSchedule() {
      return (this.data.status === 'draft' || this.data.status === 'paused') && (this.form.sendLater && this.form.sendAtDate);
    },

    canUnSchedule() {
      return this.data.status === 'scheduled';
    },

    canStart() {
      return (this.data.status === 'draft' || this.data.status === 'paused') && !this.form.sendLater;
    },

    canArchive() {
      return this.data.status !== 'cancelled' && this.data.type !== 'optin';
    },

    selectedLists() {
      if (this.selListIDs.length === 0 || !this.lists.results) {
        return [];
      }

      return this.lists.results.filter((l) => this.selListIDs.indexOf(l.id) > -1);
    },

    // The campaign API returns its lists as {id, name} ONLY -- `get-campaign` in
    // queries/campaigns.sql aggregates JSON_BUILD_OBJECT('id', …, 'name', …), with no tags. So the
    // tags have to come from the lists store, which App.vue loads with minimal=true (`SELECT *
    // FROM lists`, tags included). Resolving by id covers both a freshly-picked list and a loaded
    // campaign; deriving off form.lists directly would silently find no tags on load.
    resolvedLists() {
      const all = this.lists.results || [];
      return (this.form.lists || []).map((l) => all.find((x) => x.id === l.id) || l);
    },

    // Resolve the selected lists to one brand + From pair, or to the reason it cannot be done.
    // Returns { error, brand, fromEmail, unmapped }.
    brandDerivation() {
      const err = (error) => ({
        error, brand: null, fromEmail: null, unmapped: false,
      });

      const mapped = [];
      const halfTagged = [];

      this.resolvedLists.forEach((l) => {
        const tags = l.tags || [];
        const brandTag = tags.find((t) => t.startsWith(BRAND_TAG_PREFIX));
        const fromTag = tags.find((t) => t.startsWith(FROM_TAG_PREFIX));

        if (!brandTag && !fromTag) {
          return;
        }

        // A list carrying one tag but not the other is a misconfiguration, not an unmapped list:
        // it is how a brand ends up attributed but wrongly addressed, or vice versa.
        if (!brandTag || !fromTag) {
          halfTagged.push(l.name);
          return;
        }

        mapped.push({
          name: l.name,
          brand: brandTag.slice(BRAND_TAG_PREFIX.length),
          fromEmail: fromTag.slice(FROM_TAG_PREFIX.length),
        });
      });

      if (halfTagged.length > 0) {
        return err(this.$t('campaigns.brandFromHalfTagged', { lists: halfTagged.join(', ') }));
      }

      // No tagged list at all -> the default mapping. Not an error: the internal seed list and the
      // bounce simulator are deliberately unmapped and must keep working.
      if (mapped.length === 0) {
        return {
          error: null,
          brand: DEFAULT_BRAND,
          fromEmail: this.serverConfig.from_email,
          unmapped: true,
        };
      }

      // Multi-brand campaigns are blocked by decision, not deferred -- a cross-brand send has no
      // valid single From. Name both brands rather than silently picking the first.
      const brands = [...new Set(mapped.map((m) => m.brand))];
      if (brands.length > 1) {
        return err(this.$t('campaigns.brandFromConflict', { brands: brands.join(', ') }));
      }

      const addrs = [...new Set(mapped.map((m) => m.fromEmail))];
      if (addrs.length > 1) {
        return err(this.$t('campaigns.brandFromAddressConflict', { addresses: addrs.join(', ') }));
      }

      if (!reBrandSlug.test(brands[0])) {
        return err(this.$t('campaigns.brandFromInvalidSlug', { brand: brands[0], list: mapped[0].name }));
      }

      return {
        error: null, brand: brands[0], fromEmail: addrs[0], unmapped: false,
      };
    },

    // Blocks save/start/schedule/continue. A test send is blocked too, via onSubmit: testing with
    // an unresolvable brand proves nothing about what the audience would receive.
    isBrandBlocked() {
      return this.brandDerivation.error !== null;
    },

    // True while the derived From differs from what is STORED on the campaign — i.e. while there
    // is a repoint the user has not saved yet. Computed rather than a sticky flag so it clears
    // itself the instant a save lands (updateCampaign refreshes this.data), instead of telling
    // someone to save a change they already saved. A notice that says "not yet applied" when it
    // has been applied is the same class of lie this feature exists to remove, and it trains
    // people to ignore the one message that matters.
    brandFromRepointed() {
      return this.isEditing
        && !this.brandDerivation.error
        && !!this.data.fromEmail
        && this.data.fromEmail !== this.form.fromEmail;
    },

    brandFromMessage() {
      if (this.brandDerivation.error) {
        return this.brandDerivation.error;
      }

      if (this.brandFromRepointed) {
        return this.$t('campaigns.brandFromRepointed', {
          from: this.data.fromEmail, to: this.form.fromEmail,
        });
      }

      if (this.brandDerivation.unmapped) {
        return this.$t('campaigns.brandFromDefault', { brand: this.brandDerivation.brand });
      }

      return this.$t('campaigns.brandFromDerived', { brand: this.brandDerivation.brand });
    },

    emailMessengers() {
      return ['email', ...this.serverConfig.messengers.filter((m) => m.startsWith('email-'))];
    },

    otherMessengers() {
      return this.serverConfig.messengers.filter((m) => m !== 'email' && !m.startsWith('email-'));
    },
  },

  beforeRouteLeave(to, from, next) {
    if (this.isUnsaved()) {
      this.$utils.confirm(this.$t('globals.messages.confirmDiscard'), () => next(true));
      return;
    }
    next(true);
  },

  watch: {
    selectedLists() {
      this.form.lists = this.selectedLists;
    },

    // Re-derive on every list change, and again when the lists store finishes loading (which is
    // what carries the tags -- see the resolvedLists computed).
    brandDerivation: {
      handler() {
        this.syncBrandDerivation();
        this.syncBrandTheme();
      },
      immediate: true,
    },

    // A campaign with no lists derives the same default mapping before and after load, so
    // brandDerivation never changes and its watcher never fires. Watching the store's arrival
    // covers that case.
    'lists.results': function watchListsResults() {
      this.syncBrandDerivation();
      this.syncBrandTheme();
    },

    // eslint-disable-next-line func-names
    'data.sendAt': function () {
      if (this.data.sendAt !== null) {
        this.form.sendLater = true;
        this.form.sendAtDate = dayjs(this.data.sendAt).toDate();
      } else {
        this.form.sendLater = false;
        this.form.sendAtDate = null;
      }
    },
  },

  mounted() {
    window.onbeforeunload = () => this.isUnsaved() || null;

    // Fill default form fields.
    this.form.fromEmail = this.serverConfig.from_email;

    // New campaign.
    const { id } = this.$route.params;
    if (id === 'new') {
      this.isNew = true;

      if (this.$route.query.list_id) {
        // Multiple list_id query params.
        let strIds = [];
        if (typeof this.$route.query.list_id === 'object') {
          strIds = this.$route.query.list_id;
        } else {
          strIds = [this.$route.query.list_id];
        }

        this.selListIDs = strIds.map((v) => parseInt(v, 10));
      }
    } else {
      const intID = parseInt(id, 10);
      if (intID <= 0 || Number.isNaN(intID)) {
        this.$utils.toast(this.$t('campaigns.invalid'));
        return;
      }

      this.isEditing = true;
    }

    // Get templates list.
    this.$api.getTemplates().then((data) => {
      if (data.length > 0) {
        if (!this.form.templateId) {
          const tpl = data.find((i) => i.isDefault === true);
          this.form.templateId = tpl.id;
        }
      }
    });

    // Fetch campaign.
    if (this.isEditing) {
      this.getCampaign(id).then(() => {
        if (this.$route.hash !== '') {
          this.activeTab = this.$route.hash.replace('#', '');
        }
        this.loadedAt = Date.now();
        // Editor mounts on data.id; offer the stash once it exists.
        this.$nextTick(() => this.offerDraftRestore());
      });
    } else {
      this.form.messenger = 'email';
    }

    this.$nextTick(() => {
      this.$refs.focus.focus();
    });

    this.$events.$on('campaign.update', () => {
      this.onSubmit('update');
    });

    // Fork (session expiry): the api interceptor dispatches this before redirecting to login.
    window.addEventListener('listmonk:session-expired', this.stashDraft);
  },

  beforeDestroy() {
    this.$events.$off('campaign.update');
    window.removeEventListener('listmonk:session-expired', this.stashDraft);
  },
});
</script>
