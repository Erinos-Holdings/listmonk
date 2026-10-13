<template>
  <!-- Fork (emoji picker, integrations EMOJI-PICKER-SPEC D2/D8): one <emoji-picker> in a
    popover under its anchor. The element is created on the FIRST open (v-if) and kept
    afterwards (v-show), so a page where nobody picks never fetches the emoji data. -->
  <div v-if="created" v-show="open" ref="panel" class="emoji-picker-popover" :style="position">
    <emoji-picker class="light" :data-source="dataSource" @emoji-click="onEmojiClick" />
  </div>
</template>

<script>
// The relative path is deliberate (the CampaignPreview.vue / darkSim precedent): email-builder
// is a sibling workspace, not an installed dependency of the SPA.
// eslint-disable-next-line import/no-relative-packages
import { EMOJI_DATA_PATH } from '../../email-builder/src/emojiInsert';

// The picker's rendered size (8 columns at the library's default sizes) -- used only to keep
// the popover inside the viewport.
const PANEL_WIDTH = 360;
const PANEL_HEIGHT = 420;

export default {
  name: 'EmojiPickerPopover',

  props: {
    open: { type: Boolean, default: false },
    // The element the popover hangs under (the field's icon, the editor's toolbar). Clicks on
    // it are the caller's toggle, never an outside click.
    anchor: { type: null, default: null },
  },

  data() {
    return {
      created: false,
      position: {},
      dataSource: EMOJI_DATA_PATH,
    };
  },

  watch: {
    open(on) {
      if (on) {
        this.created = true;
        this.place();
        document.addEventListener('mousedown', this.onOutside, true);
        document.addEventListener('keydown', this.onKey, true);
        window.addEventListener('resize', this.close);
        window.addEventListener('scroll', this.close);
      } else {
        this.unlisten();
      }
    },
  },

  methods: {
    place() {
      if (!this.anchor || !this.anchor.getBoundingClientRect) {
        this.position = { top: '20%', left: '50%', transform: 'translateX(-50%)' };
        return;
      }
      const r = this.anchor.getBoundingClientRect();
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const left = Math.max(8, Math.min(r.right - PANEL_WIDTH, vw - PANEL_WIDTH - 8));
      const below = r.bottom + 4;
      const top = (below + PANEL_HEIGHT > vh && r.top - PANEL_HEIGHT - 4 > 0) ? r.top - PANEL_HEIGHT - 4 : below;
      this.position = { top: `${top}px`, left: `${left}px` };
    },

    onEmojiClick(e) {
      const unicode = e && e.detail ? e.detail.unicode : '';
      if (unicode) {
        this.$emit('pick', unicode);
      }
    },

    onOutside(e) {
      const { panel } = this.$refs;
      if (panel && panel.contains(e.target)) return;
      if (this.anchor && this.anchor.contains && this.anchor.contains(e.target)) return;
      this.close();
    },

    onKey(e) {
      if (e.key === 'Escape') {
        e.stopPropagation();
        this.close();
      }
    },

    close() {
      this.$emit('close');
    },

    unlisten() {
      document.removeEventListener('mousedown', this.onOutside, true);
      document.removeEventListener('keydown', this.onKey, true);
      window.removeEventListener('resize', this.close);
      window.removeEventListener('scroll', this.close);
    },
  },

  beforeDestroy() {
    this.unlisten();
  },
};
</script>

<style scoped>
.emoji-picker-popover {
  position: fixed;
  z-index: 100;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15);
  border-radius: 6px;
  background: #fff;
}
</style>
