# Screener widget

Vue 3 widget for the Scope360 terminal that shows the market screener served by
the Go service in this repository (`GET /screener`, contract in
`../api/screener.md`). `src/` is the widget, `sandbox/` is a small Vite app that
renders it full-window inside a stand-in for the terminal's widget frame.
`HANDOVER.md` tells the terminal team how to mount it.

## Run the sandbox

```sh
cd widget/sandbox
npm install
npm run dev          # http://localhost:5190
```

- **Server / Mock** in the top bar switches the data source. *Server* reads
  `http://localhost:8090` from the Vite dev server; a build served by the Go
  service under `/ui/` talks to its own origin. Override with
  `?server=http://host:port`, and add `?sse=1` to subscribe to `/stream`
  instead of polling.
- *Mock* renders deterministic synthetic rows with every column filled, so the
  widget works without the service.
- The theme button toggles light / dark; the sandbox stores its choice, the
  data source and the widget settings in `localStorage` under `screener-sandbox`.

`npm run build` writes `sandbox/dist/` with relative asset paths.
