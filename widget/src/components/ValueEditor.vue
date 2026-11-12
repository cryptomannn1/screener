<template>
  <div class="scr-value">
    <div class="scr-value__row">
      <label class="scr-value__field">
        <span v-if="affix[0]">{{ affix[0] }}</span>
        <input
          ref="first"
          type="text"
          inputmode="decimal"
          autocomplete="off"
          :placeholder="between ? 'From' : 'Value'"
          :aria-label="between ? 'From' : 'Value'"
          :value="text1"
          @input="onInput('v', $event.target.value)"
          @keydown.enter.prevent="between ? second?.focus() : emit('done')"
        />
        <span v-if="affix[1]">{{ affix[1] }}</span>
      </label>
      <template v-if="between">
        <span class="scr-value__and">and</span>
        <label class="scr-value__field">
          <span v-if="affix[0]">{{ affix[0] }}</span>
          <input
            ref="second"
            type="text"
            inputmode="decimal"
            autocomplete="off"
            placeholder="To"
            aria-label="To"
            :value="text2"
            @input="onInput('v2', $event.target.value)"
            @keydown.enter.prevent="emit('done')"
          />
          <span v-if="affix[1]">{{ affix[1] }}</span>
        </label>
      </template>
    </div>
    <div v-if="!between && quick.length" class="scr-value__quick">
      <button v-for="q in quick" :key="q.op + q.v" type="button" class="scr-opt" :aria-pressed="condition.op === q.op && condition.v === q.v" @click="emit('quick', q.op, q.v)">
        {{ q.label }}
      </button>
    </div>
  </div>
</template>

<script setup>
// The value of a number condition: a field with the unit of the column and,
// under it, the usual thresholds as one-click answers. The table answers
// while the value is typed; Enter closes.
import { computed, onMounted, ref, useTemplateRef } from 'vue'

import { COLUMNS } from '../columns.js'
import { UNIT_AFFIX, parseValue, quickLabel, valueInput } from '../filters.js'

const props = defineProps({
  columnKey: { type: String, required: true },
  /** `{ op, v, v2 }` */
  condition: { type: Object, required: true },
})
const emit = defineEmits(['change', 'quick', 'done'])

const first = useTemplateRef('first')
const second = useTemplateRef('second')
const between = computed(() => props.condition.op === 'between')
const affix = computed(() => UNIT_AFFIX[COLUMNS[props.columnKey].unit] || ['', ''])
const quick = computed(() => (COLUMNS[props.columnKey].quick || []).map(([op, v]) => ({ op, v, label: quickLabel(props.columnKey, op, v) })))

// The fields show what was there when the editor opened; after that the text is the user's.
const text1 = ref(valueInput(props.columnKey, props.condition.v))
const text2 = ref(valueInput(props.columnKey, props.condition.v2))

function onInput(field, text) {
  if (field === 'v') text1.value = text
  else text2.value = text
  emit('change', field, parseValue(props.columnKey, text))
}

onMounted(() => {
  first.value?.focus()
  first.value?.select()
})
</script>

<style>
.scr-value {
  display: grid;
  gap: var(--space-8);
  padding: var(--space-4);
}
.scr-value__row {
  display: flex;
  align-items: center;
  gap: var(--space-8);
}
.scr-value__field {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  flex: 1;
  height: 28px;
  padding: 0 var(--space-8);
  border-radius: var(--radius-6);
  background: var(--dropdown-hover);
  color: var(--terminal-v2-muted);
}
.scr-value__field:focus-within {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
.scr-value__field input {
  flex: 1;
  width: 104px;
  min-width: 0;
  height: 26px;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--terminal-v2-text);
}
.scr-value__and {
  color: var(--terminal-v2-muted);
  white-space: nowrap;
}
.scr-value__quick {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
}
</style>
