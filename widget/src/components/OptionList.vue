<template>
  <div class="scr-options" role="menu">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="scr-pop__row"
      :role="multiple ? 'menuitemcheckbox' : 'menuitemradio'"
      :aria-checked="Boolean(option.checked)"
      :disabled="option.disabled"
      @click="emit('pick', option.value)"
    >
      <span v-if="multiple" class="scr-pop__box"><ScreenerIcon v-if="option.checked" name="check" /></span>
      <VenueIcon v-if="option.venue" :code="option.venue" />
      {{ option.label }}
      <span class="scr-pop__hint">{{ option.hint }}</span>
      <span v-if="!multiple" class="scr-pop__tick"><ScreenerIcon v-if="option.checked" name="check" /></span>
    </button>
  </div>
</template>

<script setup>
// A list of choices inside a popover: one of many (a tick on the chosen row)
// or several (checkboxes). The hint on the right is usually a count.
import ScreenerIcon from './ScreenerIcon.vue'
import VenueIcon from './VenueIcon.vue'

defineProps({
  /** `{ value, label, hint?, checked?, disabled?, venue? }` */
  options: { type: Array, required: true },
  multiple: { type: Boolean, default: false },
})
const emit = defineEmits(['pick'])
</script>

<style>
.scr-options {
  max-height: min(60vh, 420px);
  overflow: auto;
  overscroll-behavior: contain;
}
</style>
