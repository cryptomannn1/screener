<template>
  <div class="scr-bar">
    <label class="scr-search">
      <ScreenerIcon name="search" />
      <input
        class="scr-input"
        type="search"
        placeholder="Search"
        aria-label="Search by ticker or name"
        autocomplete="off"
        :value="search"
        @input="emit('update:search', $event.target.value)"
      />
    </label>

    <button type="button" class="scr-btn -dd" data-pop="exchanges" aria-haspopup="true" :aria-expanded="open === 'exchanges'" @click="emit('open', 'exchanges', null)">
      <span class="scr-dd__name">Exchanges</span><span class="scr-dd__value">{{ exchangesLabel }}</span><ScreenerIcon name="down" />
    </button>
    <button type="button" class="scr-btn -dd" data-pop="class" aria-haspopup="true" :aria-expanded="open === 'class'" @click="emit('open', 'class', null)">
      <span class="scr-dd__name">Assets</span><span class="scr-dd__value">{{ classLabel }}</span><ScreenerIcon name="down" />
    </button>

    <!-- A condition as a phrase: column, operator, value, remove. Each part that can change is a button. -->
    <span v-for="phrase in phrases" :key="phrase.key" class="scr-phrase">
      <span class="scr-phrase__name">{{ phrase.name }}</span>
      <span v-if="phrase.word">{{ phrase.word }}</span>
      <button
        v-for="part in phrase.parts"
        :key="part.pop"
        type="button"
        class="scr-phrase__part"
        :class="{ '-value': part.strong, '-empty': part.empty }"
        :data-pop="part.pop"
        :data-key="phrase.key"
        aria-haspopup="true"
        :aria-expanded="open === part.pop && openKey === phrase.key"
        @click="emit('open', part.pop, phrase.key)"
      >
        {{ part.text }}
      </button>
      <button type="button" class="scr-phrase__remove" title="Remove the condition" :aria-label="`Remove the condition on ${phrase.name}`" @click="emit('remove', phrase.key)">
        <ScreenerIcon name="close" />
      </button>
    </span>

    <button type="button" class="scr-btn -add" data-pop="add" aria-haspopup="true" :aria-expanded="open === 'add'" @click="emit('open', 'add', null)">
      <ScreenerIcon name="filter" />Add filter
    </button>

    <span class="scr-bar__grow" />
    <span v-if="countText" class="scr-bar__count">{{ countText }}</span>
    <button v-if="canReset" type="button" class="scr-btn -ghost" @click="emit('reset')">Reset filters</button>
    <button type="button" class="scr-btn -dd" data-pop="columns" aria-haspopup="true" :aria-expanded="open === 'columns'" @click="emit('open', 'columns', null)">
      <span class="scr-dd__name">Columns</span><span class="scr-dd__value">{{ columnsCount }}</span><ScreenerIcon name="down" />
    </button>
  </div>
</template>

<script setup>
// Everything that chooses the rows, read as one line from the left: where
// (exchanges), what (asset class), then the conditions as phrases. On the
// right, the columns. Popovers are opened by the widget; the buttons only say
// which one (`data-pop`, `data-key`) so it can find them as anchors.
import ScreenerIcon from './ScreenerIcon.vue'

defineProps({
  search: { type: String, default: '' },
  exchangesLabel: { type: String, required: true },
  classLabel: { type: String, required: true },
  /** `{ key, name, word?, parts: [{ pop, text, strong?, empty? }] }` */
  phrases: { type: Array, required: true },
  /** "12 of 520" while conditions are set, else empty. */
  countText: { type: String, default: '' },
  canReset: { type: Boolean, default: false },
  columnsCount: { type: Number, required: true },
  /** The open popover and the phrase it belongs to. */
  open: { type: String, default: null },
  openKey: { type: String, default: null },
})
const emit = defineEmits(['update:search', 'open', 'remove', 'reset'])
</script>

<style scoped>
.scr-bar {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  flex: none;
  flex-wrap: wrap;
  min-height: 40px;
  padding: var(--space-6) var(--space-8);
}
.scr-bar__grow {
  flex: 1 1 0;
  min-width: var(--space-8);
}
.scr-bar__count {
  color: var(--text-mid-emphasis);
  white-space: nowrap;
}

.scr-search {
  position: relative;
}
.scr-search .scr-input {
  width: 180px;
  max-width: 40vw;
  padding-left: 28px;
}
.scr-search .scr-icon {
  position: absolute;
  left: var(--space-8);
  top: 7px;
  color: var(--icons-mid-emphasis);
  pointer-events: none;
}

/* A dropdown button: name, then the chosen value. */
.scr-btn.-dd {
  font-weight: 400;
}
.scr-dd__name {
  color: var(--text-mid-emphasis);
}
.scr-dd__value {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-btn.-dd .scr-icon {
  width: 12px;
  height: 12px;
  margin-left: -2px;
  color: var(--icons-mid-emphasis);
}
.scr-btn.-add {
  font-weight: 400;
  color: var(--text-mid-emphasis);
}
.scr-btn.-add:hover,
.scr-btn.-add[aria-expanded='true'] {
  color: var(--text-high-emphasis);
}

/* The gaps between the parts of a phrase show the bar through; no drawn lines. */
.scr-phrase {
  display: inline-flex;
  align-items: stretch;
  gap: 1px;
  flex: none;
  height: 28px;
  border-radius: var(--radius-8);
  overflow: hidden;
}
.scr-phrase > * {
  display: inline-flex;
  align-items: center;
  height: 28px;
  padding: 0 var(--space-8);
  border: 0;
  background: var(--card-neutral-background);
  color: var(--text-mid-emphasis);
  font: inherit;
  white-space: nowrap;
}
.scr-phrase__name {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-phrase__part {
  cursor: pointer;
}
.scr-phrase__part:hover,
.scr-phrase__part[aria-expanded='true'] {
  background: color-mix(in srgb, var(--text-high-emphasis) 14%, var(--card-neutral-background));
  color: var(--text-high-emphasis);
}
.scr-phrase__part.-value {
  color: var(--text-high-emphasis);
  font-weight: 500;
}
.scr-phrase__part.-empty {
  color: var(--text-low-emphasis);
}
.scr-phrase__remove {
  padding: 0 var(--space-6);
  color: var(--icons-mid-emphasis);
  cursor: pointer;
}
.scr-phrase__remove:hover {
  color: var(--text-error-accent);
}
.scr-phrase__remove .scr-icon {
  width: 12px;
  height: 12px;
}
.scr-phrase :focus-visible {
  outline: 1px solid var(--control-focus-border);
  outline-offset: -1px;
}

@container (max-width: 720px) {
  .scr-search .scr-input {
    width: 140px;
  }
}
</style>
