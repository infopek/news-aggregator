import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { pwaOptions } from './pwa.config'

describe('PWA contract', () => {
  it('has stable localhost-compatible install identity and icon metadata', () => {
    expect(pwaOptions.registerType).toBe('prompt')
    expect(pwaOptions.manifest).toMatchObject({
      id: '/news-aggregator',
      name: 'News Aggregator',
      short_name: 'News',
      display: 'standalone',
      start_url: '/',
      scope: '/'
    })
    expect(pwaOptions.manifest.icons).toEqual(expect.arrayContaining([
      expect.objectContaining({ src: '/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any maskable' }),
      expect.objectContaining({ src: '/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any maskable' }),
      expect.objectContaining({ src: '/icon.svg', sizes: 'any', purpose: expect.stringContaining('maskable') })
    ]))
  })

  it('precaches only build assets and excludes every API route from navigation fallback', () => {
    expect(pwaOptions.workbox.runtimeCaching).toEqual([])
    expect(pwaOptions.workbox.navigateFallbackDenylist).toHaveLength(1)
    const denylist = pwaOptions.workbox.navigateFallbackDenylist ?? []
    for (const route of [
      '/api/v1/health',
      '/api/v1/feed',
      '/api/v1/sources/source-1/credential',
      '/api/v1/articles/article-1/library-state'
    ]) {
      expect(denylist.some((rule) => rule.test(route))).toBe(true)
    }
  })

  it('writes production output to the Go embed directory', async () => {
    const viteConfig = await readFile(resolve(import.meta.dirname, 'vite.config.ts'), 'utf8')
    const embedSource = await readFile(resolve(import.meta.dirname, '../internal/webassets/assets.go'), 'utf8')

    expect(viteConfig).toContain("outDir: '../internal/webassets/dist'")
    expect(embedSource).toContain('//go:embed all:dist')
  })
})
