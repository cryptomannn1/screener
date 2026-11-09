-- screener ClickHouse schema. Applied by store.Open with CREATE ... IF NOT
-- EXISTS, {ttl} is replaced by Config.TTLDays. Comment lines are stripped first,
-- then statements are split on semicolons.

-- Per-minute snapshot of the batch endpoints: price, volume, OI, funding.
-- The OI history of the screener (oiD per period) is rebuilt from here on
-- restart, and the strength backtest reads prices from candles instead.
CREATE TABLE IF NOT EXISTS ticks (
  ts            DateTime('UTC'),
  inst_id       LowCardinality(String),
  last          Float64,
  mark          Float64,
  index_px      Float64,
  bid           Float64,
  ask           Float64,
  vol_ccy_24h   Float64,
  vol_quote_24h Float64,
  oi_ccy        Float64,
  oi_usd        Float64,
  funding       Float64,
  next_funding  Float64
)
ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (inst_id, ts)
TTL ts + INTERVAL {ttl} DAY;

-- OKX candles as fetched, one row per (instrument, bar size, open time).
-- A re-fetch of the same bar replaces the previous row on merge, so the
-- forming bar converges to its final values.
CREATE TABLE IF NOT EXISTS candles (
  inst_id   LowCardinality(String),
  bar       LowCardinality(String),
  ts        DateTime('UTC'),
  open      Float64,
  high      Float64,
  low       Float64,
  close     Float64,
  vol_ccy   Float64,
  vol_quote Float64,
  confirm   UInt8,
  fetched   DateTime('UTC')
)
ENGINE = ReplacingMergeTree(fetched)
PARTITION BY toYYYYMM(ts)
ORDER BY (inst_id, bar, ts)
TTL ts + INTERVAL 730 DAY;

-- Order-book depth bands per sweep (about every 30 s), quote currency.
CREATE TABLE IF NOT EXISTS depth (
  ts         DateTime('UTC'),
  inst_id    LowCardinality(String),
  mid        Float64,
  spread_bps Float64,
  bid01      Float64,
  ask01      Float64,
  bid05      Float64,
  ask05      Float64,
  bid1       Float64,
  ask1       Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (inst_id, ts)
TTL ts + INTERVAL {ttl} DAY;

-- The computed screener row once a minute: what the widget showed, for
-- preset analytics and the strength backtest. Per-period metrics are stored
-- for the 1h / 4h / 24h horizons in fixed columns.
CREATE TABLE IF NOT EXISTS rows (
  ts          DateTime('UTC'),
  inst_id     LowCardinality(String),
  last        Float64,
  mark        Float64,
  basis       Float64,
  chg_1h      Float64,
  chg_4h      Float64,
  chg_24h     Float64,
  chg_1d      Float64,
  chg_7d      Float64,
  chg_30d     Float64,
  vol24       Float64,
  volx7       Float64,
  volx_1h     Float64,
  volx_4h     Float64,
  volx_24h    Float64,
  oi          Float64,
  oid_1h      Float64,
  oid_4h      Float64,
  oid_24h     Float64,
  oi_vol      Float64,
  funding     Float64,
  funding_ann Float64,
  atr         Float64,
  rv24        Float64,
  regime      Float64,
  range1d     Float64,
  to_high     Float64,
  to_low      Float64,
  vsbtc_1h    Float64,
  vsbtc_4h    Float64,
  vsbtc_24h   Float64,
  beta        Float64,
  alpha       Float64,
  presence    Float64,
  stay        Float64,
  corr        Float64,
  depth01     Float64,
  depth05     Float64,
  depth1      Float64,
  imb         Float64,
  spread      Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (inst_id, ts)
TTL ts + INTERVAL {ttl} DAY;

-- Taker flow per closed minute from the trade stream (phase 2): quote volume
-- bought and sold by takers and the number of taker orders. Feeds takerBuy,
-- delta and tape per period, and the backtest's flow signals.
CREATE TABLE IF NOT EXISTS flow_1m (
  ts         DateTime('UTC'),
  inst_id    LowCardinality(String),
  buy_quote  Float64,
  sell_quote Float64,
  trades     UInt32,
  buy_trades UInt32
)
ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (inst_id, ts)
TTL ts + INTERVAL {ttl} DAY;

-- Levels (v0.3): distances of last from the day / week / month open and the
-- daily MA 50 / 100 / 200. Added to an existing table with ALTER so an old
-- deployment upgrades in place.
ALTER TABLE rows ADD COLUMN IF NOT EXISTS do_dist Float64;
ALTER TABLE rows ADD COLUMN IF NOT EXISTS wo_dist Float64;
ALTER TABLE rows ADD COLUMN IF NOT EXISTS mo_dist Float64;
ALTER TABLE rows ADD COLUMN IF NOT EXISTS ma50_dist Float64;
ALTER TABLE rows ADD COLUMN IF NOT EXISTS ma100_dist Float64;
ALTER TABLE rows ADD COLUMN IF NOT EXISTS ma200_dist Float64;

-- Assets (v0.4): market cap, FDV, rank and sector per crypto base symbol from
-- the metadata resolver, kept so a restart needs no external calls.
CREATE TABLE IF NOT EXISTS assets (
  symbol  String,
  cg_id   String,
  name    String,
  mcap    Float64,
  fdv     Float64,
  rank    UInt32,
  sector  LowCardinality(String),
  updated DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(updated)
ORDER BY symbol;

ALTER TABLE rows ADD COLUMN IF NOT EXISTS class LowCardinality(String);
ALTER TABLE rows ADD COLUMN IF NOT EXISTS sector LowCardinality(String);
ALTER TABLE rows ADD COLUMN IF NOT EXISTS mcap Float64;
