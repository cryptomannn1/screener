<template>
  <div
    ref="wrap"
    class="scr-table"
    :class="{ '-scrolled': scrolled }"
    tabindex="0"
    role="region"
    aria-label="Screener table"
    @scroll.passive="onScroll"
    @mouseleave="hideTip"
  >
    <table :style="{ width: `${layout.table}px` }">
      <colgroup>
        <col v-for="(key, index) in columns" :key="key" :style="{ width: `${layout.widths[index]}px` }" />
      </colgroup>
      <thead ref="head">
        <tr>
          <th
            v-for="key in columns"
            :key="key"
            :data-key="key"
            :data-pin="COLUMNS[key].pin ? 'left' : null"
            :class="{ '-end': COLUMNS[key].end, '-sortable': Boolean(COLUMNS[key].value), '-filtered': filtered.has(key) }"
            :aria-sort="ariaSort(key)"
            :title="COLUMNS[key].title || null"
            @click="COLUMNS[key].value && emit('sort', key)"
          >
            <span class="scr-th">
              {{ COLUMNS[key].label }}
              <ScreenerIcon v-if="COLUMNS[key].value" class="scr-th__sort" :name="sortIcon(key)" />
            </span>
          </th>
        </tr>
      </thead>
      <tbody @mouseover="onHover">
        <tr v-if="!rows.length">
          <td class="scr-table__empty" :colspan="columns.length">Nothing matches</td>
        </tr>
        <tr v-for="row in drawnRows" :key="row.id" :data-id="row.id" :aria-selected="row.id === selectedId" @click="emit('select', row)">
          <td v-for="key in columns" :key="key" :data-key="key" :data-pin="COLUMNS[key].pin ? 'left' : null" :class="{ '-end': COLUMNS[key].end }">
            <span v-if="key === 'symbol'" class="scr-symbol">
              <b :title="row.name || null">{{ row.base }}</b>
              <!-- One fixed place per selected exchange: an empty place means the asset does not trade there. -->
              <span v-if="venues.length > 1" class="scr-symbol__venues">
                <span v-for="code in venues" :key="code" class="scr-symbol__slot" :title="row.byVenue[code] ? EXCHANGE_NAME[code] : null">
                  <VenueIcon v-if="row.byVenue[code]" :code="code" />
                </span>
              </span>
            </span>
            <template v-else-if="COLUMNS[key].price">
              <PriceCell v-if="isNum(row.mark)" :value="row.mark" :tick="tick" />
              <span v-else class="scr-na">—</span>
            </template>
            <template v-else>
              <template v-for="cell in [COLUMNS[key].cell(row)]" :key="0">
                <span v-if="!cell" class="scr-na">—</span>
                <span v-else-if="cell.lead" class="scr-level">
                  <span class="scr-level__px">{{ cell.lead }}</span><span :class="cell.tone ? `-${cell.tone}` : null">{{ cell.text }}</span>
                </span>
                <span v-else :class="cell.tone ? `-${cell.tone}` : null" :title="cell.title || null">{{ cell.text }}</span>
              </template>
            </template>
          </td>
        </tr>
      </tbody>
    </table>

    <!-- What a summed figure is made of: the same figure on each exchange that went into it. -->
    <Teleport to="body">
      <div v-if="tipRow" ref="tipEl" class="scr-tip" :style="{ left: `${tipAt.left}px`, top: `${tipAt.top}px` }">
        <template v-for="code in tipRow.venues" :key="code">
          <span :class="{ '-lead': !tipLead || tipLead === code }"><VenueIcon :code="code" />{{ EXCHANGE_NAME[code] }}</span>
          <b :class="{ '-dim': tipLead && tipLead !== code }">{{ VENUE_FIGURES[tip.key](tipRow.byVenue[code]) }}</b>
        </template>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
// The table: sticky 32px header separated by its background only, 38px rows
// separated by hover only, Symbol pinned left. No borders between rows or
// columns. Header cells sort on click and reorder by dragging.
//
// Column widths: the symbol and text columns keep theirs; every number column
// gets its minimum plus an equal share of what is left. So a few columns fill
// the widget, more columns narrow down to their minimum, and only then the
// table scrolls sideways.
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, useTemplateRef, watch } from 'vue'

import { COLUMNS, VENUE_FIGURES, VENUE_LEAD_COLUMNS } from '../columns.js'
import { useDragReorder } from '../composables/useDragReorder.js'
import { isNum } from '../format.js'
import { EXCHANGE_NAME } from '../model/constants.js'
import PriceCell from './PriceCell.vue'
import ScreenerIcon from './ScreenerIcon.vue'
import VenueIcon from './VenueIcon.vue'

