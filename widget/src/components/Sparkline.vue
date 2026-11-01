<template>
  <svg class="scr-spark" :viewBox="`0 0 ${WIDTH} ${HEIGHT}`" aria-hidden="true">
    <defs>
      <clipPath :id="`${uid}a`"><rect x="0" y="0" :width="WIDTH" :height="baseY" /></clipPath>
      <clipPath :id="`${uid}b`"><rect x="0" :y="baseY" :width="WIDTH" :height="HEIGHT" /></clipPath>
    </defs>
    <line :x1="first.x" :x2="last.x" :y1="baseY" :y2="baseY" stroke="var(--border)" stroke-width="1" />
    <path :d="area" fill="var(--long)" opacity="0.16" :clip-path="`url(#${uid}a)`" />
    <path :d="area" fill="var(--short)" opacity="0.16" :clip-path="`url(#${uid}b)`" />
    <path :d="line" fill="none" stroke="var(--long)" stroke-width="1.2" :clip-path="`url(#${uid}a)`" />
    <path :d="line" fill="none" stroke="var(--short)" stroke-width="1.2" :clip-path="`url(#${uid}b)`" />
    <circle :cx="last.x" :cy="last.y" r="1.6" :fill="color" />
  </svg>
</template>

<script setup>
// Up to 24 hourly closes, oldest first, drawn against a baseline (the price
// 24 h ago): the line and fill are green above the baseline and red below it,
// so the sign of the day and the path to it read without the number.
import { computed } from 'vue'

const WIDTH = 72
const HEIGHT = 22
let seq = 0

const props = defineProps({
  values: { type: Array, required: true },
  base: { type: Number, default: null },
})

const uid = `spark${++seq}`
const baseline = computed(() => (props.base > 0 ? props.base : props.values[0]))

const scale = computed(() => {
  const min = Math.min(...props.values, baseline.value)
  const span = Math.max(...props.values, baseline.value) - min || 1
  return (value) => HEIGHT - 2 - ((value - min) / span) * (HEIGHT - 4)
})
const points = computed(() =>
  props.values.map((value, index) => ({
    x: (index / (props.values.length - 1)) * (WIDTH - 2) + 1,
    y: scale.value(value),
  })),
)
const first = computed(() => points.value[0])
const last = computed(() => points.value[points.value.length - 1])
const baseY = computed(() => Number(scale.value(baseline.value).toFixed(1)))
const line = computed(() =>
  points.value.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(' '),
)
const area = computed(
  () => `${line.value} L${last.value.x.toFixed(1)} ${baseY.value} L${first.value.x.toFixed(1)} ${baseY.value} Z`,
)
const color = computed(() =>
  props.values[props.values.length - 1] >= baseline.value ? 'var(--long)' : 'var(--short)',
)
</script>

<style scoped>
.scr-spark {
  display: block;
  width: 72px;
  height: 22px;
}
</style>
