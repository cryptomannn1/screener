<template>
  <Teleport to="body">
    <div ref="surface" class="scr-pop" :style="{ left: `${position.left}px`, top: `${position.top}px` }" v-bind="$attrs">
      <slot />
    </div>
  </Teleport>
</template>

<script setup>
// A dropdown surface hung from an anchor element: fixed position under the
// anchor's left edge, kept inside the window, re-placed when its content or
// the window changes size. Closes on an outside press and on Escape. In the
// terminal this is `PopperNew` + `DropdownSurface`.
import { nextTick, onBeforeUnmount, onMounted, reactive, useTemplateRef, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  anchor: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const surface = useTemplateRef('surface')
const position = reactive({ left: 0, top: 0 })
const MARGIN = 8
const GAP = 4
let observer = null

function place() {
  if (!surface.value || !props.anchor.isConnected) return
  const anchor = props.anchor.getBoundingClientRect()
  const w = surface.value.offsetWidth
  const h = surface.value.offsetHeight
  position.left = Math.max(MARGIN, Math.min(anchor.left, window.innerWidth - w - MARGIN))
  position.top = Math.max(MARGIN, Math.min(anchor.bottom + GAP, window.innerHeight - h - MARGIN))
}

function onPointerDown(event) {
  if (surface.value?.contains(event.target) || props.anchor.contains(event.target)) return
  emit('close')
}

function onKeyDown(event) {
  if (event.key === 'Escape') emit('close')
}

onMounted(async () => {
  await nextTick()
  place()
  document.addEventListener('pointerdown', onPointerDown, true)
  document.addEventListener('keydown', onKeyDown)
  window.addEventListener('resize', place)
  if (typeof ResizeObserver === 'function') {
    observer = new ResizeObserver(place)
    observer.observe(surface.value)
    observer.observe(props.anchor)
  }
})

watch(() => props.anchor, () => nextTick(place))

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown, true)
  document.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('resize', place)
  observer?.disconnect()
})
</script>

<style>
/* Not scoped: the surface is teleported to body, and the row / group / separator
   classes are used by the popover contents too. */
.scr-pop {
  position: fixed;
  z-index: var(--layer-dropdown);
  box-sizing: border-box;
  min-width: 200px;
  max-width: calc(100vw - 16px);
  padding: var(--space-4);
  border-radius: var(--radius-8);
  background: var(--dropdown-background);
  color: var(--terminal-v2-text);
  box-shadow: var(--dropdown-shadow);
  font-size: 12px;
  line-height: 16px;
  font-variant-numeric: tabular-nums;
}
.scr-pop *,
.scr-pop *::before,
.scr-pop *::after {
  box-sizing: border-box;
}
.scr-pop button,
.scr-pop input,
.scr-pop select {
  font: inherit;
  color: inherit;
  letter-spacing: normal;
}
.scr-pop__row {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  width: 100%;
  min-height: var(--space-32);
  padding: var(--space-8);
  border: 0;
  border-radius: var(--radius-4);
  background: transparent;
  color: var(--terminal-v2-text);
  text-align: left;
  cursor: pointer;
}
.scr-pop__row:hover:not(:disabled),
.scr-pop__row:focus-visible {
  background: var(--dropdown-hover);
}
.scr-pop__row:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
.scr-pop__row:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.scr-pop__row[aria-checked='true'] {
  font-weight: 500;
}
.scr-pop__row.-muted {
  color: var(--terminal-v2-muted);
}
.scr-pop__hint {
  margin-left: auto;
  color: var(--terminal-v2-muted);
  font-size: 10px;
  font-weight: 400;
}
/* a fixed place for the tick, so the hints stay in one column */
.scr-pop__tick {
  display: block;
  flex: none;
  width: 12px;
  height: 12px;
  color: var(--primary-accent);
}
.scr-pop__tick .scr-icon {
  width: 12px;
  height: 12px;
}
.scr-pop__box {
  display: grid;
  place-items: center;
  flex: none;
  width: 16px;
  height: 16px;
  border: 1px solid var(--dropdown-border);
  border-radius: var(--radius-4);
  color: var(--text-light);
}
.scr-pop__row[aria-checked='true'] .scr-pop__box {
  border-color: transparent;
  background: var(--primary-accent);
}
.scr-pop__box .scr-icon {
  width: 12px;
  height: 12px;
}
.scr-pop__row .scr-venue {
  color: var(--terminal-v2-muted);
}
.scr-pop__group {
  padding: 10px var(--space-8) var(--space-4);
  color: var(--terminal-v2-muted);
  font-size: 10px;
  line-height: 12px;
  letter-spacing: 0.4px;
  text-transform: uppercase;
}
.scr-pop__sep {
  height: 1px;
  margin: var(--space-4) 0;
  background: var(--dropdown-border);
}
.scr-pop__empty {
  padding: var(--space-16) var(--space-8);
  color: var(--terminal-v2-muted);
  text-align: center;
}
.scr-pop__search {
  position: relative;
  display: block;
  padding: var(--space-4) var(--space-4) 0;
}
.scr-pop__search input {
  width: 100%;
  height: 28px;
  padding: 0 var(--space-8) 0 28px;
  border: 0;
  border-radius: var(--radius-8);
  background: var(--dropdown-hover);
  color: var(--terminal-v2-text);
}
.scr-pop__search input:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
.scr-pop__search .scr-icon {
  position: absolute;
  left: 12px;
  top: 11px;
  color: var(--terminal-v2-muted);
  pointer-events: none;
}
/* a small option button: a quick threshold, a period chip of the catalogue */
.scr-opt {
  height: 24px;
  padding: 0 var(--space-8);
  border: 0;
  border-radius: var(--radius-6);
  background: var(--dropdown-hover);
  color: var(--terminal-v2-muted);
  white-space: nowrap;
  cursor: pointer;
}
.scr-opt:hover {
  color: var(--terminal-v2-text);
}
.scr-opt[aria-pressed='true'] {
  background: var(--card-primary-background-light);
  color: var(--terminal-v2-text);
  font-weight: 500;
}
.scr-opt:focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}
</style>
