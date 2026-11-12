// Fixed vocabulary of the screener: exchanges, asset classes, price levels and
// the words a level condition is written in.

/** Exchange code in the `/assets` payload and its display name, in display order. */
export const EXCHANGES = [
  ['okx', 'OKX'],
  ['bn', 'Binance'],
  ['bb', 'Bybit'],
]
export const EXCHANGE_NAME = Object.fromEntries(EXCHANGES)

export const CLASSES = [
  ['all', 'All'],
  ['crypto', 'Crypto'],
  ['stock', 'Stocks'],
  ['commodity', 'Commodities'],
  ['fx', 'FX'],
]

export const LEVELS = [
  ['do', 'DO'],
  ['wo', 'WO'],
  ['mo', 'MO'],
  ['ma50', 'MA50'],
  ['ma100', 'MA100'],
  ['ma200', 'MA200'],
]
export const LEVEL_TITLE = {
  do: "Today's open (UTC): its price and the distance to it",
  wo: 'Open of the week (Monday UTC): its price and the distance to it',
  mo: 'Open of the month (UTC): its price and the distance to it',
  ma50: 'MA 50 of daily closes: its price and the distance to it',
  ma100: 'MA 100 of daily closes: its price and the distance to it',
  ma200: 'MA 200 of daily closes: its price and the distance to it',
}

/** "Crossed" means the hourly closes flipped sides within this many hours. */
export const CROSS_HOURS = 4
export const LEVEL_MODES = [
  ['above', 'Above'],
  ['near', 'Near'],
  ['below', 'Below'],
  ['up', `Crossed ↑ ${CROSS_HOURS}h`],
  ['down', `Crossed ↓ ${CROSS_HOURS}h`],
]
export const levelModeLabel = (mode) => LEVEL_MODES.find((m) => m[0] === mode)?.[1] ?? mode

/** Range of "near": percent of price, or half of the hourly ATR. */
export const NEAR_RANGES = [
  ['0.25', '0.25%'],
  ['0.5', '0.5%'],
  ['1', '1%'],
  ['atr', '½ ATR'],
]

/** A condition reads as a phrase: column, operator, value. */
export const OPERATORS = [
  ['>=', 'above'],
  ['<=', 'below'],
  ['between', 'between'],
]
export const operatorLabel = (op) => OPERATORS.find((o) => o[0] === op)?.[1] ?? op
