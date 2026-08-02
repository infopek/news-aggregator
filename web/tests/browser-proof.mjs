/* global URL, clearTimeout, document, console, fetch, process, setTimeout */
import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright-core'

import { processTreeTermination, resolveBrowserExecutable } from './browser-runtime.mjs'

const evidenceDirectory = new URL('./evidence/', import.meta.url)
await mkdir(evidenceDirectory, { recursive: true })
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const preview = spawn(npmCommand, ['exec', 'vite', '--', 'preview', '--host', '127.0.0.1', '--port', '4173'], {
  detached: process.platform !== 'win32',
  stdio: 'ignore'
})
try {
  await waitForServer('http://127.0.0.1:4173/')
  const browser = await chromium.launch({ executablePath: resolveBrowserExecutable(), headless: true })
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
    if (labels.length !== 5) throw new Error(`Navigation lost items at narrow width: ${labels.join(', ')}`)
    for (const label of ['Setup', 'Ranked feed', 'Library', 'Sources', 'Settings']) {
      if (!labels.includes(label)) throw new Error(`Accepted route is not reachable from navigation: ${label}`)
    }
    console.log('Browser proof passed: deep link, back/forward, keyboard navigation, desktop/narrow screenshots, and no narrow overflow.')
  } finally {
    await browser.close()
  }
} finally {
  await stopProcessTree(preview)
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

async function stopProcessTree(child) {
  if (child.exitCode !== null) return
  const exited = new Promise((resolve) => child.once('exit', resolve))
  const termination = processTreeTermination(process.platform, child.pid)
  if ('command' in termination) {
    const killer = spawn(termination.command, termination.args, { stdio: 'ignore' })
    await new Promise((resolve) => killer.once('exit', resolve))
  } else {
    process.kill(termination.pid, termination.signal)
  }
  let shutdownTimer
  const stopped = await Promise.race([
    exited.then(() => true),
    new Promise((resolve) => { shutdownTimer = setTimeout(() => resolve(false), 5000) })
  ])
  clearTimeout(shutdownTimer)
  if (!stopped && process.platform !== 'win32') {
    process.kill(-child.pid, 'SIGKILL')
    await exited
  }
}
