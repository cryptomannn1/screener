import { fileURLToPath } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

// The widget sources live one level up from this sandbox, the prototype's snapshot two.
const repoRoot = fileURLToPath(new URL('../..', import.meta.url))

export default defineConfig({
  // Relative asset paths: the Go service can serve `dist/` under /ui/.
  base: './',
  plugins: [vue()],
  server: {
    port: 5190,
    strictPort: true,
    fs: { allow: [repoRoot] },
  },
})
