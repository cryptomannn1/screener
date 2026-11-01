<template>
  <span v-if="view.kind === 'na'" class="scr-na">—</span>
  <span v-else-if="view.kind === 'badge'" class="scr-cell-badge">
    {{ view.text }}
    <span v-if="view.badge" class="scr-badge" :class="`-${view.badge}`">{{ view.badge }}</span>
  </span>
  <span v-else-if="view.kind === 'bar'" class="scr-bar">
    <i><b :style="{ width: `${view.ratio * 100}%` }" /></i>
    {{ view.text }}
  </span>
  <span v-else-if="view.kind === 'stack'" class="scr-stack">
    <span class="scr-stack__main" :class="view.main.tone && `-${view.main.tone}`">{{ view.main.text }}</span>
    <span class="scr-stack__sub" :class="view.sub.tone && `-${view.sub.tone}`">{{ view.sub.text }}</span>
  </span>
  <span v-else :class="view.tone && `-${view.tone}`">{{ view.text }}</span>
</template>

<script setup>
// Draws one formatted metric (see `format.js` for the view shapes).
defineProps({
  view: { type: Object, required: true },
})
</script>

<style scoped>
.-pos {
  color: var(--long);
}
.-neg {
  color: var(--short);
}
.-mid {
  color: var(--text-mid-emphasis);
}
.-dim {
  color: var(--text-low-emphasis);
}
.scr-na {
  color: var(--text-low-emphasis);
}
.scr-stack {
  display: inline-flex;
  align-items: baseline;
  gap: var(--space-8);
  white-space: nowrap;
}
.scr-stack__main {
  color: var(--text-high-emphasis);
}
.scr-cell-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
}
.scr-badge {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 var(--space-6);
  border-radius: var(--radius-6);
  font-size: 10px;
  line-height: 12px;
  font-weight: 500;
}
.scr-badge.-squeeze {
  background: var(--card-primary-background-light);
  color: var(--primary-accent);
}
.scr-badge.-expansion {
  background: var(--card-error-background-light);
  color: var(--text-error-accent);
}
.scr-bar {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
}
.scr-bar i {
  position: relative;
  display: inline-block;
  width: 40px;
  height: 4px;
  overflow: hidden;
  border-radius: var(--space-2);
  background: var(--card-neutral-background);
}
.scr-bar b {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: var(--space-2);
  background: var(--long);
}
</style>
