<template>
  <div ref="root" class="screener">
    <ScreenerTabs :tabs="TABS" :active="state.tab" :counts="tabCounts" @select="selectTab" />

    <ScreenerBar
      v-model:search="search"
      :exchanges-label="exchangesLabel"
      :class-label="classLabel"
      :phrases="phrases"
      :count-text="countText"
      :can-reset="filtersChanged"
      :columns-count="view.cols.length"
      :open="popover.name"
      :open-key="popover.key"
      @open="open"
      @remove="removeFilter"
      @reset="resetFilters"
    />

    <div class="screener__body">
      <p v-if="status === 'loading'" class="screener__state"><span class="screener__pulse" /> Loading assets</p>
      <p v-else-if="status === 'error'" class="screener__state" :title="error?.message">{{ sourceLabel }} unreachable, retrying</p>
      <ScreenerTable
        v-else
        ref="table"
        :columns="view.cols"
        :rows="visibleRows"
        :sort="view.sort"
        :filtered="filteredKeys"
        :venues="venuesOn"
        :selected-id="selectedId"
        :tick="tick"
        @sort="view.sort = nextSort(view.sort, $event)"
        @select="selectRow"
        @move="moveColumn"
        @drag-start="close"
      />
    </div>

    <PopoverSurface v-if="popover.anchor" :anchor="popover.anchor" role="dialog" :aria-label="POPOVER_LABELS[popover.name]" @close="close">
      <OptionList v-if="popover.name === 'exchanges'" multiple :options="exchangeOptions" @pick="toggleExchange" />
      <OptionList v-else-if="popover.name === 'class'" :options="classOptions" @pick="pickClass" />
      <AddFilterMenu v-else-if="popover.name === 'add'" :columns="freeColumns" @add="addFilter" @more="open('columns')" />
      <OptionList v-else-if="popover.name === 'operator'" :options="operatorOptions" @pick="pickOperator" />
      <ValueEditor
        v-else-if="popover.name === 'value'"
        :key="popover.key"
        :column-key="popover.key"
        :condition="view.nums[popover.key]"
        @change="(field, value) => (view.nums[popover.key][field] = value)"
        @quick="pickQuick"
        @done="close"
      />
      <OptionList v-else-if="popover.name === 'level'" :options="levelOptions" @pick="pickLevelMode" />
      <div v-else-if="popover.name === 'near'" class="screener__near">
        <span>Near is within</span>
        <span class="screener__seg" role="group" aria-label="Range of near">
          <button v-for="[value, label] in NEAR_RANGES" :key="value" type="button" :aria-pressed="view.near === value" @click="pickNear(value)">{{ label }}</button>
        </span>
      </div>
      <OptionList v-else-if="popover.name === 'sector'" :options="sectorOptions" @pick="pickSector" />
      <ColumnsPanel v-else-if="popover.name === 'columns'" :columns="view.cols" @toggle="toggleColumn" @remove="hideColumn" @move="moveColumn" @reset="resetTab" />
    </PopoverSurface>
  </div>
</template>

<script setup>
// The screener widget: one table of perpetual contracts over three exchanges.
// Data comes from `source` (see `api/`), the widget adds up the exchanges the
// user chose (`model/aggregate.js`), state lives in `settings` /
// `update:settings` so the host persists it, and clicking a row emits `select`.
import { computed, ref, useTemplateRef, watch } from 'vue'

