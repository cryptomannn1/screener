# UI prototype

`screener-v2.html` is the screener as one self-contained page: a clean table
with one row per asset over OKX, Binance and Bybit. It is the UI contract for
the terminal widget.

- Opened as a file it shows the data snapshot built into it.
- Served by the service it goes live:

  ```bash
  go run ./cmd/screener -static prototype
  ```

  then http://localhost:8090/ui/screener-v2.html. The page reads `GET /assets`
  and follows `GET /stream?view=assets`.

## What is on the page

- **Tabs**: ready tables. A tab is its columns in order, its conditions and its
  sort: Overview, Anomalies, Strong vs BTC, Weak vs BTC. What is changed in a
  tab stays in that tab; the number on a tab is how many assets match its
  conditions now. `#anomalies`, `#strong`, `#weak` in the address open a tab.
- **Exchanges** and **Assets**: which exchanges go into a row and which asset
  class is shown. Volume and open interest add up over the chosen exchanges,
  price and 24 h change come from the busiest one, funding is brought to 8 h
  and weighted by open interest. Hovering a summed cell shows the figure on
  each exchange.
- **Filter phrases**: a condition reads `column | operator | value`. Add filter
  offers the columns of the table; the value is typed (`20m`, `1.5b`) or picked
  from the usual thresholds.
- **Columns**: the catalogue of metrics, drag to reorder. A few columns stretch
  over the whole width, more columns narrow down, then the table scrolls.

## Rebuilding

Edit `src/proto2-template.html`, never the built page, then:

```bash
cd src && python3 build.py
```

| file in `src/` | role |
|---|---|
| `proto2-template.html` | the page itself: markup, styles and script of this prototype |
| `lab2-template.html` | the previous lab; the build cuts shared blocks out of it (number formatting, levels, column catalogue, drag) |
| `screener-v1.html` | the first prototype; the build takes the terminal kit CSS and the icons from it |
| `proto2-snapshot.json` | the built-in data snapshot, same shape as `GET /assets` |
| `build.py` | puts them together into `../screener-v2.html` |
