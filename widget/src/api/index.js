// Turns the widget's `source` prop into a running source. The prop is either
// a config `{ kind: 'server' | 'static', ...options }` or an object that already
// has `start` and `stop` (a source the host app built itself).

import { createAssetsSource } from './assetsSource.js'
import { createStaticSource } from './staticSource.js'

export function createSource(config = {}) {
  if (typeof config.start === 'function' && typeof config.stop === 'function') return config
  const { kind = 'server', ...options } = config
  return kind === 'static' ? createStaticSource(options) : createAssetsSource(options)
}

export { createAssetsSource, createStaticSource }
