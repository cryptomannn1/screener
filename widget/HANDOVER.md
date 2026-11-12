# Screener widget — handover to the terminal team

## What this folder is

`screener/widget/` is a self-contained Vue 3 widget (plain JS, `<script setup>`,
no runtime dependency besides Vue) that renders the market screener fed by the
Go service in this repository: one table of perpetual contracts over OKX,
Binance and Bybit (`GET /assets` and `GET /stream?view=assets`, contract:
`../api/screener.md`). It is a port of the HTML prototype
(`../prototype/screener-v2.html`, source `../prototype/src/proto2-template.html`)
and follows terminal-front's table conventions and design tokens, so that it can
be dropped into `terminal-front` as a widget type.

```
widget/
├── HANDOVER.md, README.md
├── src/
│   ├── ScreenerWidget.vue        the widget: props, events, wiring
│   ├── columns.js                column catalogue: value, cell, unit, quick thresholds; the Columns catalogue
│   ├── tabs.js                   ready tables (Overview, Anomalies, Strong vs BTC, Weak vs BTC), view normalisation
│   ├── filters.js                conditions, search, order, how a threshold is typed and shown
│   ├── format.js                 number formatting
│   ├── screener.css              shared primitives (`scr-btn`, `scr-input`, `scr-icon`)
│   ├── model/
│   │   ├── aggregate.js          one row per asset over the chosen exchanges
│   │   ├── levels.js             price against a level, recent crosses
│   │   └── constants.js          exchanges, classes, levels, operators
│   ├── api/
│   │   ├── assetsSource.js       GET /assets, SSE /stream?view=assets, polling as the fallback
│   │   ├── staticSource.js       a fixed payload, for a sandbox or a test
│   │   └── index.js              createSource(config)
│   ├── composables/
│   │   ├── useAssets.js          runs the source: assets, ts, tick, status
│   │   ├── useScreenerState.js   persisted state ↔ `settings` / `update:settings`
│   │   ├── usePopover.js         one popover at a time, anchored by `data-pop` / `data-key`
│   │   └── useDragReorder.js     drag to reorder (header cells, the Columns list)
│   └── components/
│       ├── ScreenerTabs.vue      the tabs with live counts
│       ├── ScreenerBar.vue       search, Exchanges, Assets, conditions as phrases, Add filter, Columns
│       ├── ScreenerTable.vue     adaptive widths, sticky header, pinned Symbol, sort, hover card
│       ├── PriceCell.vue         the price as a running figure
│       ├── VenueIcon.vue         exchange marks
│       ├── PopoverSurface.vue    fixed-position dropdown surface (Teleport to body)
│       ├── OptionList.vue, AddFilterMenu.vue, ValueEditor.vue, ColumnsPanel.vue
│       └── ScreenerIcon.vue      the few inline glyphs
└── sandbox/                      Vite app on port 5190, see README.md
```

## Data model

The service sends one entry per asset with a block per exchange (`v.okx`,
`v.bn`, `v.bb`: price, 24h change, volume, open interest, funding) and the
metrics computed once per asset (`m`). Which exchanges go into a row is the
user's choice, so the widget adds the blocks up itself (`model/aggregate.js`):

- volume and open interest are sums over the chosen exchanges;
- price and the 24h change come from the busiest of them;
- funding is brought to 8 hours on every exchange and weighted by open interest;
- strength against BTC over 24h uses the same exchange choice.

A hover on Price, Chg 24h, Vol 24h, OI or Funding shows the figure on each
exchange that went into the cell.

## Mounting it in terminal-front

Copy `src/` to `src/components/terminal/widgets/screener/` and register the
type in `src/components/terminal/widgets/registry.js` (one entry, like the
others):

```js
screener: {
  type: 'screener',
  title: 'Screener',
  icon: 'Table',                      // any SVG/ name you prefer [?]
  group: 'market-data',
  component: markRaw(defineAsyncComponent(() => import('./screener/ScreenerHost.vue'))),
  settingsComponent: null,
  defaultSize: { w: 24, h: 10 },
  minSize: { w: 10, h: 5 },
  usesMarket: true,                   // gives the frame a symbol and the link-group menu
  defaultSettings: {},
},
```

Widgets in the terminal receive only `widgetId`, so wrap the widget in a small
host component that reads `useWidget`:

