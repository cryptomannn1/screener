<template>
  <div
    class="scr-table"
    :class="{ '-scrolled': scrolled }"
    tabindex="0"
    role="region"
    aria-label="Screener table"
    @scroll.passive="scrolled = $event.target.scrollLeft > 0"
  >
    <table>
      <thead>
        <tr>
          <th
            v-for="column in columns"
            :key="column.key"
            :class="{ '-end': isEndAligned(column), '-sortable': Boolean(column.value) }"
            :data-pin="column.pin"
            :style="column.width ? { minWidth: `${column.width}px` } : null"
            :aria-sort="ariaSort(column)"
            :title="column.title || null"
            @click="column.value && emit('sort', column.key)"
          >
            <span class="scr-th">
              {{ columnLabel(column, ctx) }}
              <ScreenerIcon
                v-if="column.value"
                class="scr-th__sort"
                :name="sort.key === column.key && sort.dir === 'asc' ? 'up' : 'down'"
              />
              <span v-if="sort.key === column.key && sort.dir === 'absdesc'" class="scr-th__abs">abs</span>
            </span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!rows.length">
          <td class="scr-table__empty" :colspan="columns.length">No instruments match the filters</td>
        </tr>
        <tr
          v-for="row in drawnRows"
          :key="row.instId"
          :aria-selected="row.instId === selectedId"
          @click="emit('select', row.instId)"
        >
          <td
            v-for="column in columns"
            :key="column.key"
            :class="{ '-end': isEndAligned(column) }"
            :data-pin="column.pin"
          >
            <span v-if="column.key === 'symbol'" class="scr-symbol">
              <button
                type="button"
                class="scr-star"
                :class="{ '-on': watchlist.has(row.instId) }"
                :aria-pressed="watchlist.has(row.instId)"
                :title="watchlist.has(row.instId) ? 'Remove from watchlist' : 'Add to watchlist'"
                @click.stop="emit('toggle-star', row.instId)"
              >
                <ScreenerIcon name="star" />
              </button>
              <span class="scr-symbol__text">
                <span class="scr-symbol__base">{{ row.symbol }}</span>
                <span class="scr-symbol__rest">{{ row.settle }} SWAP</span>
              </span>
            </span>
            <template v-else-if="column.key === 'spark'">
              <Sparkline v-if="row.spark?.length > 1" :values="row.spark" :base="sparkBase(row)" />
              <span v-else class="scr-na" title="Sparkline arrives with the next server version">—</span>
            </template>
            <button
              v-else-if="column.key === 'actions'"
              type="button"
              class="scr-btn -ghost -icon scr-row-menu"
              title="Row actions"
              aria-haspopup="menu"
              :aria-expanded="menuOpenFor === row.instId"
              @click.stop="emit('row-menu', row.instId, $event.currentTarget)"
            >
              <ScreenerIcon name="more" />
            </button>
            <span
              v-else-if="column.text"
              class="scr-text"
              :class="column.tone?.(row) ? '-' + column.tone(row) : ''"
              :title="row.name ?? null"
            >{{ column.text(row) ?? '—' }}</span>
            <ScreenerCell
              v-else
              :view="column.format(column.value(row, ctx), row, ctx)"
              :title="column.cellTitle?.(row) ?? null"
            />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup>
// The table itself: sticky 32px header separated by its background only, 38px
// rows separated by hover only, Symbol pinned left and the row menu pinned
// right. No borders between rows or columns.
import { computed, ref } from 'vue'

import { columnLabel, isEndAligned } from '../columns.js'
import ScreenerCell from './ScreenerCell.vue'
import ScreenerIcon from './ScreenerIcon.vue'
import Sparkline from './Sparkline.vue'

// Baseline of the sparkline: the price 24 h ago, recovered from the 24h change.
function sparkBase(row) {
  const change = row.chg?.['24h']
  return Number.isFinite(change) && row.last > 0 ? row.last / (1 + change) : null
}

