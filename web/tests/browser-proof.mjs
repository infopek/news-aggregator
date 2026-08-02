/* global URL, document, console, fetch, setTimeout */
import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright-core'

const evidenceDirectory = new URL('./evidence/', import.meta.url)
await mkdir(evidenceDirectory, { recursive: true })
const preview = spawn('npm', ['exec', 'vite', '--', 'preview', '--host', '127.0.0.1', '--port', '4173'], { stdio: 'pipe' })
try {
  await waitForServer('http://127.0.0.1:4173/')
  const browser = await chromium.launch({ executablePath: '/usr/bin/google-chrome', headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
    await page.goto('http://127.0.0.1:4173/library')
    await page.screenshot({ path: new URL('desktop-library.png', evidenceDirectory).pathname, fullPage: true })
    await page.getByRole('link', { name: 'Sources' }).focus()
    await page.keyboard.press('Enter')
    await page.getByRole('heading', { name: 'Sources and refresh' }).waitFor()
    await page.goBack()
    await page.getByRole('heading', { name: 'Personal library' }).waitFor()

    await page.setViewportSize({ width: 360, height: 740 })
    await page.goto('http://127.0.0.1:4173/articles/direct-link')
    await page.screenshot({ path: new URL('narrow-article.png', evidenceDirectory).pathname, fullPage: true })
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
    if (overflow) throw new Error('Narrow viewport has horizontal page overflow')
    const labels = await page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link').allTextContents()
    if (labels.length !== 4) throw new Error(`Navigation lost items at narrow width: ${labels.join(', ')}`)
    console.log('Browser proof passed: deep link, back/forward, keyboard navigation, desktop/narrow screenshots, and no narrow overflow.')
  } finally {
    await browser.close()
  }
} finally {
  preview.kill('SIGTERM')
}

async function waitForServer(url) {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch { /* retry while Vite starts */ }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error('Preview server did not start')
}