```vue
<!-- ScreenerHost.vue -->
<template>
  <ScreenerWidget
    :source="source"
    :settings="settings"
    :symbol="baseTicker"
    @update:settings="setSettings"
    @select="onSelect"
  />
</template>

<script setup>
import { computed } from 'vue'
import ScreenerWidget from './ScreenerWidget.vue'
import { useWidget } from '@/composables/useWidget'

const props = defineProps({ widgetId: { type: String, required: true } })
const { settings, setSettings, market, setInstrument } = useWidget(() => props.widgetId)
const source = { kind: 'server', baseUrl: import.meta.env.VITE_SCREENER_URL }
const baseTicker = computed(() => market.value.instrumentId?.split('-')[0] ?? '')   // [?] via the instruments store
function onSelect(asset) { /* map asset.symbol + asset.primary to a terminal instrument id, then setInstrument(id) [?] */ }
</script>
```

- **`select`.** A row click emits
  `{ key, symbol, name, cls, primary, venues }`: the asset key of the service
  (`c:BTC`), its ticker, the busiest of the chosen exchanges (`okx` | `bn` |
  `bb`) and all exchanges it trades on. The payload carries no exchange
  instrument id: `/assets` does not send one, so the host maps ticker and
  exchange to its own instrument through the instruments store [?].
  `setInstrument` then moves the link group as for any other widget.