import './screener.css'
import { COLUMNS, isFilterable } from './columns.js'
import AddFilterMenu from './components/AddFilterMenu.vue'
import ColumnsPanel from './components/ColumnsPanel.vue'
import OptionList from './components/OptionList.vue'
import PopoverSurface from './components/PopoverSurface.vue'
import ScreenerBar from './components/ScreenerBar.vue'
import ScreenerTable from './components/ScreenerTable.vue'
import ScreenerTabs from './components/ScreenerTabs.vue'
import ValueEditor from './components/ValueEditor.vue'
import { useAssets } from './composables/useAssets.js'
import { usePopover } from './composables/usePopover.js'
import { useScreenerState } from './composables/useScreenerState.js'
import { countLevel, isDraft, isFiltered, nextSort, numOn, numText, parseSearch, passes, sortRows } from './filters.js'
import { aggregate, classCounts, exchangeCounts, sectorCounts } from './model/aggregate.js'
import { CLASSES, EXCHANGES, LEVEL_MODES, NEAR_RANGES, OPERATORS, levelModeLabel, operatorLabel } from './model/constants.js'
import { FILTER_KEYS, TABS, cleanView, sameView } from './tabs.js'

const POPOVER_LABELS = {
  exchanges: 'Exchanges',
  class: 'Assets',
  add: 'Add filter',
  operator: 'Operator',
  value: 'Value',
  level: 'Level condition',
  near: 'Range of near',
  sector: 'Sector',
  columns: 'Columns',
}

const props = defineProps({
  /** `{ kind: 'server', baseUrl, pollMs, sse }`, `{ kind: 'static', data | load }`, or an object with start/stop. */
  source: { type: Object, default: () => ({ kind: 'server' }) },
  /** Persisted state; every change comes back through `update:settings` as a partial. */
  settings: { type: Object, default: () => ({}) },
  /** Ticker of the host's current instrument (`BTC`); highlights that row. */
  symbol: { type: String, default: '' },
})
const emit = defineEmits(['select', 'update:settings'])

const root = useTemplateRef('root')
const table = useTemplateRef('table')
const { assets, ts, tick, error, status, sourceLabel } = useAssets(() => props.source)
const { state, views, view, switchTab } = useScreenerState(
  () => props.settings,
  (partial) => emit('update:settings', partial),
)
const search = ref('')

/* ---- rows ---- */

const venuesOn = computed(() => EXCHANGES.map(([code]) => code).filter((code) => state.exchanges[code]))
const rows = computed(() => aggregate(assets.value, state.exchanges))
const scope = computed(() => ({ cls: state.cls, terms: parseSearch(search.value), ts: ts.value }))
const visibleRows = computed(() => sortRows(rows.value.filter((row) => passes(row, view.value, scope.value)), view.value, ts.value))
const filteredKeys = computed(() => new Set(view.value.cols.filter((key) => isFiltered(view.value, key))))

/** How many assets match each tab's conditions now; a tab without conditions has no number. */
const tabCounts = computed(() => {
  const counts = {}
  for (const tab of TABS) {
    const v = views[tab.id]
    if (!v.cols.some((key) => isFiltered(v, key))) counts[tab.id] = null
    else if (tab.id === state.tab) counts[tab.id] = visibleRows.value.length
    else counts[tab.id] = rows.value.filter((row) => passes(row, v, scope.value)).length
  }
  return counts
})
const countText = computed(() => {
  if (!view.value.filters.length) return ''
  const total = state.cls === 'all' ? rows.value.length : rows.value.filter((row) => row.cls === state.cls).length
  return `${visibleRows.value.length} of ${total}`
})

const selectedId = computed(() => {
  const ticker = props.symbol.toUpperCase()
  const match = ticker && rows.value.find((row) => row.base === ticker)
  return match ? match.id : state.selected
})
function selectRow(row) {
  state.selected = row.id
  emit('select', { key: row.id, symbol: row.base, name: row.name, cls: row.cls, primary: row.primary, venues: row.venues.slice() })
}

/* ---- popovers ---- */

const { popover, open, close } = usePopover(root, {
  beforeOpen: (key) => pruneDrafts(key),
  afterClose: () => pruneDrafts(null),
})

/** A phrase left without a value goes away when its popover closes. */
function pruneDrafts(keep) {
  for (const key of view.value.filters.slice()) if (key !== keep && isDraft(view.value, key)) dropFilter(key)
}

