// Drag to reorder, the terminal's pointer pattern: a 4px threshold before the
// drag starts, the source takes the accent colour (`data-dragging`), the target
// shows a line on the edge the item will land at (`data-drop-edge`). Items are
// the elements matching `selector` inside the root; each carries its key in
// `data-key`. A key listed in `locked` neither moves nor accepts a drop.

import { onBeforeUnmount, onMounted } from 'vue'

const THRESHOLD_PX = 4
const DRAGGING_CLASS = 'scr-dragging'

/**
 * @param {import('vue').Ref<HTMLElement>} root
 * @param {{ selector: string, axis: 'x' | 'y', locked?: string[], onMove: (key: string, target: string, edge: 'before' | 'after') => void, onStart?: () => void }} options
 */
export function useDragReorder(root, { selector, axis, locked = [], onMove, onStart }) {
  let gesture = null
  let target = null
  let edge = null
  let suppressClick = false

  function paint() {
    root.value?.querySelectorAll(selector).forEach((el) => {
      el.toggleAttribute('data-dragging', Boolean(gesture?.active && el.dataset.key === gesture.key))
      if (target && el.dataset.key === target) el.dataset.dropEdge = edge
      else delete el.dataset.dropEdge
    })
  }

  function finish() {
    if (suppressClick) setTimeout(() => (suppressClick = false), 0)
    gesture = target = edge = null
    removeEventListener('pointermove', onPointerMove)
    removeEventListener('pointerup', onPointerUp)
    removeEventListener('pointercancel', finish)
    document.body.classList.remove(DRAGGING_CLASS)
    paint()
  }

  function onPointerMove(event) {
    if (!gesture || event.pointerId !== gesture.id) return
    if (!gesture.active && Math.hypot(event.clientX - gesture.x, event.clientY - gesture.y) < THRESHOLD_PX) return
    if (!gesture.active) {
      gesture.active = true
      suppressClick = true
      document.body.classList.add(DRAGGING_CLASS)
      onStart?.()
    }
    event.preventDefault()
    const hit = document.elementFromPoint(event.clientX, event.clientY)
    const el = hit?.closest(selector)
    target = edge = null
    if (el && root.value.contains(el) && el.dataset.key !== gesture.key && !locked.includes(el.dataset.key)) {
      const box = el.getBoundingClientRect()
      const before = axis === 'x' ? event.clientX < box.left + box.width / 2 : event.clientY < box.top + box.height / 2
      target = el.dataset.key
      edge = before ? 'before' : 'after'
    }
    paint()
  }

  function onPointerUp(event) {
    if (!gesture || event.pointerId !== gesture.id) return
    onPointerMove(event)
    const move = gesture.active && target ? [gesture.key, target, edge] : null
    finish()
    if (move) onMove(...move)
  }

  function onPointerDown(event) {
    if (event.button !== 0 || event.target.closest('button, input, select')) return
    const el = event.target.closest(selector)
    if (!el || !root.value.contains(el) || !el.dataset.key || locked.includes(el.dataset.key)) return
    gesture = { key: el.dataset.key, x: event.clientX, y: event.clientY, id: event.pointerId, active: false }
    addEventListener('pointermove', onPointerMove)
    addEventListener('pointerup', onPointerUp)
    addEventListener('pointercancel', finish)
  }

  // The click that ends a drag must not also sort the column.
  function onClickCapture(event) {
    if (!suppressClick) return
    event.stopPropagation()
    event.preventDefault()
  }

  onMounted(() => {
    root.value.addEventListener('pointerdown', onPointerDown)
    root.value.addEventListener('click', onClickCapture, true)
  })
  onBeforeUnmount(() => {
    root.value?.removeEventListener('pointerdown', onPointerDown)
    root.value?.removeEventListener('click', onClickCapture, true)
    if (gesture) finish()
  })
}