const props = defineProps({
  /** Column keys in display order, Symbol first. */
  columns: { type: Array, required: true },
  rows: { type: Array, required: true },
  sort: { type: Object, required: true },
  /** Keys of the columns that carry a condition; their headers take the accent colour. */
  filtered: { type: Set, required: true },
  /** Selected exchange codes, in display order. */
  venues: { type: Array, required: true },
  selectedId: { type: String, default: '' },
  tick: { type: Number, default: 0 },
  /** Rows drawn at most. */
  renderLimit: { type: Number, default: 400 },
})
const emit = defineEmits(['sort', 'select', 'move', 'drag-start'])

const wrap = useTemplateRef('wrap')
const head = useTemplateRef('head')
const tipEl = useTemplateRef('tipEl')
const scrolled = ref(false)
const available = ref(0)
const drawnRows = computed(() => props.rows.slice(0, props.renderLimit))

/* ---- widths ---- */

function minWidth(key) {
  const column = COLUMNS[key]
  if (key === 'symbol') {
    const n = props.venues.length
    return 16 + 104 + (n > 1 ? 8 + n * 12 + (n - 1) * 4 : 0)
  }
  const base = column.width || (column.level ? 156 : column.end ? 100 : 116)
  return Math.ceil(Math.max(base, column.label.length * 6.6 + (column.end ? 36 : 34)))
}
const layout = computed(() => {
  const mins = props.columns.map(minWidth)
  const flexible = props.columns.map((key) => Boolean(COLUMNS[key].end))
  const sum = mins.reduce((a, b) => a + b, 0)
  const n = flexible.filter(Boolean).length
  const share = n ? Math.max(0, available.value - sum) / n : 0
  return {
    widths: mins.map((min, i) => +(min + (flexible[i] ? share : 0)).toFixed(2)),
    table: +(n ? Math.max(available.value, sum) : sum).toFixed(2),
  }
})

let observer = null
onMounted(() => {
  available.value = wrap.value.clientWidth
  if (typeof ResizeObserver === 'function') {
    observer = new ResizeObserver(() => (available.value = wrap.value.clientWidth))
    observer.observe(wrap.value)
  }
})
onBeforeUnmount(() => observer?.disconnect())

/* ---- header ---- */

function ariaSort(key) {
  if (props.sort.key !== key) return null
  return props.sort.dir === 'asc' ? 'ascending' : props.sort.dir === 'near' ? 'other' : 'descending'
}
function sortIcon(key) {
  if (props.sort.key !== key) return 'down'
  return props.sort.dir === 'asc' ? 'up' : props.sort.dir === 'near' ? 'near' : 'down'
}

useDragReorder(head, {
  selector: 'th[data-key]',
  axis: 'x',
  locked: ['symbol'],
  onStart: () => emit('drag-start'),
  onMove: (key, target, edge) => emit('move', key, target, edge),
})

/* ---- hover card ---- */

const tip = reactive({ id: null, key: null })
const tipAt = reactive({ left: 0, top: 0 })
const tipRow = computed(() => {
  if (!tip.id) return null
  const row = props.rows.find((candidate) => candidate.id === tip.id)
  return row && row.venues.length > 1 ? row : null
})
const tipLead = computed(() => (tipRow.value && VENUE_LEAD_COLUMNS.has(tip.key) ? tipRow.value.primary : null))

function hideTip() {
  tip.id = tip.key = null
}
function onHover(event) {
  const td = event.target.closest('td[data-key]')
  const id = td?.parentElement.dataset.id
  if (td && id && VENUE_FIGURES[td.dataset.key]) {
    tip.id = id
    tip.key = td.dataset.key
  } else hideTip()
}
// The card follows its cell and survives the redraw of a live tick.
async function placeTip() {
  if (!tipRow.value) return
  await nextTick()
  const td = wrap.value?.querySelector(`tr[data-id="${CSS.escape(tip.id)}"] td[data-key="${tip.key}"]`)
  if (!td || !tipEl.value) return hideTip()
  const box = td.getBoundingClientRect()
  const w = tipEl.value.offsetWidth
  const h = tipEl.value.offsetHeight
  tipAt.left = Math.max(8, Math.min(box.right - w, window.innerWidth - w - 8))
  tipAt.top = box.bottom + h + 6 > window.innerHeight ? box.top - h - 2 : box.bottom + 2
}
watch([() => tip.id, () => tip.key, tipRow], placeTip, { flush: 'post' })

function onScroll(event) {
  scrolled.value = event.target.scrollLeft > 0
  hideTip()
}

defineExpose({
  /** After a column is added: bring it into view. */
  scrollToEnd: () => nextTick(() => wrap.value && (wrap.value.scrollLeft = wrap.value.scrollWidth)),
  scrollToStart: () => wrap.value && wrap.value.scrollTo(0, 0),
})
</script>