function selectTab(id) {
  close()
  if (switchTab(id)) table.value?.scrollToStart()
}

/* ---- exchanges and asset class ---- */

const exchangeTotals = computed(() => exchangeCounts(assets.value))
const exchangeOptions = computed(() =>
  EXCHANGES.map(([code, name]) => ({ value: code, label: name, venue: code, checked: state.exchanges[code], hint: exchangeTotals.value[code] })),
)
const exchangesLabel = computed(() =>
  venuesOn.value.length === EXCHANGES.length ? 'All' : EXCHANGES.filter(([code]) => state.exchanges[code]).map((e) => e[1]).join(', '),
)
function toggleExchange(code) {
  if (state.exchanges[code] && venuesOn.value.length === 1) return // one exchange always stays on
  state.exchanges[code] = !state.exchanges[code]
}

const classTotals = computed(() => classCounts(rows.value))
const classOptions = computed(() =>
  CLASSES.map(([value, label]) => ({
    value,
    label: value === 'all' ? 'All assets' : label,
    checked: state.cls === value,
    hint: classTotals.value[value] || 0,
    disabled: !classTotals.value[value],
  })),
)
const classLabel = computed(() => CLASSES.find((c) => c[0] === state.cls)[1])
function pickClass(value) {
  state.cls = value
  close()
}

const sectors = computed(() => sectorCounts(rows.value, state.cls))
// A class the chosen exchanges do not list, or a sector the chosen class does not have, cannot stay selected.
watch([rows, () => state.cls], () => {
  if (!rows.value.length) return
  if (state.cls !== 'all' && !classTotals.value[state.cls]) state.cls = 'all'
  if (view.value.sector && !sectors.value.some((s) => s[0] === view.value.sector)) dropFilter('sector')
})

/* ---- conditions ---- */

const phrases = computed(() => {
  const v = view.value
  return v.filters
    .filter((key) => v.cols.includes(key))
    .map((key) => {
      const name = COLUMNS[key].label
      if (v.levels[key]) {
        const parts = [{ pop: 'level', text: levelModeLabel(v.levels[key]).toLowerCase(), strong: true }]
        if (v.levels[key] === 'near') parts.push({ pop: 'near', text: NEAR_RANGES.find((r) => r[0] === v.near)[1], strong: true })
        return { key, name, parts }
      }
      if (key === 'sector') {
        return { key, name, word: 'is', parts: [{ pop: 'sector', text: v.sector || 'Select value', strong: Boolean(v.sector), empty: !v.sector }] }
      }
      const text = numText(v, key)
      return {
        key,
        name,
        parts: [
          { pop: 'operator', text: operatorLabel(v.nums[key].op) },
          { pop: 'value', text: text || 'Select value', strong: Boolean(text), empty: !text },
        ],
      }
    })
})

const freeColumns = computed(() =>
  view.value.cols.filter((key) => isFilterable(key) && !view.value.filters.includes(key)).map((key) => ({ key, label: COLUMNS[key].label })),
)

/** A new phrase: a number waits for its value, a level starts as "near", the sector waits for a choice. */
function addFilter(key) {
  const v = view.value
  if (!v.filters.includes(key)) v.filters.push(key)
  if (COLUMNS[key].level) {
    v.levels[key] = 'near'
    open('level', key)
  } else if (key === 'sector') {
    open('sector', key)
  } else {
    v.nums[key] = { op: '>=', v: null, v2: null }
    open('value', key)
  }
}

function dropFilter(key) {
  const v = view.value
  v.filters = v.filters.filter((k) => k !== key)
  delete v.nums[key]
  delete v.levels[key]
  if (key === 'sector') v.sector = ''
}

function removeFilter(key) {
  close()
  dropFilter(key)
}

