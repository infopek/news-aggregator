import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import { VitePWA } from 'vite-plugin-pwa'

import { pwaOptions } from './pwa.config.ts'

export default defineConfig({
  plugins: [
    vue(),
    VitePWA(pwaOptions)
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    outDir: '../internal/webassets/dist',
    emptyOutDir: false
  }
})
