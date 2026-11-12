# Screener — handover to the terminal team

## What this folder is

In the terminal the screener is a page of its own, next to the trading
workspace, not a widget of the workspace grid. The folder and the component
keep the name they were started with (`widget/`, `ScreenerWidget.vue`); rename
them when moving the code if you prefer.

`screener/widget/` is a self-contained Vue 3 component (plain JS, `<script setup>`,
no runtime dependency besides Vue) that renders the market screener fed by the
Go service in this repository: one table of perpetual contracts over OKX,
Binance and Bybit (`GET /assets` and `GET /stream?view=assets`, contract:
`../api/screener.md`). It is a port of the HTML prototype
(`../prototype/screener-v2.html`, source `../prototype/src/proto2-template.html`)
and follows terminal-front's table conventions and design tokens, so that it can
be dropped into `terminal-front` as the content of that page.

```
widget/
├── HANDOVER.md, README.md
├── src/
│   ├── ScreenerWidget.vue        the screener: props, events, wiring
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
user's choice, so the component adds the blocks up itself (`model/aggregate.js`):

- volume and open interest are sums over the chosen exchanges;
- price and the 24h change come from the busiest of them;
- funding is brought to 8 hours on every exchange and weighted by open interest;
- strength against BTC over 24h uses the same exchange choice.

A hover on Price, Chg 24h, Vol 24h, OI or Funding shows the figure on each
exchange that went into the cell.

## Mounting it in terminal-front

Copy `src/` to `src/components/screener/`, add a view that hosts it and a
route next to the terminal's own:

```js
// src/router/index.ts
{
  path: '/screener',
  name: 'screener',
  component: () => import('@/views/ScreenerView.vue'),
},
```

```vue
<!-- src/views/ScreenerView.vue -->
<template>
  <main class="screener-view">
    <ScreenerWidget
      :source="source"
      :settings="settings"
      @update:settings="saveSettings"
      @select="onSelect"
    />
  </main>
</template>

<script setup>
import ScreenerWidget from '@/components/screener/ScreenerWidget.vue'

const source = { kind: 'server', baseUrl: import.meta.env.VITE_SCREENER_URL }
// settings: the page's own key in /settings, read once and written back on change [?]
// onSelect: see `select` below
</script>

<style scoped>
.screener-view { height: 100%; display: flex; flex-direction: column; min-height: 0; }
</style>
```

The component fills the box it is given and draws no frame of its own: no
border, no title bar, no outer padding. The page gives it the full height
under the application header.

- **Navigation.** The page needs an entry in the terminal's main navigation
  and a way back to the workspace [?].
- **`select`.** A row click emits
  `{ key, symbol, name, cls, primary, venues }`: the asset key of the service
  (`c:BTC`), its ticker, the busiest of the chosen exchanges (`okx` | `bn` |
  `bb`) and all exchanges it trades on. The payload carries no exchange
  instrument id: `/assets` does not send one, so the page maps ticker and
  exchange to a terminal instrument through the instruments store [?].
  On the page a row click does not switch a chart: it opens the asset panel
  (below), and the panel's "Open chart" goes to `/terminal` with that
  instrument [?].
- **Asset panel.** Designed as a prototype, not in this component yet: a side
  panel over the right part of the page with a plain-language description of
  the project, its links, the figures per exchange, movement over 1h / 4h /
  24h, levels and market data. It needs an endpoint of the service that is not
  written yet (`../PLAN.md`, stage 2).
- **Highlighted row.** The component keeps the clicked row in its own
  `selected`. The `symbol` prop highlights a row from outside; a page has no
  instrument of its own, so leave it empty.
- **Settings persistence.** The component keeps its state in one flat object
  and emits `update:settings` with the changed keys, at most once per 300 ms.
  Keys: `exchanges` (`{ okx, bn, bb }`), `cls` (asset class), `tab`,
  `selected`, and `views` (tab id → columns, order, conditions, sort). `views`
  holds only the tabs the user changed, so a ready table follows its
  definition in `tabs.js` until the user makes it their own. Store the object
  under one key of the user's `/settings`, as the control bar does [?]. A
  settings change that comes from outside (another device, a reset) is
  applied; the component's own echo is ignored. Search is not persisted.

### Props

| prop       | type   | default              | meaning |
|------------|--------|----------------------|---------|
| `source`   | Object | `{ kind: 'server' }` | `{ kind: 'server', baseUrl, pollMs = 5000, sse = true }`, `{ kind: 'static', data \| load }`, or any object with `start(onData, onError)` / `stop()` |
| `settings` | Object | `{}`                 | persisted state, see above |
| `symbol`   | String | `''`                 | ticker to highlight from outside; empty on the page |

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

Styles are plain CSS, scoped where the markup stays inside the component and
global (prefixed `scr-` / `screener__`) for what is teleported to `body`. When
moving into the terminal you may convert to SCSS with the `$token` aliases from
`_tokens.scss`; the names map one to one.

### Where the terminal kit should replace local pieces

- `PopoverSurface.vue` → `PopperNew` + `DropdownSurface` / `DropdownMenuItem`
  (teleport target and z-index via `useTeleportTarget` / `WIDGET_SETTINGS_Z_INDEX`).
  The local surface uses `--layer-dropdown` (100); check it against the
  page's own layers [?].
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
  the free width equally. A few columns fill the page, more columns narrow
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
  version had them, the current design does not.
- The asset panel: prototype only.
- User-made tabs ("save this table as a tab").
- Not verified: behaviour inside the real terminal (teleport target, layers,
  mapping of an asset to a terminal instrument) [?].
- The flow, order-book and OI-change columns are empty for assets OKX does
  not list; this is a limit of the service, see `../PLAN.md`.