<style scoped>
.scr-table {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  scrollbar-color: var(--border) var(--widget-background);
}
.scr-table:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
table {
  table-layout: fixed;
  border-collapse: separate;
  border-spacing: 0;
}
th,
td {
  box-sizing: border-box;
  padding: 0 var(--space-8);
  overflow: hidden;
  vertical-align: middle;
  white-space: nowrap;
  text-overflow: ellipsis;
}
th {
  position: sticky;
  top: 0;
  z-index: var(--layer-sticky);
  height: var(--space-32);
  background: var(--widget-header-background);
  color: var(--text-mid-emphasis);
  font-weight: 400;
  text-align: left;
  user-select: none;
}
th.-sortable {
  cursor: pointer;
}
th[data-key]:not([data-key='symbol']) {
  touch-action: none;
}
th[aria-sort] {
  color: var(--text-high-emphasis);
}
th.-filtered .scr-th {
  color: var(--primary-accent);
}
th[data-dragging] {
  color: var(--primary-accent);
  cursor: grabbing;
}
th[data-drop-edge='before'] {
  box-shadow: inset 2px 0 var(--primary-accent);
}
th[data-drop-edge='after'] {
  box-shadow: inset -2px 0 var(--primary-accent);
}
th.-end,
td.-end {
  text-align: right;
}
th.-end {
  padding-left: 22px;
}
/* a text column after a number column keeps clear of its neighbour */
th:not(.-end):not([data-pin]),
td:not(.-end):not([data-pin]):not(.scr-table__empty) {
  padding-left: var(--space-24);
}
.scr-th {
  position: relative;
  display: inline-block;
}
.scr-th__sort {
  position: absolute;
  left: calc(100% + var(--space-2));
  top: 2px;
  width: 12px;
  height: 12px;
  opacity: 0;
}
th.-end .scr-th__sort {
  left: auto;
  right: calc(100% + var(--space-2));
}
th:hover .scr-th__sort,
th[aria-sort] .scr-th__sort {
  opacity: 1;
}
td {
  height: 38px;
}
tbody tr {
  cursor: pointer;
}
tbody tr:hover td {
  background: var(--card-neutral-background);
}
tbody tr[aria-selected='true'] td {
  background: var(--card-primary-background-light);
}
[data-pin] {
  position: sticky;
  left: 0;
}
td[data-pin] {
  z-index: var(--layer-raised);
  background: var(--widget-background);
}
th[data-pin] {
  z-index: calc(var(--layer-sticky) + 1);
}
tbody tr:hover td[data-pin] {
  background: var(--card-neutral-background);
}
tbody tr[aria-selected='true'] td[data-pin] {
  background: color-mix(in srgb, var(--primary-accent) 15%, var(--widget-background));
}
.scr-table.-scrolled [data-pin] {
  box-shadow: 4px 0 6px -4px var(--table-pin-shadow);
}
.scr-table__empty {
  padding: var(--space-24);
  color: var(--text-mid-emphasis);
  text-align: center;
  cursor: default;
}
.scr-na,
.-dim {
  color: var(--text-low-emphasis);
}
.-pos {
  color: var(--long);
}
.-neg {
  color: var(--short);
}
.-mid {
  color: var(--text-mid-emphasis);
}

.scr-symbol {
  display: flex;
  align-items: center;
  gap: var(--space-8);
}
.scr-symbol b {
  min-width: 64px;
  max-width: 104px;
  overflow: hidden;
  color: var(--text-high-emphasis);
  font-weight: 600;
  text-overflow: ellipsis;
}
.scr-symbol__venues {
  display: inline-flex;
  gap: var(--space-4);
  color: var(--icons-low-emphasis);
}
tr:hover .scr-symbol__venues {
  color: var(--icons-mid-emphasis);
}
.scr-symbol__slot {
  display: block;
  width: 12px;
  height: 12px;
}
.scr-level {
  display: inline-flex;
  align-items: baseline;
  gap: var(--space-8);
  white-space: nowrap;
}
.scr-level__px {
  color: var(--text-high-emphasis);
}
</style>

<style>
/* Not scoped: the card is teleported to body. */
.scr-tip {
  position: fixed;
  z-index: var(--layer-tooltip, var(--layer-dropdown));
  display: grid;
  grid-template-columns: auto auto;
  gap: var(--space-4) var(--space-16);
  align-items: center;
  padding: var(--space-8) 10px;
  border-radius: var(--radius-8);
  background: var(--dropdown-background);
  color: var(--terminal-v2-muted);
  box-shadow: var(--dropdown-shadow);
  font-size: 12px;
  line-height: 16px;
  white-space: nowrap;
  pointer-events: none;
}
.scr-tip span {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
}
.scr-tip span.-lead {
  color: var(--terminal-v2-text);
}
.scr-tip b {
  color: var(--terminal-v2-text);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.scr-tip b.-dim {
  color: var(--terminal-v2-muted);
  font-weight: 400;
}
</style>
