<template>
  <div class="scr-pop__group">{{ instId }}</div>
  <button
    type="button"
    class="scr-pop__row"
    role="menuitem"
    title="Switches the linked chart, order book and order form"
    @click="emit('action', 'select')"
  >
    Show on chart
  </button>
  <button type="button" class="scr-pop__row" role="menuitem" title="Opens the order form" @click="emit('action', 'trade')">
    Trade
  </button>
  <button type="button" class="scr-pop__row" role="menuitem" @click="emit('action', 'star')">
    {{ starred ? 'Remove from watchlist' : 'Add to watchlist' }}
  </button>
  <button type="button" class="scr-pop__row" role="menuitem" disabled title="Alerts come in v2">
    Set alert
  </button>
  <div class="scr-pop__sep" />
  <button type="button" class="scr-pop__row" role="menuitem" @click="emit('action', 'copy')">
    Copy symbol
  </button>
</template>

<script setup>
defineProps({
  instId: { type: String, required: true },
  starred: { type: Boolean, default: false },
})
const emit = defineEmits(['action'])
</script>
