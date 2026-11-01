<template>
  <div class="scr-pop__group">Add a numeric filter</div>
  <form class="scr-filter-form" @submit.prevent="add">
    <select v-model="key" aria-label="Metric">
      <optgroup v-for="group in groups" :key="group.name" :label="group.name">
        <option v-for="column in group.columns" :key="column.key" :value="column.key">
          {{ columnLabel(column, ctx) }}
        </option>
      </optgroup>
    </select>
    <select v-model="op" aria-label="Condition">
      <option value=">=">≥</option>
      <option value="<=">≤</option>
    </select>
    <input
      ref="valueInput"
      v-model="raw"
      type="text"
      inputmode="decimal"
      :placeholder="valueHint"
      aria-label="Value"
      :title="valueTitle"
      :aria-invalid="invalid"
    />
    <button type="submit" class="scr-btn -primary">Add</button>
  </form>
  <div class="scr-pop__sep" />
  <div class="scr-pop__group">Quick filters</div>
  <button
    v-for="quick in QUICK_FILTERS"
    :key="quick.label"
    type="button"
    class="scr-pop__row"
    @click="emit('add', { ...quick.filter })"
  >
    {{ quick.label }}
  </button>
</template>

<script setup>
import { computed, ref, useTemplateRef } from 'vue'

import { GROUPS, NUMERIC_COLUMNS, columnLabel, findColumn } from '../columns.js'
import { parseFilterValue } from '../format.js'
import { QUICK_FILTERS } from '../presets.js'

defineProps({
  ctx: { type: Object, required: true },
})
const emit = defineEmits(['add'])

const key = ref(NUMERIC_COLUMNS[0].key)
const op = ref('>=')
const raw = ref('')
const invalid = ref(false)
const valueInput = useTemplateRef('valueInput')

// Percent metrics are typed in percent, money in USD with K / M / B.
const UNIT_HINTS = {
  pct: ['2 = 2%', 'Percent value: 2 means 2%'],
  usd: ['50M', 'USD, K / M / B allowed: 50M, 1.2B'],
  x: ['1.5', 'Multiple of the norm'],
  bps: ['3', 'Basis points'],
  days: ['30', 'Days'],
  price: ['100', 'Price in the quote currency'],
  num: ['1', 'Plain number'],
}
const valueHint = computed(() => UNIT_HINTS[findColumn(key.value).unit][0])
const valueTitle = computed(() => UNIT_HINTS[findColumn(key.value).unit][1])

const groups = computed(() =>
  GROUPS.map((name) => ({ name, columns: NUMERIC_COLUMNS.filter((column) => column.group === name) })).filter(
    (group) => group.columns.length,
  ),
)

function add() {
  const value = parseFilterValue(findColumn(key.value).unit, raw.value)
  if (value === null) {
    invalid.value = true
    valueInput.value?.focus()
    return
  }
  invalid.value = false
  emit('add', { key: key.value, op: op.value, value })
}
</script>

<style scoped>
.scr-filter-form {
  display: flex;
  align-items: center;
  gap: var(--space-6);
  flex-wrap: wrap;
  padding: var(--space-8);
}
.scr-filter-form select,
.scr-filter-form input {
  height: 28px;
  padding: 0 var(--space-8);
  border: 1px solid var(--dropdown-border);
  border-radius: var(--radius-8);
  background: var(--dropdown-hover);
  color: var(--terminal-v2-text);
}
.scr-filter-form input {
  width: 90px;
}
.scr-filter-form input[aria-invalid='true'] {
  border-color: var(--text-error-accent);
}
.scr-filter-form select:focus-visible,
.scr-filter-form input:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
</style>
