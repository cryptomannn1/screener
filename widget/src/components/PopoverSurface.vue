<template>
  <Teleport to="body">
    <div
      ref="surface"
      class="scr-pop"
      :style="{ left: `${position.left}px`, top: `${position.top}px` }"
      v-bind="$attrs"
    >
      <slot />
    </div>
  </Teleport>
</template>

<script setup>
// A dropdown surface hung from an anchor element: fixed position, flips above
// the anchor when there is no room below, closes on an outside click, Escape,
// a resize or a scroll. In the terminal this is `PopperNew` + `DropdownSurface`.
import { nextTick, onBeforeUnmount, onMounted, reactive, useTemplateRef, watch } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  anchor: { type: Object, required: true },
  align: { type: String, default: 'end' },
})
const emit = defineEmits(['close'])

const surface = useTemplateRef('surface')
const position = reactive({ left: 0, top: 0 })
const MARGIN = 8
const GAP = 4

function place() {
  const anchor = props.anchor.getBoundingClientRect()
  const size = surface.value.getBoundingClientRect()
  let left = props.align === 'end' ? anchor.right - size.width : anchor.left
  left = Math.max(MARGIN, Math.min(left, window.innerWidth - size.width - MARGIN))
  let top = anchor.bottom + GAP
  if (top + size.height > window.innerHeight - MARGIN) {
    top = Math.max(MARGIN, anchor.top - size.height - GAP)
  }
  position.left = left
  position.top = top
}

function onPointerDown(event) {
  if (surface.value?.contains(event.target) || props.anchor.contains(event.target)) return
  emit('close')
}

function onKeyDown(event) {
  if (event.key === 'Escape') emit('close')
}

function onLayoutChange(event) {
  // Scrolling inside the surface itself is not a reason to close.
  if (event.type === 'scroll' && surface.value?.contains(event.target)) return
  emit('close')
}

onMounted(async () => {
  await nextTick()
  place()
  document.addEventListener('pointerdown', onPointerDown, true)
  document.addEventListener('keydown', onKeyDown)
  window.addEventListener('resize', onLayoutChange)
  document.addEventListener('scroll', onLayoutChange, true)
})

watch(() => props.anchor, () => nextTick(place))

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown, true)
  document.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('resize', onLayoutChange)
  document.removeEventListener('scroll', onLayoutChange, true)
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
.scr-pop__foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-6);
  padding: var(--space-8);
}
</style>
