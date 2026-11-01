<template>
  <div class="lab">
    <div class="lab__bar">
      <span class="lab__title">Screener widget sandbox</span>
      <span class="lab__spacer" />
      <div class="lab__seg" role="group" aria-label="Data source">
        <button
          type="button"
          :aria-pressed="sourceKind === 'server'"
          :title="`Rows from ${serverUrl}`"
          @click="sourceKind = 'server'"
        >
          Server
        </button>
        <button
          type="button"
          :aria-pressed="sourceKind === 'mock'"
          title="Deterministic synthetic rows, every column filled"
          @click="sourceKind = 'mock'"
        >
          Mock
        </button>
      </div>
      <button type="button" class="scr-btn -ghost -icon" title="Toggle theme" @click="toggleTheme">
        <ScreenerIcon name="theme" />
      </button>
    </div>

    <!-- A stand-in for terminal-front's WidgetFrame: 32px header, 8px radius. -->
    <section class="frame" aria-label="Screener widget">
      <div class="frame__header">
        <span class="frame__grip"><ScreenerIcon name="grip" /></span>
        <span class="frame__title">Screener</span>
        <span class="frame__symbol" title="Linked instrument group follows the selected row">
          <i />{{ selectedBase }} <small>{{ selectedRest }}</small>
        </span>
        <div class="frame__actions">
          <button type="button" class="scr-btn -ghost -icon" title="Widget settings"><ScreenerIcon name="gear" /></button>
          <button type="button" class="scr-btn -ghost -icon" title="Maximize"><ScreenerIcon name="maximize" /></button>
        </div>
      </div>
      <div class="frame__body">
        <ScreenerWidget
          :source="source"
          :settings="settings"
          :instrument-id="selected"
          @update:settings="patchSettings"
          @select="onSelect"
          @trade="toast(`Order form opened for ${$event}`)"
          @notify="toast"
        />
      </div>
    </section>

    <div class="lab__toast" :class="{ '-show': toastText }"><i />{{ toastText }}</div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'

import ScreenerIcon from '../../src/components/ScreenerIcon.vue'
import ScreenerWidget from '../../src/ScreenerWidget.vue'

const STORAGE_KEY = 'screener-sandbox'
const query = new URLSearchParams(window.location.search)

/**
 * Where the rows come from: `?server=http://host:port` wins; a page the Go
 * service serves itself (under /ui/) talks to its own origin; the Vite dev
 * server talks to the service on its default port.
 */
function resolveServerUrl() {
  if (query.get('server')) return query.get('server').replace(/\/+$/, '')
  if (/^https?:$/.test(window.location.protocol) && window.location.pathname.startsWith('/ui')) {
    return window.location.origin
  }
  return 'http://localhost:8090'
}

function readStored() {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY)) ?? {}
  } catch {
    return {}
  }
}
const stored = readStored()

const serverUrl = resolveServerUrl()
const sourceKind = ref(stored.sourceKind ?? 'server')
const theme = ref(stored.theme ?? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'))
const settings = ref(stored.settings ?? {})
const selected = ref(stored.selected ?? 'BTC-USDT-SWAP')

const source = computed(() =>
  sourceKind.value === 'server'
    ? { kind: 'server', baseUrl: serverUrl, pollMs: 5000, sse: query.get('sse') !== '0' }
    : { kind: 'mock', pollMs: 5000 },
)
const selectedBase = computed(() => selected.value.split('-')[0])
const selectedRest = computed(() => selected.value.split('-').slice(1).join(' '))

watch(
  [sourceKind, theme, settings, selected],
  () => {
    document.documentElement.dataset.theme = theme.value
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        sourceKind: sourceKind.value,
        theme: theme.value,
        settings: settings.value,
        selected: selected.value,
      }),
    )
  },
  { immediate: true, deep: true },
)

/** What `useWidget().setSettings(partial)` does in the terminal. */
function patchSettings(partial) {
  settings.value = { ...settings.value, ...partial }
}

function onSelect(instId) {
  selected.value = instId
  toast(`Chart, order book and order form switched to ${instId}`)
}

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
}

const toastText = ref('')
let toastTimer = null
function toast(message) {
  toastText.value = message
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toastText.value = ''), 2200)
}
</script>

<style scoped>
.lab {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  height: 100%;
  padding: var(--space-12) var(--space-16);
  box-sizing: border-box;
  overflow: hidden;
}
.lab__bar {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  flex: none;
  color: var(--text-mid-emphasis);
}
.lab__title {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.lab__spacer {
  flex: 1;
}
.lab__seg {
  display: inline-flex;
  gap: var(--space-2);
  padding: var(--space-2);
  border-radius: var(--radius-8);
  background: var(--card-neutral-background);
}
.lab__seg button {
  height: 24px;
  padding: 0 var(--space-8);
  border: 0;
  border-radius: var(--radius-6);
  background: transparent;
  color: var(--text-mid-emphasis);
  font: inherit;
  cursor: pointer;
}
.lab__seg button[aria-pressed='true'] {
  background: var(--widget-background);
  color: var(--text-high-emphasis);
  font-weight: 500;
}

.frame {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius-8);
  background: var(--widget-background);
}
.frame__header {
  display: flex;
  align-items: center;
  gap: var(--space-6);
  flex: none;
  height: 32px;
  padding: 0 var(--space-6);
  border-bottom: 1px solid var(--border);
  background: var(--widget-header-background);
  user-select: none;
}
.frame__grip {
  color: var(--icons-low-emphasis);
}
.frame__grip .scr-icon {
  width: 12px;
  height: 12px;
}
.frame__title {
  color: var(--text-mid-emphasis);
}
.frame__symbol {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
  height: 24px;
  padding: 0 var(--space-8);
  border-radius: var(--radius-6);
  color: var(--text-high-emphasis);
  font-weight: 600;
}
.frame__symbol i {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-circle);
  background: var(--link-group-orange);
}
.frame__symbol small {
  color: var(--text-mid-emphasis);
  font-weight: 400;
}
.frame__actions {
  display: flex;
  gap: var(--space-2);
  margin-left: auto;
}
.frame__actions .scr-btn {
  width: 24px;
  height: 24px;
}
.frame__body {
  flex: 1;
  min-height: 0;
}

.lab__toast {
  position: fixed;
  left: 50%;
  bottom: var(--space-24);
  z-index: var(--layer-toast);
  display: inline-flex;
  align-items: center;
  gap: var(--space-8);
  padding: var(--space-6) var(--space-8);
  border-radius: var(--radius-8);
  background: var(--dark-background);
  color: var(--text-light);
  transform: translateX(-50%);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--duration-fast) ease-out;
}
.lab__toast.-show {
  opacity: 1;
}
.lab__toast i {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-circle);
  background: var(--link-group-orange);
}
</style>
