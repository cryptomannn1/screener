# Screener widget — handover to the terminal team

## What this folder is

`screener/widget/` is a self-contained Vue 3 widget (plain JS, `<script setup>`,
no runtime dependency besides Vue) that renders the market screener fed by the
Go service in this repository (`GET /screener`, contract: `../api/screener.md`).
It was built from the approved HTML prototype (session 016, `screener-v1.html`)
and follows terminal-front's table conventions and design tokens, so that it can
be dropped into `terminal-front` as a widget type.

```
widget/
├── HANDOVER.md, README.md
├── src/
│   ├── ScreenerWidget.vue        the widget: props, events, wiring
│   ├── columns.js                column catalogue (keys = contract fields, titles, units)
│   ├── format.js                 cell formatting, filter value parse / format
│   ├── presets.js                built-in presets and quick filters
│   ├── screener.css              shared primitives (`scr-btn`, `scr-input`, `scr-icon`)
│   ├── api/
│   │   ├── serverSource.js       GET /screener polling, optional SSE /stream
│   │   ├── mockSource.js         deterministic synthetic rows, every field filled
│   │   └── index.js              createSource(config)
│   ├── composables/
│   │   ├── useScreenerData.js    runs the source: rows, meta, status
│   │   ├── useScreenerSettings.js  persisted state ↔ `settings` / `update:settings`
│   │   ├── useScreenerRows.js    search, filters, preset, sort, preset counts
│   │   └── usePopover.js         one popover at a time
│   └── components/
│       ├── ScreenerToolbar.vue   presets, search, period switch, Filters / Columns
│       ├── FilterStrip.vue       filter chips, sort in words, save as preset
│       ├── ScreenerTable.vue     sticky header, pinned Symbol / actions, sort, rows
│       ├── ScreenerCell.vue      one formatted metric (text, badge, bar, dash)
│       ├── Sparkline.vue         24 hourly closes
│       ├── StatusLine.vue        Instruments, Shown, Updated, coverage
│       ├── PopoverSurface.vue    fixed-position dropdown surface (Teleport to body)
│       ├── FiltersPopover.vue, ColumnsPopover.vue, RowMenu.vue
│       └── ScreenerIcon.vue      the few inline glyphs
└── sandbox/                      Vite app on port 5190, see README.md
```

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
    :instrument-id="market.instrumentId"
    @update:settings="setSettings"
    @select="setInstrument"
    @trade="openOrderForm"
  />
</template>

<script setup>
import ScreenerWidget from './ScreenerWidget.vue'
import { useWidget } from '@/composables/useWidget'

const props = defineProps({ widgetId: { type: String, required: true } })
const { settings, setSettings, market, setInstrument } = useWidget(() => props.widgetId)
const source = { kind: 'server', baseUrl: import.meta.env.VITE_SCREENER_URL, pollMs: 5000 }
function openOrderForm(instId) { setInstrument(instId) /* then focus the order form [?] */ }
</script>
```

- **`select` → link group.** `setInstrument(instId)` from `useWidget` calls
  `instrumentSwitch.switchInstrument(widgetId, instId)`, which moves the whole
  colour group when the widget is linked (`settings.linkGroup`) and only this
  widget otherwise — exactly the "click a row switches chart, order book and
  order form" behaviour from the prototype. Instrument ids are OKX
  `BTC-USDT-SWAP`; terminal instrument ids may need a mapping through the
  instruments store [?].
- **Settings persistence.** The widget keeps all its state in one flat object
  (`period`, `preset`, `filters`, `sort`, `columns`, `watchlist`,
  `customPresets`, `selected`) and emits `update:settings` with the changed
  keys; `setSettings(partial)` from `useWidget` is the matching writer. Other
  keys the terminal stores in `widget.settings` (`linkGroup`) are left alone.
  Note `workspace.setWidgetSettings` is a user edit, so it syncs to the account.
- **Highlighted row.** `instrumentId` (the frame's `market.instrumentId`)
  highlights that row; without it the widget falls back to its own `selected`.
- **`trade`.** Emitted from the row menu's "Trade" after `select`; the terminal
  decides what "opens order form" means [?].
- **`notify`.** Short messages ("Copied BTC-USDT-SWAP"); route to `useToastNew`.
- **Also to touch:** `src/views/design-system/widgetGalleryCatalog.js` if you
  want it in the gallery (`['screener', 1080, 480]`), and the layout presets if
  it should appear in a default workspace.

### Props

| prop           | type   | default            | meaning |
|----------------|--------|--------------------|---------|
| `source`       | Object | `{ kind: 'mock' }` | `{ kind: 'server', baseUrl, pollMs = 5000, sse = false }`, `{ kind: 'mock', pollMs, seed }`, or any object with `start(onRows, onError)` / `stop()` |
| `settings`     | Object | `{}`               | persisted state, see above |
| `instrumentId` | String | `''`               | the host's instrument, highlights the row |

Events: `select(instId)`, `trade(instId)`, `update:settings(partial)`,
`notify(message)`.

### API base URL

`serverSource.js` fetches `${baseUrl}/screener` (no trailing slash) every
`pollMs`; with `sse: true` it opens `EventSource(${baseUrl}/stream)` and listens
to `rows` events, keeping polling as the fallback when the stream errors. The
service answers with `Access-Control-Allow-Origin: *`. In the terminal the URL
should come from the environment or `src/api/index.ts` rather than be hardcoded
[?]; the sandbox defaults to `http://localhost:8090`.

## Design tokens used

