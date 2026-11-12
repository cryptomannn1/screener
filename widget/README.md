# Screener component

Vue 3 component for the screener page of the Scope360 terminal (a page of its
own, not a workspace widget; the folder keeps its first name). It shows the market screener served by
the Go service in this repository: one table over OKX, Binance and Bybit
(`GET /assets`, `GET /stream?view=assets`, contract in `../api/screener.md`).
`src/` is the component, `sandbox/` is a small Vite app that renders it
full-window inside a plain frame. `HANDOVER.md`
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
  (`../prototype/src/proto2-snapshot.json`), so the component works without the
  service.
- The theme button toggles light / dark; the sandbox stores its choice, the
  data source and the screener's settings in `localStorage` under
  `screener-sandbox-v2`.

`npm run build` writes `sandbox/dist/` with relative asset paths.
