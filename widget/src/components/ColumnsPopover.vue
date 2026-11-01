<template>
  <div class="scr-columns">
    <div v-for="group in groups" :key="group.name">
      <div class="scr-pop__group">{{ group.name }}</div>
      <button
        v-for="column in group.columns"
        :key="column.key"
        type="button"
        class="scr-pop__row"
        role="menuitemcheckbox"
        :aria-checked="visible.has(column.key)"
        :title="column.title"
        @click="emit('toggle', column.key)"
      >
        <span class="scr-columns__box">
          <ScreenerIcon v-if="visible.has(column.key)" name="check" />
        </span>
        {{ columnLabel(column, ctx) }}
      </button>
    </div>
  </div>
  <div class="scr-pop__sep" />
  <div class="scr-pop__foot">
    <button type="button" class="scr-btn -ghost" @click="emit('show-all')">Show all</button>
    <button type="button" class="scr-btn -ghost" @click="emit('reset')">Reset to default</button>
  </div>
</template>

<script setup>
import { computed } from 'vue'

import { COLUMNS, FIXED_COLUMN_KEYS, GROUPS, columnLabel } from '../columns.js'
import ScreenerIcon from './ScreenerIcon.vue'

defineProps({
  visible: { type: Set, required: true },
  ctx: { type: Object, required: true },
})
const emit = defineEmits(['toggle', 'show-all', 'reset'])

const groups = computed(() =>
  GROUPS.map((name) => ({
    name,
    columns: COLUMNS.filter((column) => column.group === name && !FIXED_COLUMN_KEYS.includes(column.key)),
  })).filter((group) => group.columns.length),
)
</script>

<style scoped>
.scr-columns {
  display: grid;
  grid-template-columns: repeat(3, minmax(150px, 1fr));
  gap: 0 var(--space-8);
  max-height: 60vh;
  overflow: auto;
}
.scr-columns__box {
  display: grid;
  place-items: center;
  flex: none;
  width: 16px;
  height: 16px;
  border: 1px solid var(--dropdown-border);
  border-radius: var(--radius-4);
}
.scr-pop__row[aria-checked='true'] .scr-columns__box {
  border-color: transparent;
}
.scr-columns__box .scr-icon {
  width: 12px;
  height: 12px;
}
@media (max-width: 720px) {
  .scr-columns {
    grid-template-columns: 1fr;
  }
}
</style>