const filtersSignature = (v) => JSON.stringify(FILTER_KEYS.map((key) => v[key]))
const filtersChanged = computed(() => {
  const v = view.value
  if (v.filters.some((key) => isDraft(v, key))) return true
  return filtersSignature(cleanView(v, state.tab)) !== filtersSignature(cleanView(null, state.tab))
})
/** Reset returns the conditions the tab came with; a column they need comes back with them. */
function resetFilters() {
  close()
  const shipped = cleanView(null, state.tab)
  const v = view.value
  for (const key of FILTER_KEYS) v[key] = shipped[key]
  for (const key of shipped.filters) if (!v.cols.includes(key)) v.cols.push(key)
}

const operatorOptions = computed(() => {
  const f = view.value.nums[popover.key]
  return f ? OPERATORS.map(([value, label]) => ({ value, label, checked: f.op === value })) : []
})
function pickOperator(op) {
  const key = popover.key
  const f = view.value.nums[key]
  f.op = op
  if (op !== 'between') f.v2 = null
  if (numOn(f) && op !== 'between') close()
  else open('value', key)
}
function pickQuick(op, v) {
  view.value.nums[popover.key] = { op, v, v2: null }
  close()
}

const levelOptions = computed(() => {
  const key = popover.key
  if (popover.name !== 'level' || !view.value.levels[key]) return []
  return LEVEL_MODES.map(([value, label]) => ({
    value,
    label,
    checked: view.value.levels[key] === value,
    hint: countLevel(rows.value, view.value, scope.value, key, value),
  }))
})
function pickLevelMode(mode) {
  view.value.levels[popover.key] = mode
  close()
}
function pickNear(value) {
  view.value.near = value
  close()
}

const sectorOptions = computed(() => sectors.value.map(([name, n]) => ({ value: name, label: name, hint: n, checked: view.value.sector === name })))
function pickSector(name) {
  view.value.sector = name
  close()
}

/* ---- columns ---- */

function toggleColumn(key) {
  if (view.value.cols.includes(key)) return hideColumn(key)
  if (!COLUMNS[key]) return
  view.value.cols.push(key)
  table.value?.scrollToEnd()
}
/** A hidden column takes its condition and its order with it: nothing may filter the table from out of sight. */
function hideColumn(key) {
  if (key === 'symbol') return
  const v = view.value
  v.cols = v.cols.filter((k) => k !== key)
  dropFilter(key)
  if (v.sort.key === key) v.sort = { key: null, dir: null }
}
function moveColumn(key, target, edge) {
  if (key === 'symbol' || target === 'symbol' || key === target) return
  const list = view.value.cols.filter((k) => k !== key)
  const at = list.indexOf(target)
  if (at < 0) return
  list.splice(edge === 'before' ? at : at + 1, 0, key)
  view.value.cols = list
}
/** Back to the columns, order and conditions this tab came with. */
function resetTab() {
  views[state.tab] = cleanView(null, state.tab)
}

defineExpose({
  /** True when the open tab differs from the one that ships: for a host that offers its own "reset". */
  isTabChanged: () => !sameView(cleanView(view.value, state.tab), cleanView(null, state.tab)),
  resetTab,
})
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

<style>
/* Not scoped: drawn inside the popover, which is teleported to body. */
.screener__near {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  padding: var(--space-4);
  color: var(--terminal-v2-muted);
  white-space: nowrap;
}
.screener__seg {
  display: inline-flex;
  gap: var(--space-2);
  padding: var(--space-2);
  border-radius: var(--radius-8);
  background: var(--dropdown-hover);
}
.screener__seg button {
  height: 24px;
  padding: 0 var(--space-8);
  border: 0;
  border-radius: var(--radius-6);
  background: transparent;
  color: var(--terminal-v2-muted);
  cursor: pointer;
}
.screener__seg button[aria-pressed='true'] {
  background: var(--dropdown-background);
  color: var(--terminal-v2-text);
  font-weight: 500;
}
</style>
