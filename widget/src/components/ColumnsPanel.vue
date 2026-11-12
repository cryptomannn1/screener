<template>
  <div class="scr-cols">
    <div>
      <div class="scr-pop__group">In the table</div>
      <div ref="list" class="scr-cols__list">
        <div
          v-for="key in columns"
          :key="key"
          class="scr-cols__row"
          :data-key="key"
          :data-locked="key === 'symbol' ? '' : null"
          :title="key === 'symbol' ? 'Symbol always stays first' : 'Drag to reorder'"
        >
          <ScreenerIcon name="grip" class="scr-cols__grip" />
          <span class="scr-cols__name">{{ COLUMNS[key].label }}</span>
          <button v-if="key !== 'symbol'" type="button" class="scr-cols__remove" title="Remove from the table" :aria-label="`Remove ${COLUMNS[key].label}`" @click="emit('remove', key)">
            <ScreenerIcon name="close" />
          </button>
        </div>
      </div>
      <div class="scr-cols__foot">
        <button type="button" class="scr-btn -ghost" title="Back to the columns, order and conditions this tab came with" @click="emit('reset')">Reset</button>
      </div>
    </div>

    <div>
      <div class="scr-pop__group">Add a column</div>
      <label class="scr-pop__search">
        <ScreenerIcon name="search" />
        <input ref="input" v-model="query" type="search" placeholder="Find a metric" aria-label="Find a metric" autocomplete="off" />
      </label>
      <div class="scr-cols__catalog">
        <div v-if="!groups.length" class="scr-pop__empty">Nothing found</div>
        <template v-for="group in groups" :key="group.group">
          <div class="scr-pop__group">{{ group.group }}</div>
          <div v-for="item in group.items" :key="item.name" class="scr-cols__item">
            <div class="scr-cols__text">
              <b>{{ item.name }}</b>
              <span>{{ item.desc }}</span>
            </div>
            <div class="scr-cols__keys">
              <button
                v-for="[key, label] in item.keys"
                :key="key"
                type="button"
                class="scr-opt"
                :aria-pressed="shown.has(key)"
                :title="shown.has(key) ? 'Remove from the table' : 'Add to the table'"
                @click="emit('toggle', key)"
              >
                {{ label || (shown.has(key) ? 'Added' : 'Add') }}
              </button>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
// Columns: what is in the table (drag to reorder, cross to remove) next to
// what can be added. The catalogue is written for a newcomer: what the metric
// is in one line; periods are chips of one family.
import { computed, onMounted, ref, useTemplateRef } from 'vue'

import { CATALOG, COLUMNS } from '../columns.js'
import { useDragReorder } from '../composables/useDragReorder.js'
import ScreenerIcon from './ScreenerIcon.vue'

const props = defineProps({
  /** Column keys of the table in display order. */
  columns: { type: Array, required: true },
})
const emit = defineEmits(['toggle', 'remove', 'move', 'reset'])

const list = useTemplateRef('list')
const input = useTemplateRef('input')
const query = ref('')
const shown = computed(() => new Set(props.columns))
const groups = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return CATALOG
  return CATALOG.map((group) => ({
    group: group.group,
    items: group.items.filter((item) => `${group.group} ${item.name} ${item.desc}`.toLowerCase().includes(q)),
  })).filter((group) => group.items.length)
})

useDragReorder(list, {
  selector: '.scr-cols__row',
  axis: 'y',
  locked: ['symbol'],
  onMove: (key, target, edge) => emit('move', key, target, edge),
})

onMounted(() => window.matchMedia?.('(hover: hover)').matches && input.value?.focus())
</script>

<style>
.scr-cols {
  display: grid;
  grid-template-columns: 216px 380px;
  gap: 0 var(--space-4);
  align-items: start;
  max-width: calc(100vw - 32px);
}
.scr-cols > div {
  min-width: 0;
}
.scr-cols__list,
.scr-cols__catalog {
  max-height: min(60vh, 520px);
  overflow: auto;
  overscroll-behavior: contain;
}
.scr-cols__row {
  display: grid;
  grid-template-columns: 12px minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-8);
  height: 30px;
  padding: 0 var(--space-4) 0 var(--space-8);
  border-radius: var(--radius-4);
  cursor: grab;
  user-select: none;
  touch-action: none;
}
.scr-cols__row[data-locked] {
  color: var(--terminal-v2-muted);
  cursor: default;
}
.scr-cols__row:hover {
  background: var(--dropdown-hover);
}
.scr-cols__row[data-dragging] {
  background: var(--dropdown-hover);
  color: var(--primary-accent);
}
.scr-cols__row[data-drop-edge='before'] {
  box-shadow: inset 0 2px var(--primary-accent);
}
.scr-cols__row[data-drop-edge='after'] {
  box-shadow: inset 0 -2px var(--primary-accent);
}
.scr-cols__grip {
  width: 12px;
  height: 12px;
  color: var(--icons-low-emphasis);
}
.scr-cols__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.scr-cols__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-4);
  background: transparent;
  color: var(--icons-mid-emphasis);
  opacity: 0;
  cursor: pointer;
}
.scr-cols__row:hover .scr-cols__remove,
.scr-cols__remove:focus-visible {
  opacity: 1;
}
.scr-cols__remove:hover {
  color: var(--text-error-accent);
}
.scr-cols__remove .scr-icon {
  width: 12px;
  height: 12px;
}
.scr-cols__foot {
  display: flex;
  padding: var(--space-8) var(--space-4) var(--space-4);
}
.scr-cols__item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--space-4) var(--space-12);
  align-items: center;
  padding: var(--space-6) var(--space-8);
  border-radius: var(--radius-6);
}
.scr-cols__item:hover {
  background: var(--dropdown-hover);
}
.scr-cols__item:hover .scr-opt:not([aria-pressed='true']) {
  background: color-mix(in srgb, var(--terminal-v2-text) 9%, var(--dropdown-background));
}
.scr-cols__text b {
  display: block;
  color: var(--terminal-v2-text);
  font-weight: 500;
}
.scr-cols__text span {
  color: var(--terminal-v2-muted);
}
.scr-cols__keys {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
  max-width: 190px;
}
@media (max-width: 680px) {
  .scr-cols {
    grid-template-columns: minmax(0, 1fr);
    width: calc(100vw - 32px);
  }
  .scr-cols__list,
  .scr-cols__catalog {
    max-height: 32vh;
  }
  .scr-cols__keys {
    max-width: 150px;
  }
}
</style>