Only token names, read through `var(--…)`; the terminal's `_theme.scss` applies
as is. Colours: `--text-high-emphasis`, `--text-mid-emphasis`,
`--text-low-emphasis`, `--text-light`, `--text-error-accent`,
`--text-attention-accent`, `--icons-mid-emphasis`, `--icons-low-emphasis`,
`--card-background`, `--card-neutral-background`,
`--card-primary-background-light`, `--card-error-background-light`, `--border`,
`--long`, `--short`, `--widget-background`, `--widget-header-background`,
`--primary-accent`, `--primary-deep`, `--control-focus-border`,
`--dropdown-background`, `--dropdown-hover`, `--dropdown-border`,
`--dropdown-shadow`, `--terminal-v2-text`, `--terminal-v2-muted`,
`--table-pin-shadow`. Layout: `--space-2…32`, `--radius-4/6/8/circle`,
`--layer-raised`, `--layer-sticky`, `--layer-dropdown`, `--duration-fast`.

Typography is `fz-12` (12px/16px, tabular-nums) inherited from the host's font
(Geist on `body`). Rows are 38px, the header 32px on
`--widget-header-background`; there are no borders between rows or columns.

Styles are plain scoped CSS (the sandbox has no Sass). When moving into the
terminal you may convert to SCSS with the `$token` aliases from `_tokens.scss`;
the names map one to one.

### Where the terminal kit should replace local pieces

- `PopoverSurface.vue` → `PopperNew` + `DropdownSurface` / `DropdownMenuItem`
  (teleport target and z-index via `useTeleportTarget` / `WIDGET_SETTINGS_Z_INDEX`).
  The local surface uses `--layer-dropdown` (100), which sits under a
  maximized frame (`--layer-overlay` 500) [?].
- `ScreenerIcon.vue` (inline SVG) → `IconNew`: search, star, filter, columns,
  more, close, check, up, down. AGENTS.md forbids inline SVG in the terminal.
- `.scr-btn` / `.scr-input` → `ButtonNew size="sm"` and the kit input; the
  period switch and preset tabs follow `PositionsWidget`'s tab styling.
- `title` attributes on headers, cells and menu items are the tooltips; the
  terminal's `TitleTooltip` picks them up.

## Behaviour summary

- Toolbar, left to right: search, (preset tabs), spacer, period switch,
  Filters, Columns.
- Presets: only "All" ships (the owner removed Movers, Squeeze, Strong vs BTC
  and the rest for now); the mechanism stays in `presets.js`. The tabs row is
  drawn only when there is more than one preset, which happens once the user
  saves a filter set with "Save as preset" (custom presets live in settings,
  tabs show live counts, search excluded). A saved preset id that no longer
  exists reads as All.
- Search accepts several tickers separated by commas (`btc, eth`), matching
  symbol or instrument id.
- Period 1h / 4h / 24h; 5m / 15m shown disabled.
- Sort on numeric headers cycles desc → asc → by size (signed metrics only)
  → desc; `null` always sorts last.
- Filters: `>=` / `<=` on any numeric column, percent typed as percent, money
  with K / M / B. One condition per metric and direction.
- Row click emits `select`; the row menu offers Show on chart, Trade,
  watchlist toggle, Set alert (disabled, v2), Copy symbol. Starred rows float
  to the top of the All preset; there is no Watchlist tab.
- Status line: Instruments, Shown (with "first 600 drawn" when cut), Updated
  (the response `ts`; the source name is its tooltip), and coverage as
  `Candles n/N, daily n/N, books n/N, OI Δ n/N, flow n/N` with fully covered
  families omitted. `flow` is counted from `cov.flow` per row.
- States: "Loading instruments" before the first rows, "`<source>` unreachable,
  retrying" when the fetch fails with nothing shown yet (error text in the
  tooltip), "No instruments match the filters" inside the table.

## Contract notes (server vs prototype)

Followed the server where the prototype differed: `chg1d` is against the UTC
daily close, `toHigh` / `toLow` against the 30-day high / low (both as
`last / level − 1`, so `toHigh` is negative below the high), `atr` is one value
(ATR 14 on 1H / last), `regime` thresholds 0.6 / 1.6, `rv24` from hourly log
returns, beta / corr over 168 bars. Extra contract fields exposed as optional
columns: `last`, `depth1`.

Levels (owner request, 2026-10-31): six row fields `do`, `wo`, `mo`, `ma50`,
`ma100`, `ma200`, each `{ px, dist }` with `dist = last / px − 1` and `px`
null when unknown. Column group "Levels" (DO, WO, MO, MA 50, MA 100, MA 200)
sits between "Strength vs BTC" and "Order book"; cells show `dist` as a signed
percent, the tooltip shows the level price ("Level 77 450.3"), sorting is by
`dist`. MA 100 is hidden by default. The widget reads the fields as soon as the
server sends them; until then they are dashes. A `settings.columns` list saved
before this change does not include the new defaults — "Reset to default" in
the Columns popover does.

Observed on the running service (2026-10-31): `spark` is already present;
`oiD` is `null` everywhere (`oiHist` coverage 0 until the service has run an
hour); `takerBuy` / `delta` null (phase 2); `fundingNext` null; `presence` /
`stay` null for the benchmark; `depth1` null for some rows. All render as dashes.

Ambiguities: `coverage` has no `flow` count (derived from rows here); `listed`
is fractional days (floored for display); `vsBtc` for the benchmark itself is
`0`, not `null`.

## What is mocked, what is not done

- `mockSource.js` fills every field including phase-2 ones and `spark`.
- Not done: alerts (menu item disabled), 5m / 15m periods, column reorder and
  resize, virtual scrolling (the table draws at most 600 rows), keyboard
  navigation of rows, a settings panel for the frame's gear.
- Not verified: behaviour inside the real terminal (teleport target, z-index
  under a maximized frame, instrument id mapping) [?]; SSE against a live
  `/stream` (endpoint not up yet, the code path is exercised only by its polling
  fallback) [?].
