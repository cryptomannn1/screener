<template>
  <div class="scr-toolbar">
    <label class="scr-search">
      <ScreenerIcon name="search" />
      <input
        class="scr-input"
        type="search"
        placeholder="Search symbols"
        aria-label="Search symbols, several separated by commas"
        autocomplete="off"
        :value="search"
        @input="emit('update:search', $event.target.value)"
      />
    </label>

    <!-- Only with a choice to make: a single built-in preset draws no tabs. -->
    <div v-if="presets.length > 1" class="scr-toolbar__tabs" role="group" aria-label="Presets">
      <button
        v-for="preset in presets"
        :key="preset.id"
        type="button"
        class="scr-tab"
        :aria-pressed="preset.id === activePreset"
        :title="preset.description"
        @click="emit('select-preset', preset.id)"
      >
        {{ preset.label }}
        <span class="scr-tab__count">{{ counts[preset.id] ?? 0 }}</span>
      </button>
    </div>

    <span class="scr-toolbar__spacer" />

    <div class="scr-seg" role="group" aria-label="Change period">
      <button
        v-for="option in PERIOD_OPTIONS"
        :key="option.id"
        type="button"
        :disabled="option.disabled"
        :aria-pressed="option.id === period"
        :title="option.title"
        @click="emit('update:period', option.id)"
      >
        {{ option.id }}
      </button>
    </div>

    <button
      type="button"
      class="scr-btn"
      aria-haspopup="dialog"
      :aria-expanded="openPopover === 'filters'"
      @click="emit('open-filters', $event.currentTarget)"
    >
      <ScreenerIcon name="filter" />
      Filters
      <span v-if="filtersCount" class="scr-count">{{ filtersCount }}</span>
    </button>
    <button
      type="button"
      class="scr-btn"
      aria-haspopup="dialog"
      :aria-expanded="openPopover === 'columns'"
      @click="emit('open-columns', $event.currentTarget)"
    >
      <ScreenerIcon name="columns" />
      Columns
    </button>
  </div>
</template>

<script setup>
import ScreenerIcon from './ScreenerIcon.vue'

// 5m and 15m need intraday candles the service does not keep yet.
const PERIOD_OPTIONS = [
  { id: '5m', disabled: true, title: 'Needs 5m candles, planned for v2' },
  { id: '15m', disabled: true, title: 'Needs 15m candles, planned for v2' },
  { id: '1h' },
  { id: '4h' },
  { id: '24h' },
]

defineProps({
  presets: { type: Array, required: true },
  counts: { type: Object, required: true },
  activePreset: { type: String, required: true },
  search: { type: String, default: '' },
  period: { type: String, required: true },
  filtersCount: { type: Number, default: 0 },
  openPopover: { type: String, default: null },
})

const emit = defineEmits([
  'select-preset',
  'update:search',
  'update:period',
  'open-filters',
  'open-columns',
])
</script>

<style scoped>
.scr-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  flex: none;
  flex-wrap: wrap;
  padding: var(--space-4) var(--space-8);
  border-bottom: 1px solid var(--border);
}
.scr-toolbar__spacer {
  flex: 1 1 0;
}
.scr-toolbar__tabs {
  display: flex;
  gap: var(--space-4);
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;
}
.scr-toolbar__tabs::-webkit-scrollbar {
  display: none;
}
.scr-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
  flex: none;
  height: var(--space-32);
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
}
.scr-search {
  position: relative;
}
.scr-search .scr-input {
  width: 180px;
  max-width: 40vw;
  padding-left: 28px;
}
.scr-search .scr-icon {
  position: absolute;
  left: var(--space-8);
  top: 7px;
  color: var(--icons-mid-emphasis);
  pointer-events: none;
}
.scr-seg {
  display: inline-flex;
  gap: var(--space-2);
  padding: var(--space-2);
  border-radius: var(--radius-8);
  background: var(--card-neutral-background);
}
.scr-seg button {
  height: 24px;
  padding: 0 var(--space-8);
  border: 0;
  border-radius: var(--radius-6);
  background: transparent;
  color: var(--text-mid-emphasis);
  font: inherit;
  cursor: pointer;
}
.scr-seg button[aria-pressed='true'] {
  background: var(--widget-background);
  color: var(--text-high-emphasis);
  font-weight: 500;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}
.scr-seg button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.scr-seg button:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
@container (max-width: 720px) {
  .scr-search .scr-input {
    width: 140px;
  }
}
</style>
