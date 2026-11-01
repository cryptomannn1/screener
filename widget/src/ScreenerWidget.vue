<template>
  <div class="screener">
    <ScreenerToolbar
      :presets="presets"
      :counts="presetCounts"
      :active-preset="state.preset"
      :search="search"
      :period="state.period"
      :filters-count="state.filters.length"
      :open-popover="popover.kind"
      @select-preset="applyPreset"
      @update:search="search = $event"
      @update:period="patch({ period: $event })"
      @open-filters="toggle('filters', $event)"
      @open-columns="toggle('columns', $event)"
    />

    <FilterStrip
      v-if="filterChips.length || state.preset !== ALL_PRESET.id"
      :chips="filterChips"
      :sort-text="sortText"
      :can-save="presetChanged"
      :can-delete="Boolean(activePreset?.custom)"
      @remove="removeFilter"
      @save="savePreset"
      @delete="deletePreset"
    />

    <div class="screener__body">
      <p v-if="status === 'loading'" class="screener__state">
        <span class="screener__pulse" /> Loading instruments
      </p>
      <p v-else-if="status === 'error'" class="screener__state" :title="error?.message">
        {{ sourceLabel }} unreachable, retrying
      </p>
      <ScreenerTable
        v-else
        :columns="visibleColumns"
        :rows="visibleRows"
        :ctx="ctx"
        :sort="state.sort"
        :selected-id="selectedId"
        :watchlist="watchlist"
        :menu-open-for="popover.kind === 'row' ? popover.payload : ''"
        :render-limit="RENDER_LIMIT"
        @sort="cycleSort"
        @select="selectRow"
        @toggle-star="toggleStar"
        @row-menu="(instId, anchor) => toggle('row', anchor, instId)"
      />
    </div>

    <StatusLine
      :total="rows.length"
      :shown="visibleRows.length"
      :drawn="Math.min(visibleRows.length, RENDER_LIMIT)"
      :updated-at="meta.ts"
      :source-label="sourceLabel"
      :coverage-text="coverageText"
    />

    <PopoverSurface
      v-if="popover.kind"
      :anchor="popover.anchor"
      :role="popover.kind === 'row' ? 'menu' : 'dialog'"
      :aria-label="POPOVER_LABELS[popover.kind]"
      @close="close"
    >
      <FiltersPopover v-if="popover.kind === 'filters'" :ctx="ctx" @add="addFilter" />
      <ColumnsPopover
        v-else-if="popover.kind === 'columns'"
        :visible="visibleColumnKeys"
        :ctx="ctx"
        @toggle="toggleColumn"
        @show-all="patch({ columns: COLUMNS.map((column) => column.key) })"
        @reset="patch({ columns: DEFAULT_COLUMN_KEYS })"
      />
      <RowMenu
        v-else
        :inst-id="popover.payload"
        :starred="watchlist.has(popover.payload)"
        @action="rowAction(popover.payload, $event)"
      />
    </PopoverSurface>
  </div>
</template>

<script setup>
// The screener widget. Data comes from `source` (see `api/`), state lives in
// `settings` / `update:settings` so the host persists it, and clicking a row
// emits `select` with the instrument id for the host's link group.
import { computed, ref } from 'vue'

import './screener.css'
import { COLUMNS, DEFAULT_COLUMN_KEYS, FIXED_COLUMN_KEYS, columnLabel, findColumn } from './columns.js'
import ColumnsPopover from './components/ColumnsPopover.vue'
import FilterStrip from './components/FilterStrip.vue'
import FiltersPopover from './components/FiltersPopover.vue'
import PopoverSurface from './components/PopoverSurface.vue'
import RowMenu from './components/RowMenu.vue'
import ScreenerTable from './components/ScreenerTable.vue'
import ScreenerToolbar from './components/ScreenerToolbar.vue'
import StatusLine from './components/StatusLine.vue'
import { usePopover } from './composables/usePopover.js'
import { useScreenerData } from './composables/useScreenerData.js'
import { useScreenerRows } from './composables/useScreenerRows.js'
import { useScreenerSettings } from './composables/useScreenerSettings.js'
import { formatFilterValue } from './format.js'
import { ALL_PRESET, PRESETS, sameFilters } from './presets.js'

const RENDER_LIMIT = 600
const POPOVER_LABELS = { filters: 'Filters', columns: 'Columns', row: 'Row actions' }
const SORT_WORDS = { asc: 'ascending', desc: 'descending', absdesc: 'by size' }

const props = defineProps({
  /** `{ kind: 'server', baseUrl, pollMs, sse }`, `{ kind: 'mock' }`, or an object with start/stop. */
  source: { type: Object, default: () => ({ kind: 'mock' }) },
  /** Persisted state; every change comes back through `update:settings` as a partial. */
  settings: { type: Object, default: () => ({}) },
  /** The host's current instrument (link group); highlights that row. */
  instrumentId: { type: String, default: '' },
})
const emit = defineEmits(['select', 'trade', 'update:settings', 'notify'])

const { rows, meta, error, status, sourceLabel } = useScreenerData(() => props.source)
const { state, patch } = useScreenerSettings(
  () => props.settings,
  (partial) => emit('update:settings', partial),
)
const { popover, toggle, close } = usePopover()
const search = ref('')

