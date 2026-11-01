<template>
  <svg class="scr-icon" viewBox="0 0 16 16" aria-hidden="true" v-html="ICONS[name]" />
</template>

<script setup>
// The few glyphs the widget needs, inline so it has no asset pipeline. In the
// terminal these map onto `IconNew` names (see HANDOVER.md).
const STROKE = 'fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"'
const ICONS = {
  search: `<circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M10.5 10.5 14 14" ${STROKE}/>`,
  star: '<path fill="currentColor" d="M8 1.5l1.9 4.1 4.5.5-3.3 3 .9 4.4L8 11.3l-4 2.2.9-4.4-3.3-3 4.5-.5z"/>',
  filter: `<path d="M2 3h12l-4.5 5.5V13l-3-1.5V8.5z" ${STROKE}/>`,
  columns: `<path d="M2.5 2.5h11v11h-11zM6 2.5v11M10 2.5v11" ${STROKE}/>`,
  more: '<circle cx="3.5" cy="8" r="1.4" fill="currentColor"/><circle cx="8" cy="8" r="1.4" fill="currentColor"/><circle cx="12.5" cy="8" r="1.4" fill="currentColor"/>',
  close: `<path d="M4 4l8 8M12 4l-8 8" ${STROKE}/>`,
  check: `<path d="M3 8.5l3 3 7-7" ${STROKE}/>`,
  up: `<path d="M4 10l4-4 4 4" ${STROKE}/>`,
  down: `<path d="M4 6l4 4 4-4" ${STROKE}/>`,
  grip: '<path fill="currentColor" d="M2 3h12v1.5H2zM2 7.25h12v1.5H2zM2 11.5h12V13H2z"/>',
  theme: '<circle cx="8" cy="8" r="5.5" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M8 2.5v11A5.5 5.5 0 0 0 8 2.5z" fill="currentColor"/>',
  maximize: `<path d="M9.5 2.5h4v4M6.5 13.5h-4v-4M13.5 2.5 9 7M2.5 13.5 7 9" ${STROKE}/>`,
  gear: `<circle cx="8" cy="8" r="2" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4" ${STROKE}/>`,
}

defineProps({
  name: { type: String, required: true },
})
</script>
