import type { VitePWAOptions } from 'vite-plugin-pwa'

export const pwaOptions = {
  registerType: 'prompt',
  injectRegister: false,
  includeAssets: ['icon.svg'],
  manifest: {
    id: '/news-aggregator',
    name: 'News Aggregator',
    short_name: 'News',
    description: 'A private news aggregator running on this computer.',
    theme_color: '#172554',
    background_color: '#f8fafc',
    display: 'standalone',
    start_url: '/',
    scope: '/',
    icons: [
      {
        src: '/icon.svg',
        sizes: 'any',
        type: 'image/svg+xml',
        purpose: 'any maskable'
      }
    ]
  },
  workbox: {
    cleanupOutdatedCaches: true,
    navigateFallback: '/index.html',
    navigateFallbackDenylist: [/^\/api(?:\/|$)/],
    // API responses are deliberately absent. The cache contains only the
    // generated static application shell; mutations always reach Go.
    runtimeCaching: []
  }
} satisfies Partial<VitePWAOptions>
