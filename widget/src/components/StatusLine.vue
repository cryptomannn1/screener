<template>
  <div class="scr-status">
    <span>Instruments <b>{{ total }}</b></span>
    <span>Shown <b>{{ shownText }}</b></span>
    <span :title="`Source ${sourceLabel}`">Updated <b>{{ time(updatedAt) }}</b></span>
    <span class="scr-status__spacer" />
    <span class="scr-status__coverage" title="Instruments with each data family">{{ coverageText }}</span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

import { time } from '../format.js'

const props = defineProps({
  total: { type: Number, default: 0 },
  shown: { type: Number, default: 0 },
  drawn: { type: Number, default: 0 },
  updatedAt: { type: Number, default: null },
  sourceLabel: { type: String, default: '' },
  /** "Candles 517/517, books 517/517, OI Δ 0/517, flow 0/517" or ''. */
  coverageText: { type: String, default: '' },
})

const shownText = computed(() =>
  props.drawn < props.shown ? `${props.shown} (first ${props.drawn} drawn)` : String(props.shown),
)
</script>

<style scoped>
.scr-status {
  display: flex;
  align-items: center;
  gap: var(--space-16);
  flex: none;
  flex-wrap: wrap;
  padding: var(--space-6) var(--space-8);
  border-top: 1px solid var(--border);
  background: var(--widget-header-background);
  color: var(--text-mid-emphasis);
}
.scr-status b {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-status__spacer {
  flex: 1;
}
.scr-status__coverage {
  text-align: right;
}
</style>
