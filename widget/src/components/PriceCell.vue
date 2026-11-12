<template>
  <span :key="flashCount" class="scr-price" :class="flash ? `-flash-${flash}` : null">${{ priceText(shown) }}</span>
</template>

<script setup>
// Price as a running figure, as in the terminal control bar (AnimatedNumber,
// tone="none"): on a live tick the digits run to the new value in 200 ms and
// take the side's colour, then fade back. A change the user caused (another
// exchange became the busiest one after a toggle) is shown at once, no flash.
import { onBeforeUnmount, ref, watch } from 'vue'

import { priceText } from '../format.js'

const TWEEN_MS = 200

const props = defineProps({
  value: { type: Number, required: true },
  /** Count of live deliveries: a value that changed together with it changed live. */
  tick: { type: Number, default: 0 },
})

const shown = ref(props.value)
const flash = ref('')
// Re-keying the element restarts the colour animation when two ticks in a row go the same way.
const flashCount = ref(0)
let frame = null

const canAnimate = () => !document.hidden && !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

function stop() {
  if (frame !== null) cancelAnimationFrame(frame)
  frame = null
}

watch(
  () => [props.value, props.tick],
  ([value, tick], [oldValue, oldTick]) => {
    if (value === oldValue) return
    stop()
    if (tick === oldTick || !canAnimate()) {
      shown.value = value
      flash.value = ''
      return
    }
    const from = shown.value
    const startedAt = performance.now()
    flash.value = value > from ? 'up' : 'down'
    flashCount.value++
    const step = (now) => {
      const p = Math.min(1, (now - startedAt) / TWEEN_MS)
      shown.value = p >= 1 ? value : from + (value - from) * (1 - (1 - p) ** 3)
      frame = p >= 1 ? null : requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
  },
)

onBeforeUnmount(stop)
</script>

<style>
.scr-price {
  display: inline-block;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.scr-price.-flash-up {
  animation: scr-price-flash-up var(--duration-base) var(--ease-standard);
}
.scr-price.-flash-down {
  animation: scr-price-flash-down var(--duration-base) var(--ease-standard);
}
@keyframes scr-price-flash-up {
  from {
    color: var(--long);
  }
}
@keyframes scr-price-flash-down {
  from {
    color: var(--short);
  }
}
</style>
