// Turns the widget's `source` prop into a running source. The prop is either
// a config `{ kind: 'server' | 'mock', ...options }` or an object that already
// has `start` and `stop` (a source the host app built itself).

import { createMockSource } from './mockSource.js'
import { createServerSource } from './serverSource.js'

export function createSource(config = {}) {
  if (typeof config.start === 'function' && typeof config.stop === 'function') return config
  const { kind = 'mock', ...options } = config
  return kind === 'server' ? createServerSource(options) : createMockSource(options)
}

export { createMockSource, createServerSource }