const props = defineProps({
  columns: { type: Array, required: true },
  rows: { type: Array, required: true },
  ctx: { type: Object, required: true },
  sort: { type: Object, required: true },
  selectedId: { type: String, default: '' },
  watchlist: { type: Set, required: true },
  menuOpenFor: { type: String, default: '' },
  /** Rows drawn at most; the status line says when the list is cut. */
  renderLimit: { type: Number, default: 600 },
})
const emit = defineEmits(['sort', 'select', 'toggle-star', 'row-menu'])

const scrolled = ref(false)
const drawnRows = computed(() => props.rows.slice(0, props.renderLimit))

function ariaSort(column) {
  if (props.sort.key !== column.key) return null
  return props.sort.dir === 'asc' ? 'ascending' : 'descending'
}
</script>

<style scoped>
.scr-table {
  flex: 1;
  min-height: 0;
  overflow: auto;
  margin-right: var(--space-2);
  scrollbar-gutter: stable;
  scrollbar-color: var(--border) var(--widget-background);
}
.scr-table:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
table {
  width: max-content;
  min-width: 100%;
  border-collapse: separate;
  border-spacing: 0;
}
th,
td {
  box-sizing: border-box;
  padding: 0 var(--space-8);
  vertical-align: middle;
  white-space: nowrap;
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
th[aria-sort] {
  color: var(--text-high-emphasis);
}
th.-end,
td.-end {
  text-align: right;
}
th.-end {
  padding-left: 22px;
}
.scr-th {
  position: relative;
  display: inline-block;
}
.scr-th__sort {
  position: absolute;
  left: calc(100% + var(--space-2));
  top: 1px;
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
.scr-th__abs {
  position: absolute;
  left: calc(100% + 16px);
  top: 2px;
  font-size: 10px;
  line-height: 12px;
  color: var(--text-low-emphasis);
}
th.-end .scr-th__abs {
  left: auto;
  right: calc(100% + 16px);
}
td {
  height: 38px;
  overflow: hidden;
  text-overflow: ellipsis;
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
}
td[data-pin] {
  z-index: var(--layer-raised);
  background: var(--widget-background);
}
th[data-pin] {
  z-index: calc(var(--layer-sticky) + 1);
}
[data-pin='left'] {
  left: 0;
}
[data-pin='right'] {
  right: 0;
}
tbody tr:hover td[data-pin] {
  background: var(--card-neutral-background);
}
tbody tr[aria-selected='true'] td[data-pin] {
  background: color-mix(in srgb, var(--primary-accent) 15%, var(--widget-background));
}
.scr-table.-scrolled [data-pin='left'] {
  box-shadow: 4px 0 6px -4px var(--table-pin-shadow);
}
.scr-table__empty {
  padding: var(--space-24);
  color: var(--text-mid-emphasis);
  text-align: center;
}
.scr-na {
  color: var(--text-low-emphasis);
}

.scr-symbol {
  display: flex;
  align-items: center;
  gap: var(--space-8);
}
.scr-symbol__text {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.scr-symbol__base {
  color: var(--text-high-emphasis);
  font-weight: 600;
}
.scr-symbol__rest {
  font-size: 10px;
  line-height: 12px;
  color: var(--text-low-emphasis);
}
.scr-star {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 16px;
  height: 16px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--icons-low-emphasis);
  opacity: 0;
  cursor: pointer;
}
.scr-star .scr-icon {
  width: 12px;
  height: 12px;
}
tr:hover .scr-star,
.scr-star.-on,
.scr-star:focus-visible {
  opacity: 1;
}
.scr-star.-on {
  color: var(--text-attention-accent);
}
.scr-star:hover {
  color: var(--text-high-emphasis);
}
.scr-row-menu {
  opacity: 0;
}
tr:hover .scr-row-menu,
.scr-row-menu[aria-expanded='true'],
.scr-row-menu:focus-visible {
  opacity: 1;
}
</style>
