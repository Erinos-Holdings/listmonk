<template>
  <div class="richtext-editor" v-if="isRichtextReady">
    <tiny-mce v-model="computedValue" :disabled="disabled" :init="richtextConf" />

    <!-- Fork (emoji picker, integrations EMOJI-PICKER-SPEC D2): the toolbar's emoji button opens
      this component's picker; the pick goes in at TinyMCE's own selection. -->
    <emoji-picker-popover :open="isEmojiOpen" :anchor="emojiAnchor" @pick="onEmojiPick"
      @close="isEmojiOpen = false" />

    <b-modal scroll="keep" :width="1200" :aria-modal="true" :active.sync="isRichtextSourceVisible">
      <div>
        <section expanded class="modal-card-body preview">
          <code-editor lang="html" v-model="richTextSourceBody" key="richtext-source" />
        </section>
        <footer class="modal-card-foot has-text-right">
          <b-button @click="onFormatRichtextHTML">
            {{ $t('campaigns.formatHTML') }}
          </b-button>
          <b-button @click="() => { this.isRichtextSourceVisible = false; }">
            {{ $t('globals.buttons.close') }}
          </b-button>
          <b-button @click="onSaveRichTextSource" class="is-primary">
            {{ $t('globals.buttons.save') }}
          </b-button>
        </footer>
      </div>
    </b-modal>

    <b-modal scroll="keep" :width="750" :aria-modal="true" :active.sync="isInsertHTMLVisible">
      <div>
        <section expanded class="modal-card-body preview">
          <code-editor lang="html" v-model="insertHTMLSnippet" key="richtext-snippet" />
        </section>
        <footer class="modal-card-foot has-text-right">
          <b-button @click="onFormatRichtextHTMLSnippet">
            {{ $t('campaigns.formatHTML') }}
          </b-button>
          <b-button @click="() => { this.isInsertHTMLVisible = false; }">
            {{ $t('globals.buttons.close') }}
          </b-button>
          <b-button @click="onInsertHTML" class="is-primary">
            {{ $t('globals.buttons.insert') }}
          </b-button>
        </footer>
      </div>
    </b-modal>

    <!-- image picker -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isMediaVisible" :width="900">
      <div class="modal-card content" style="width: auto">
        <section expanded class="modal-card-body">
          <media is-modal @selected="onMediaSelect" />
        </section>
      </div>
    </b-modal>
  </div>
</template>

<script>
import { html } from 'js-beautify';
import { mapState } from 'vuex';

import TinyMce from '@tinymce/tinymce-vue';
import 'tinymce';
import 'tinymce/icons/default';
import 'tinymce/plugins/anchor';
import 'tinymce/plugins/autolink';
import 'tinymce/plugins/autoresize';
import 'tinymce/plugins/charmap';
import 'tinymce/plugins/colorpicker';
import 'tinymce/plugins/contextmenu';
import 'tinymce/plugins/emoticons';
import 'tinymce/plugins/emoticons/js/emojis';
import 'tinymce/plugins/fullscreen';
import 'tinymce/plugins/help';
import 'tinymce/plugins/hr';
import 'tinymce/plugins/image';
import 'tinymce/plugins/imagetools';
import 'tinymce/plugins/link';
import 'tinymce/plugins/lists';
import 'tinymce/plugins/paste';
import 'tinymce/plugins/searchreplace';
import 'tinymce/plugins/table';
import 'tinymce/plugins/textcolor';
import 'tinymce/plugins/visualblocks';
import 'tinymce/plugins/visualchars';
import 'tinymce/plugins/wordcount';
import 'tinymce/skins/ui/oxide/skin.css';
import 'tinymce/themes/silver';

import { colors, uris } from '../constants';
import Media from '../views/Media.vue';
import CodeEditor from './CodeEditor.vue';
import EmojiPickerPopover from './EmojiPickerPopover.vue';
// eslint-disable-next-line import/no-relative-packages
import { attributeMenu } from '../../email-builder/src/attributes';

// Map of listmonk language codes to corresponding TinyMCE language files.
const LANGS = {
  cs: 'cs',
  de: 'de',
  es: 'es_419',
  fr: 'fr_FR',
  it: 'it_IT',
  pl: 'pl',
  pt: 'pt_PT',
  'pt-BR': 'pt_BR',
  ro: 'ro',
  tr: 'tr',
};

