# Screener widget

Vue 3 widget for the Scope360 terminal that shows the market screener served by
the Go service in this repository: one table over OKX, Binance and Bybit
(`GET /assets`, `GET /stream?view=assets`, contract in `../api/screener.md`).
`src/` is the widget, `sandbox/` is a small Vite app that renders it
full-window inside a stand-in for the terminal's widget frame. `HANDOVER.md`
tells the terminal team how to mount it.

## Run the sandbox

```sh
cd widget/sandbox
npm install
npm run dev          # http://localhost:5190
```

- **Server / Snapshot** in the top bar switches the data source. *Server* reads
  `http://localhost:8090` from the Vite dev server (start the service with
  `go run ./cmd/screener` in the repository root); a build served by the Go
  service under `/ui/` talks to its own origin. Override with
  `?server=http://host:port`, and add `?sse=0` to poll `/assets` instead of
  listening to the stream.
- *Snapshot* shows the fixed payload the HTML prototype carries
  (`../prototype/src/proto2-snapshot.json`), so the widget works without the
  service.
- The theme button toggles light / dark; the sandbox stores its choice, the
  data source and the widget settings in `localStorage` under
  `screener-sandbox-v2`.

`npm run build` writes `sandbox/dist/` with relative asset paths.
