<template>
  <div class="scr-addf">
    <label class="scr-pop__search">
      <ScreenerIcon name="search" />
      <input ref="input" v-model="query" type="search" placeholder="Find a column" aria-label="Find a column" autocomplete="off" @keydown.enter.prevent="found[0] && emit('add', found[0].key)" />
    </label>
    <div class="scr-addf__list">
      <div v-if="!columns.length" class="scr-pop__empty">Every column already has a condition</div>
      <div v-else-if="!found.length" class="scr-pop__empty">Nothing found</div>
      <button v-for="column in found" :key="column.key" type="button" class="scr-pop__row" @click="emit('add', column.key)">{{ column.label }}</button>
    </div>
    <div class="scr-pop__sep" />
    <button type="button" class="scr-pop__row -muted" @click="emit('more')">Another metric: add its column</button>
  </div>
</template>

<script setup>
// "Add filter" offers the columns of the table that have no condition yet.
// A metric that is not in the table is added as a column first.
import { computed, onMounted, ref, useTemplateRef } from 'vue'

import ScreenerIcon from './ScreenerIcon.vue'

const props = defineProps({
  /** `{ key, label }` of the free columns, in table order. */
  columns: { type: Array, required: true },
})
const emit = defineEmits(['add', 'more'])

const query = ref('')
const input = useTemplateRef('input')
const found = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? props.columns.filter((column) => column.label.toLowerCase().includes(q)) : props.columns
})

// On a touch screen focusing the field would raise the keyboard over the list.
onMounted(() => window.matchMedia?.('(hover: hover)').matches && input.value?.focus())
</script>

<style>
.scr-addf {
  width: 248px;
  max-width: calc(100vw - 32px);
}
.scr-addf__list {
  max-height: min(50vh, 340px);
  margin-top: var(--space-4);
  overflow: auto;
  overscroll-behavior: contain;
}
</style>