// Fork (attribute picker, ATTRIBUTE-PICKER-SPEC D6): the mdi `code-braces` glyph as a TinyMCE
// icon (TinyMCE's default icon pack has no braces).
const CODE_BRACES_SVG = '<svg width="24" height="24" viewBox="0 0 24 24"><path d="'
  + 'M8,3A2,2 0 0,0 6,5V9A2,2 0 0,1 4,11H3V13H4A2,2 0 0,1 6,15V19A2,2 0 0,0 8,21H10V19H8V14'
  + 'A2,2 0 0,0 6,12A2,2 0 0,0 8,10V5H10V3M16,3A2,2 0 0,1 18,5V9A2,2 0 0,0 20,11H21V13H20A2,2 0 0,0 18,15V19'
  + 'A2,2 0 0,1 16,21H14V19H16V14A2,2 0 0,1 18,12A2,2 0 0,1 16,10V5H14V3H16Z"/></svg>';

const TRACK_LINK = 'trackLink';
const TRACK_SUFFIX = '@TrackLink';
const EMBED_IMAGE = 'embedImage';

export default {
  components: {
    Media,
    'tiny-mce': TinyMce,
    'code-editor': CodeEditor,
    EmojiPickerPopover,
  },

  props: {
    disabled: { type: Boolean, default: false },
    value: {
      type: String,
      default: '',
    },
  },

  data() {
    return {
      isPreviewing: false,
      isMediaVisible: false,
      isReady: false,
      isRichtextReady: false,
      isRichtextSourceVisible: false,
      isInsertHTMLVisible: false,
      insertHTMLSnippet: '',
      richtextConf: {},
      richTextSourceBody: '',
      contentType: '',
      isEmojiOpen: false,
      emojiAnchor: null,
    };
  },

  methods: {
    initRichtextEditor() {
      const { lang } = this.serverConfig;

      this.richtextConf = {
        init_instance_callback: () => { this.isReady = true; },
        urlconverter_callback: this.onEditorURLConvert,

        setup: (editor) => {
          editor.addShortcut('ctrl+s', 'Save content', () => {
            this.$events.$emit('campaign.update', {});
          });
          editor.addShortcut('f9', 'Preview', () => {
            this.$events.$emit('campaign.preview', {});
          });

          editor.on('init', () => {
            editor.focus();
            this.onEditorDialogOpen(editor);
          });

          // Custom HTML editor.
          editor.ui.registry.addButton('html', {
            icon: 'sourcecode',
            tooltip: 'Source code',
            onAction: this.onRichtextViewSource,
          });

          editor.ui.registry.addButton('insert-html', {
            icon: 'code-sample',
            tooltip: 'Insert HTML',
            onAction: this.onOpenInsertHTML,
          });

          // Fork (emoji picker, EMOJI-PICKER-SPEC D2): the same picker as the subject and
          // preheader, not TinyMCE's bundled emoticons plugin.
          this.emojiEditor = editor;
          editor.ui.registry.addButton('emoji', {
            icon: 'emoji',
            tooltip: this.$t('campaigns.insertEmoji'),
            onAction: () => this.onToggleEmoji(editor),
          });
          // Fork (attribute picker, integrations ATTRIBUTE-PICKER-SPEC D2/D5/D6): TinyMCE's own
          // menu of the roster's text group, inserted at its selection as the emoji is.
          editor.ui.registry.addIcon('code-braces', CODE_BRACES_SVG);
          editor.ui.registry.addMenuButton('attributes', {
            icon: 'code-braces',
            tooltip: this.$t('campaigns.insertAttribute'),
            fetch: (callback) => {
              callback(attributeMenu('text').items.map((it) => ({
                type: 'menuitem',
                text: this.attributeLabel(it),
                onAction: () => this.onAttributePick(editor, it.expr),
              })));
            },
          });
          // A click or Esc inside the editor iframe never reaches the page document.
          editor.on('mousedown', () => { this.isEmojiOpen = false; });
          editor.on('keydown', (e) => {
            if (e.key === 'Escape') this.isEmojiOpen = false;
          });

          editor.on('CloseWindow', () => {
            editor.selection.getNode().scrollIntoView(false);
          });
        },

        browser_spellcheck: true,
        min_height: 500,
        toolbar_sticky: true,
        entity_encoding: 'raw',
        convert_urls: true,
        relative_urls: false,
        remove_script_host: false,
        extended_valid_elements: 'img[*]',
        plugins: [
          'anchor', 'autoresize', 'autolink', 'charmap', 'emoticons', 'fullscreen',
          'help', 'hr', 'image', 'imagetools', 'link', 'lists', 'paste', 'searchreplace',
          'table', 'visualblocks', 'visualchars', 'wordcount',
        ],
        toolbar: `undo redo | formatselect styleselect fontsizeselect |
                  bold italic underline strikethrough forecolor backcolor subscript superscript |
                  alignleft aligncenter alignright alignjustify |
                  bullist numlist table image insert-html emoji attributes | outdent indent | link hr removeformat |
                  html fullscreen help`,
        fontsize_formats: '10px 11px 12px 14px 15px 16px 18px 24px 36px',
        skin: false,
        content_css: false,
        content_style: `
          body { font-family: 'Inter', sans-serif; font-size: 15px; }
          img { max-width: 100%; }
          img.img-float-left { float: left; margin: 0 1em 1em 0; }
          img.img-float-right { float: right; margin: 0 0 1em 1em; }
          a { color: ${colors.primary}; }
          table, td { border-color: #ccc;}
        `,

        language: LANGS[lang] || null,
        language_url: LANGS[lang] ? `${uris.static}/tinymce/lang/${LANGS[lang]}.js` : null,

        image_advtab: true,
        image_class_list: [
          { title: 'None', value: '' },
          { title: 'Float left', value: 'img-float-left' },
          { title: 'Float right', value: 'img-float-right' },
        ],

        file_picker_types: 'image',
        file_picker_callback: (callback) => {
          this.isMediaVisible = true;
          this.imageCallack = callback;
        },
      };

      this.isRichtextReady = true;
    },

    onEditorURLConvert(url) {
      return url;
    },

    onRichtextViewSource() {
      this.richTextSourceBody = this.computedValue;
      this.isRichtextSourceVisible = true;
    },

    onOpenInsertHTML() {
      this.isInsertHTMLVisible = true;
    },

    // Fork (emoji picker): the toolbar button toggles the picker under itself.
    onToggleEmoji(editor) {
      if (this.disabled) return;
      if (this.isEmojiOpen) {
        this.isEmojiOpen = false;
        return;
      }
      const container = editor.getContainer();
      const label = this.$t('campaigns.insertEmoji');
      const button = container && Array.from(container.querySelectorAll('[aria-label]'))
        .find((el) => el.getAttribute('aria-label') === label);
      this.emojiAnchor = button || container;
      this.isEmojiOpen = true;
    },

    // The pick is exactly the picker's unicode (D5/I2), inserted at TinyMCE's selection.
    onEmojiPick(unicode) {
      this.isEmojiOpen = false;
      const editor = this.emojiEditor;
      if (!editor || this.disabled) return;
      editor.focus();
      editor.insertContent(unicode);
      this.computedValue = editor.getContent();
    },

    // Fork (attribute picker): the expression goes in at TinyMCE's selection; entity_encoding
    // 'raw' keeps its quotes as `"`.
    onAttributePick(editor, expr) {
      if (!editor || !expr || this.disabled) return;
      editor.focus();
      editor.insertContent(expr);
      this.computedValue = editor.getContent();
    },

    // D8: the roster's ids keyed into i18n (`first_name` -> `attributes.firstName`); the roster's
    // English label when a locale lacks the key.
    attributeLabel(item) {
      const key = `attributes.${item.id.replace(/_([a-z])/g, (m, c) => c.toUpperCase())}`;
      return this.$te(key) ? this.$t(key) : item.label;
    },

    onInsertHTML() {
      this.isInsertHTMLVisible = false;
      window.tinymce.editors[0].execCommand('mceInsertContent', false, this.insertHTMLSnippet);
      this.insertHTMLSnippet = '';
    },

    onFormatRichtextHTML() {
      this.richTextSourceBody = this.beautifyHTML(this.richTextSourceBody);
    },

    onFormatRichtextHTMLSnippet() {
      this.insertHTMLSnippet = this.beautifyHTML(this.insertHTMLSnippet);
    },

    onSaveRichTextSource() {
      this.computedValue = this.richTextSourceBody;
      window.tinymce.editors[0].setContent(this.computedValue);
      this.richTextSourceBody = '';
      this.isRichtextSourceVisible = false;
    },

    onEditorDialogOpen(editor) {
      const ed = editor;
      const oldEd = ed.windowManager.open;

      ed.windowManager.open = (t, r) => {
        const data = t.initialData || {};
        const isLink = data.url && 'anchor' in data;
        const isImage = data.src && !isLink;

        if (!isLink && !isImage) {
          return oldEd.call(ed.windowManager, t, r);
        }

        const { onSubmit } = t;
        const checkbox = isLink ? { type: 'checkbox', name: TRACK_LINK, label: 'Track link?' } : { type: 'checkbox', name: EMBED_IMAGE, label: this.$t('media.embed') };
        const spec = { ...t, body: this.withDialogCheckbox(t.body, checkbox) };

        if (isLink) {
          const cleanURL = (data.url.value || '').replace(/@TrackLink$/, '');
          const checked = data.url.value !== cleanURL
            || (!cleanURL && JSON.parse(localStorage.getItem(TRACK_LINK) || 'false'));
          spec.initialData = { ...data, [TRACK_LINK]: checked, url: { ...data.url, value: cleanURL } };
          spec.onSubmit = (api) => {
            const d = api.getData();
            const shouldTrack = Boolean(d[TRACK_LINK]);
            const url = (d.url.value || '').replace(/@TrackLink$/, '');
            localStorage.setItem(TRACK_LINK, JSON.stringify(shouldTrack));
            if (shouldTrack && /^https?:\/\//i.test(url)) {
              api.setData({ url: { ...d.url, value: `${url}${TRACK_SUFFIX}` } });
            }
            onSubmit(api);
          };
        } else {
          const img = this.getSelectedImage(ed);
          spec.initialData = { ...data, [EMBED_IMAGE]: Boolean(img && img.hasAttribute('data-embed')) };
          spec.onSubmit = (api) => {
            const d = api.getData();
            const shouldEmbed = d[EMBED_IMAGE] === true || d[EMBED_IMAGE] === 'true';

            onSubmit(api);

            // Apply 'embed' attr.
            const node = (img && ed.getBody().contains(img)) ? img : this.getSelectedImage(ed);
            if (!node) {
              return;
            }
            if (shouldEmbed) {
              ed.dom.setAttrib(node, 'data-embed', 'true');
            } else {
              node.removeAttribute('data-embed');
            }
            ed.fire('change');
            ed.save();
            this.computedValue = ed.getContent();
          };
        }

        return oldEd.call(ed.windowManager, spec, r);
      };
    },

    withDialogCheckbox(body, checkbox) {
      if (body.type === 'tabpanel') {
        return {
          ...body,
          tabs: body.tabs.map((tab) => (
            tab.name === 'general' || tab.title === 'General'
              ? { ...tab, items: [...tab.items, checkbox] }
              : tab
          )),
        };
      }
      return { ...body, items: [...body.items, checkbox] };
    },

    getSelectedImage(editor) {
      const node = editor.selection.getNode();
      if (!node) {
        return null;
      }
      if (node.nodeName === 'IMG') {
        return node;
      }
      const figure = editor.dom.getParent(node, 'figure.image');
      return figure ? figure.querySelector('img') : null;
    },

    onMediaSelect(media) {
      this.imageCallack(media.url);
    },

    beautifyHTML(str) {
      // Pad all tags with linebreaks.
      let s = this.trimLines(str.replace(/(<(?!(\/)?a|span)([^>]+)>)/gi, '\n$1\n'), true);
      // Remove extra linebreaks.
      s = s.replace(/\n+/g, '\n');

      try {
        s = html(s).trim();
      } catch (error) {
        // eslint-disable-next-line no-console
        console.log('error formatting HTML', error);
      }

      return s;
    },

    trimLines(str, removeEmptyLines) {
      const out = str.split('\n');
      for (let i = 0; i < out.length; i += 1) {
        const line = out[i].trim();
        if (removeEmptyLines) {
          out[i] = line;
        } else if (line === '') {
          out[i] = '';
        }
      }

      return out.join('\n').replace(/\n\s*\n\s*\n/g, '\n\n');
    },
  },

  mounted() {
    this.initRichtextEditor();
  },

  computed: {
    ...mapState(['serverConfig']),

    computedValue: {
      get() {
        return this.value;
      },
      set(newValue) {
        this.$emit('input', newValue);
      },
    },
  },
};
</script>
