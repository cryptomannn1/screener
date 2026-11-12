<template>
  <svg class="scr-venue" viewBox="0 0 12 12" aria-hidden="true" v-html="GLYPHS[code]" />
</template>

<script setup>
// Monochrome exchange marks, 12px, drawn in the current text colour. In the
// terminal these map onto its own exchange icons (see HANDOVER.md).
const GLYPHS = {
  okx: '<path fill="currentColor" d="M1 1h3.3v3.3H1zM7.7 1H11v3.3H7.7zM4.35 4.35h3.3v3.3h-3.3zM1 7.7h3.3V11H1zM7.7 7.7H11V11H7.7z"/>',
  bn: '<path fill="currentColor" d="M6 .5 7.7 2.2 6 3.9 4.3 2.2zM2.2 4.3 3.9 6 2.2 7.7.5 6zM9.8 4.3 11.5 6 9.8 7.7 8.1 6zM6 4.3 7.7 6 6 7.7 4.3 6zM6 8.1 7.7 9.8 6 11.5 4.3 9.8z"/>',
  bb: '<path fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" d="M1.6 1.5h3.3a2.1 2.1 0 0 1 0 4.2H1.6zM1.6 5.7h3.9a2.4 2.4 0 0 1 0 4.8H1.6z"/><path stroke="currentColor" stroke-width="1.5" stroke-linecap="round" d="M10.4 1.8v8.4"/>',
}

defineProps({
  /** Exchange code of the `/assets` payload: okx | bn | bb. */
  code: { type: String, required: true },
})
</script>

<style>
.scr-venue {
  display: block;
  flex: none;
  width: 12px;
  height: 12px;
}
</style>
