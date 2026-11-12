// One popover at a time: which one (`name`), what it is about (`key`, the
// column of a phrase) and the element it hangs from. The opener is found by
// its `data-pop` / `data-key` attributes after the next render, because a
// phrase that was just added does not exist yet when its popover is asked for.

import { nextTick, reactive } from 'vue'

/**
 * @param {import('vue').Ref<HTMLElement>} root  the widget's root element
 * @param {{ beforeOpen?: (key: string | null) => void, afterClose?: () => void }} hooks
 */
export function usePopover(root, hooks = {}) {
  const popover = reactive({ name: null, key: null, anchor: null })

  function close() {
    if (!popover.name) return
    popover.name = null
    popover.key = null
    popover.anchor = null
    hooks.afterClose?.()
  }

  /** Opens `name` for `key`; asking for the one that is open closes it. */
  async function open(name, key = null) {
    if (popover.name === name && popover.key === key) {
      close()
      return
    }
    hooks.beforeOpen?.(key)
    popover.name = name
    popover.key = key
    popover.anchor = null
    await nextTick()
    if (popover.name !== name || popover.key !== key) return
    const selector = `[data-pop="${name}"]` + (key ? `[data-key="${key}"]` : '')
    const anchor = root.value?.querySelector(selector)
    if (anchor) popover.anchor = anchor
    else close()
  }

  return { popover, open, close }
}