const ctx = computed(() => ({
  period: state.period,
  benchmark: meta.value.benchmark.split('-')[0] || 'BTC',
}))
const { visibleRows, presetCounts, activePreset, watchlist } = useScreenerRows(rows, state, search, ctx)

const presets = computed(() => [...PRESETS, ...state.customPresets])
const presetChanged = computed(
  () =>
    Boolean(activePreset.value) &&
    !(sameFilters(activePreset.value.filters, state.filters) && sameFilters(activePreset.value.sort, state.sort)),
)
const selectedId = computed(() => props.instrumentId || state.selected)

const visibleColumnKeys = computed(() => new Set([...state.columns, ...FIXED_COLUMN_KEYS]))
const visibleColumns = computed(() => COLUMNS.filter((column) => visibleColumnKeys.value.has(column.key)))

const filterChips = computed(() =>
  state.filters.map((filter) => {
    const column = findColumn(filter.key)
    return {
      label: column ? columnLabel(column, ctx.value) : filter.key,
      op: filter.op === '>=' ? '≥' : '≤',
      value: formatFilterValue(column?.unit, filter.value),
    }
  }),
)
const sortText = computed(() => {
  const column = findColumn(state.sort.key)
  return column ? `sorted by ${columnLabel(column, ctx.value)} ${SORT_WORDS[state.sort.dir]}` : ''
})

/** "Candles 517/517, books 517/517, OI Δ 0/517, flow 0/517"; a family every row has is left out. */
const coverageText = computed(() => {
  const coverage = meta.value.coverage
  const total = rows.value.length
  if (!coverage || !total) return ''
  const flow = rows.value.filter((row) => row.cov?.flow).length
  const families = [
    ['Candles', coverage.hourly],
    ['daily', coverage.daily],
    ['books', coverage.book],
    ['OI Δ', coverage.oiHist],
    ['flow', flow],
  ]
  return families
    .filter(([, count]) => (count ?? 0) < total)
    .map(([name, count]) => `${name} ${count ?? 0}/${total}`)
    .join(', ')
})

/* ---- presets and filters ---- */

function applyPreset(id) {
  const preset = presets.value.find((candidate) => candidate.id === id) ?? ALL_PRESET
  patch({
    preset: preset.id,
    filters: preset.filters.map((filter) => ({ ...filter })),
    sort: { ...preset.sort },
  })
}

function addFilter(filter) {
  // One condition per metric and direction: a new value replaces the old one.
  const others = state.filters.filter((f) => !(f.key === filter.key && f.op === filter.op))
  patch({ filters: [...others, filter] })
  close()
}

function removeFilter(index) {
  patch({ filters: state.filters.filter((_, i) => i !== index) })
}

function savePreset(label) {
  const preset = {
    id: `custom-${Date.now()}`,
    label,
    custom: true,
    filters: state.filters.map((filter) => ({ ...filter })),
    sort: { ...state.sort },
    description: 'Saved preset',
  }
  patch({ customPresets: [...state.customPresets, preset], preset: preset.id })
}

function deletePreset() {
  patch({ customPresets: state.customPresets.filter((preset) => preset.id !== state.preset) })
  applyPreset(ALL_PRESET.id)
}

/* ---- sort and columns ---- */

/** desc → asc → by size (signed metrics only) → desc. A new column starts descending. */
function cycleSort(key) {
  const current = state.sort
  if (current.key !== key) {
    patch({ sort: { key, dir: 'desc' } })
    return
  }
  const signed = Boolean(findColumn(key)?.signed)
  const next = current.dir === 'desc' ? 'asc' : current.dir === 'asc' && signed ? 'absdesc' : 'desc'
  patch({ sort: { key, dir: next } })
}

function toggleColumn(key) {
  const columns = state.columns.includes(key)
    ? state.columns.filter((candidate) => candidate !== key)
    : [...state.columns, key]
  patch({ columns })
}

/* ---- rows ---- */

function selectRow(instId) {
  patch({ selected: instId })
  emit('select', instId)
}

function toggleStar(instId) {
  const watchlist = state.watchlist.includes(instId)
    ? state.watchlist.filter((candidate) => candidate !== instId)
    : [...state.watchlist, instId]
  patch({ watchlist })
}

function rowAction(instId, action) {
  close()
  if (action === 'select') selectRow(instId)
  else if (action === 'trade') {
    selectRow(instId)
    emit('trade', instId)
  } else if (action === 'star') toggleStar(instId)
  else if (action === 'copy') {
    navigator.clipboard
      ?.writeText(instId)
      .then(() => emit('notify', `Copied ${instId}`))
      .catch(() => emit('notify', instId))
  }
}
</script>

<style scoped>
.screener {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  overflow: hidden;
  container-type: inline-size;
  background: var(--widget-background);
  color: var(--text-high-emphasis);
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}
.screener :deep(*),
.screener :deep(*::before),
.screener :deep(*::after) {
  box-sizing: border-box;
}
.screener__body {
  position: relative;
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.screener__state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-8);
  margin: 0;
  padding: var(--space-24);
  color: var(--text-mid-emphasis);
  text-align: center;
}
.screener__pulse {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-circle);
  background: var(--text-low-emphasis);
  animation: screener-breathe 1s infinite alternate;
}
@keyframes screener-breathe {
  to {
    opacity: 0.3;
  }
}
@media (prefers-reduced-motion: reduce) {
  .screener__pulse {
    animation: none;
  }
}
</style>
