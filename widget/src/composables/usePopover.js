// One popover at a time: which one, the element it hangs from and what it is
// about (a row's instrument id for the row menu).

import { reactive } from 'vue'

export function usePopover() {
  const popover = reactive({ kind: null, anchor: null, payload: null })

  function close() {
    popover.kind = null
    popover.anchor = null
    popover.payload = null
  }

  /** Opens `kind` from `anchor`; a second click on the same anchor closes it. */
  function toggle(kind, anchor, payload = null) {
    if (popover.kind === kind && popover.anchor === anchor) {
      close()
      return
    }
    popover.kind = kind
    popover.anchor = anchor
    popover.payload = payload
  }

  return { popover, toggle, close }
}
