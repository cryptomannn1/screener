<template>
  <div class="scr-tabs" role="group" aria-label="Tables">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      type="button"
      class="scr-tab"
      :aria-pressed="tab.id === active"
      :title="tab.title"
      @click="emit('select', tab.id)"
    >
      {{ tab.name }}
      <span v-if="counts[tab.id] != null" class="scr-tab__count">{{ counts[tab.id] }}</span>
    </button>
  </div>
</template>

<script setup>
// Ready tables. The number is how many assets match the tab's conditions right
// now, within the chosen exchanges, class and search; no conditions, no number.
defineProps({
  tabs: { type: Array, required: true },
  active: { type: String, required: true },
  counts: { type: Object, required: true },
})
const emit = defineEmits(['select'])
</script>

<style scoped>
.scr-tabs {
  display: flex;
  gap: var(--space-4);
  flex: none;
  min-width: 0;
  padding: var(--space-6) var(--space-8) 0;
  overflow-x: auto;
  scrollbar-width: none;
}
.scr-tabs::-webkit-scrollbar {
  display: none;
}
.scr-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
  flex: none;
  height: 28px;
  padding: 0 var(--space-8);
  border: 0;
  border-radius: var(--radius-6);
  background: transparent;
  color: var(--text-mid-emphasis);
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}
.scr-tab:hover {
  color: var(--text-high-emphasis);
}
.scr-tab[aria-pressed='true'] {
  background: var(--card-neutral-background);
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-tab:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
.scr-tab__count {
  color: var(--text-low-emphasis);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}
</style>
