<template>
  <div class="scr-filters">
    <span v-for="(chip, index) in chips" :key="index" class="scr-chip">
      <b>{{ chip.label }}</b> {{ chip.op }} {{ chip.value }}
      <button type="button" title="Remove filter" aria-label="Remove filter" @click="emit('remove', index)">
        <ScreenerIcon name="close" />
      </button>
    </span>

    <span class="scr-filters__note">{{ chips.length ? '' : 'No filters' }} {{ sortText }}</span>

    <form v-if="saving" class="scr-filters__save" @submit.prevent="save">
      <input
        ref="nameInput"
        v-model="name"
        class="scr-input"
        placeholder="Preset name"
        aria-label="Preset name"
      />
      <button type="submit" class="scr-btn">Save</button>
      <button type="button" class="scr-btn -ghost" @click="saving = false">Cancel</button>
    </form>
    <button v-else-if="canSave" type="button" class="scr-btn -ghost" @click="startSaving">
      Save as preset
    </button>
    <button v-if="canDelete" type="button" class="scr-btn -ghost" @click="emit('delete')">
      Delete preset
    </button>
  </div>
</template>

<script setup>
// The active filters as removable chips, the sort in words, and saving the
// current filter set as a custom preset.
import { nextTick, ref, useTemplateRef } from 'vue'

import ScreenerIcon from './ScreenerIcon.vue'

defineProps({
  /** `{ label, op, value }` with already formatted text. */
  chips: { type: Array, required: true },
  sortText: { type: String, default: '' },
  canSave: { type: Boolean, default: false },
  canDelete: { type: Boolean, default: false },
})
const emit = defineEmits(['remove', 'save', 'delete'])

const saving = ref(false)
const name = ref('')
const nameInput = useTemplateRef('nameInput')

async function startSaving() {
  saving.value = true
  name.value = ''
  await nextTick()
  nameInput.value?.focus()
}

function save() {
  emit('save', name.value.trim() || 'My preset')
  saving.value = false
}
</script>

<style scoped>
.scr-filters {
  display: flex;
  align-items: center;
  gap: var(--space-6);
  flex: none;
  flex-wrap: wrap;
  padding: var(--space-6) var(--space-8);
  border-bottom: 1px solid var(--border);
  background: var(--widget-header-background);
}
.scr-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-4);
  height: 24px;
  padding: 0 var(--space-4) 0 var(--space-8);
  border: 1px solid var(--border);
  border-radius: var(--radius-8);
  background: var(--card-background);
  color: var(--text-mid-emphasis);
  font-weight: 500;
  white-space: nowrap;
}
.scr-chip b {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-chip button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-circle);
  background: transparent;
  color: var(--icons-mid-emphasis);
  cursor: pointer;
}
.scr-chip button:hover {
  color: var(--text-error-accent);
}
.scr-chip .scr-icon {
  width: 12px;
  height: 12px;
}
.scr-filters__note {
  color: var(--text-mid-emphasis);
}
.scr-filters__save {
  display: inline-flex;
  gap: var(--space-6);
}
.scr-filters .scr-btn,
.scr-filters .scr-input {
  height: 24px;
}
.scr-filters .scr-input {
  width: 140px;
}
</style>