- **Highlighted row.** `symbol` (the ticker of the frame's instrument)
  highlights that row; without it the widget falls back to its own `selected`.
- **Settings persistence.** The widget keeps its state in one flat object and
  emits `update:settings` with the changed keys, at most once per 300 ms;
  `setSettings(partial)` from `useWidget` is the matching writer. Keys:
  `exchanges` (`{ okx, bn, bb }`), `cls` (asset class), `tab`, `selected`, and
  `views` (tab id → columns, order, conditions, sort). `views` holds only the
  tabs the user changed, so a ready table follows its definition in `tabs.js`
  until the user makes it their own. Other keys the terminal stores in
  `widget.settings` (`linkGroup`) are left alone. A settings change that comes
  from the host (another device, a reset) is applied; the widget's own echo is
  ignored. Search is not persisted.
- **Also to touch:** `src/views/design-system/widgetGalleryCatalog.js` if you
  want it in the gallery (`['screener', 1080, 480]`), and the layout presets if
  it should appear in a default workspace.

### Props

| prop       | type   | default              | meaning |
|------------|--------|----------------------|---------|
| `source`   | Object | `{ kind: 'server' }` | `{ kind: 'server', baseUrl, pollMs = 5000, sse = true }`, `{ kind: 'static', data \| load }`, or any object with `start(onData, onError)` / `stop()` |
| `settings` | Object | `{}`                 | persisted state, see above |
| `symbol`   | String | `''`                 | ticker of the host's instrument, highlights the row |

Events: `select(asset)`, `update:settings(partial)`. Exposed: `resetTab()`,
`isTabChanged()` for a host that offers its own reset.

### API base URL

`assetsSource.js` reads `${baseUrl}/assets` once, then listens to
`EventSource(${baseUrl}/stream?view=assets)` (events named `assets`, the same
payload, about once a second). While the stream is down it polls `/assets`
every `pollMs`; the stream reconnects by itself. The table redraws at most
four times a second and not at all while the page is hidden. The service
answers with `Access-Control-Allow-Origin: *`. In the terminal the URL should
come from the environment or `src/api/index.ts` rather than be hardcoded [?];
the sandbox defaults to `http://localhost:8090`.

## Design tokens used

Only token names, read through `var(--…)`; the terminal's `_theme.scss` applies
as is. Colours: `--text-high-emphasis`, `--text-mid-emphasis`,
`--text-low-emphasis`, `--text-light`, `--text-error-accent`,
`--icons-mid-emphasis`, `--icons-low-emphasis`, `--card-background`,
`--card-neutral-background`, `--card-primary-background-light`, `--border`,
`--long`, `--short`, `--widget-background`, `--widget-header-background`,
`--primary-accent`, `--primary-deep`, `--control-focus-border`,
`--dropdown-background`, `--dropdown-hover`, `--dropdown-border`,
`--dropdown-shadow`, `--terminal-v2-text`, `--terminal-v2-muted`,
`--table-pin-shadow`. Layout and motion: `--space-2…32`, `--radius-4/6/8/circle`,
`--layer-raised`, `--layer-sticky`, `--layer-dropdown`, `--layer-tooltip`,
`--duration-fast`, `--duration-base`, `--ease-standard`.

Typography is `fz-12` (12px/16px, tabular-nums) inherited from the host's font
(Geist on `body`). Rows are 38px, the header 32px on
`--widget-header-background`; there are no borders between rows or columns.

Styles are plain CSS, scoped where the markup stays inside the widget and
global (prefixed `scr-` / `screener__`) for what is teleported to `body`. When
moving into the terminal you may convert to SCSS with the `$token` aliases from
`_tokens.scss`; the names map one to one.

### Where the terminal kit should replace local pieces

- `PopoverSurface.vue` → `PopperNew` + `DropdownSurface` / `DropdownMenuItem`
  (teleport target and z-index via `useTeleportTarget` / `WIDGET_SETTINGS_Z_INDEX`).
  The local surface uses `--layer-dropdown` (100), which sits under a
  maximized frame (`--layer-overlay` 500) [?].
- `PriceCell.vue` → `AnimatedNumber` with `tone="none"` (the control bar's
  running figure); the local cell repeats its 200 ms tween and colour flash.
- `ScreenerIcon.vue`, `VenueIcon.vue` (inline SVG) → `IconNew` and the
  terminal's exchange icons. AGENTS.md forbids inline SVG in the terminal.
- `.scr-btn` / `.scr-input` → `ButtonNew size="sm"` and the kit input; the tabs
  follow `PositionsWidget`'s tab styling.
- `useDragReorder.js` repeats the pointer pattern of the Positions table
  (4px threshold, accent source, edge line on the target); use the terminal's
  own helper if it is shared.
- `title` attributes on headers, cells and menu items are the tooltips; the
  terminal's `TitleTooltip` picks them up.

## Behaviour summary

- **Tabs.** Overview, Anomalies, Strong vs BTC, Weak vs BTC. A tab is its
  columns, their order, its conditions and its sort; exchanges, asset class
  and search are shared. The number on a tab is how many assets match its
  conditions now; a tab without conditions has no number. What is changed in
  a tab stays in that tab.
- **Bar**, left to right: search (several tickers or names separated by
  commas), Exchanges (checklist, one always stays on), Assets (class with
  counts; a class the chosen exchanges do not list is disabled), the
  conditions, Add filter, then "N of M", Reset filters and Columns.
- **Conditions are phrases**: column, operator, value, remove. Number columns
  read "above / below / between" with a typed value (`20m`, `1.5b`, `3` for
  percent) or a one-click usual threshold; the table answers while the value
  is typed. Level columns read "above / near / below / crossed ↑ / crossed ↓"
  with the range of "near" as a second part. Sector reads "is". Add filter
  offers the columns of the table that have no condition yet; a phrase left
  without a value goes away when its popover closes. Reset filters returns the
  conditions the tab came with.
- **Columns** popover: the table's columns on the left (drag to reorder, cross
  to remove), the catalogue on the right (metric families with a one-line
  description, periods as chips). Removing a column removes its condition and
  its sort. Reset returns the tab as it ships. Header cells reorder by
  dragging too.
- **Widths.** Symbol and text columns keep their width; number columns share
  the free width equally. A few columns fill the widget, more columns narrow
  down to their minimum, and only then the table scrolls sideways with Symbol
  pinned.
- **Sort.** A header click: descending, ascending, back to the default order
  (text columns start ascending). The default order is by 24h volume, or by
  distance when a "near" condition is set, or by the latest cross.
  Empty values always go last.
- **Price** runs to its new value on a live tick and flashes the side's colour;
  a redraw the user caused never flashes.
- States: "Loading assets" before the first payload, "`<host>` unreachable,
  retrying" when the fetch fails with nothing shown yet (error text in the
  tooltip), "Nothing matches" inside the table.

## What is not done

- Virtual scrolling: the table draws at most 400 rows.
- Keyboard navigation of rows and of drag reorder.
- A watchlist, row actions (trade, alert, copy) and a status line: the first
  version of the widget had them, the current design does not.
- User-made tabs ("save this table as a tab").
- Not verified: behaviour inside the real terminal (teleport target, z-index
  under a maximized frame, mapping of an asset to a terminal instrument) [?].
- The flow, order-book and OI-change columns are empty for assets OKX does
  not list; this is a limit of the service, see `../PLAN.md`.
