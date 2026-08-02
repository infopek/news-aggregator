#!/usr/bin/env node

import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const dist = resolve(root, 'internal/webassets/dist')
const manifest = JSON.parse(await readFile(resolve(dist, 'manifest.webmanifest'), 'utf8'))

assertEqual(manifest.id, '/news-aggregator', 'manifest id')
assertEqual(manifest.start_url, '/', 'manifest start_url')
assertEqual(manifest.scope, '/', 'manifest scope')
assertEqual(manifest.display, 'standalone', 'manifest display')

for (const expected of [
  { src: '/icon-192.png', sizes: '192x192', width: 192, height: 192 },
  { src: '/icon-512.png', sizes: '512x512', width: 512, height: 512 }
]) {
  const icon = manifest.icons?.find((candidate) => candidate.src === expected.src)
  if (!icon) throw new Error(`manifest is missing ${expected.src}`)
  assertEqual(icon.sizes, expected.sizes, `${expected.src} sizes`)
  assertEqual(icon.type, 'image/png', `${expected.src} type`)
  if (!String(icon.purpose).split(' ').includes('maskable')) throw new Error(`${expected.src} is not maskable`)

  const bytes = await readFile(resolve(dist, expected.src.slice(1)))
  assertPng(bytes, expected.width, expected.height, expected.src)
}

const serviceWorker = await readFile(resolve(dist, 'sw.js'), 'utf8')
for (const forbidden of ['/api/v1/', 'credential', 'library-state']) {
  if (serviceWorker.includes(forbidden)) throw new Error(`service worker contains forbidden API cache target: ${forbidden}`)
}

console.log('RESULT OK pwa_manifest_installable=true raster_icons=192x192,512x512')
console.log('RESULT OK pwa_output_icons_valid=true png_signature=true dimensions=true')
console.log('RESULT OK pwa_api_cache_excluded=true')

function assertPng(bytes, width, height, label) {
  const signature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])
  if (!bytes.subarray(0, 8).equals(signature)) throw new Error(`${label} has an invalid PNG signature`)
  assertEqual(bytes.toString('ascii', 12, 16), 'IHDR', `${label} IHDR`)
  assertEqual(bytes.readUInt32BE(16), width, `${label} width`)
  assertEqual(bytes.readUInt32BE(20), height, `${label} height`)
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) throw new Error(`${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`)
}
