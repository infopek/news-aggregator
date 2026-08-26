/* global URL, clearTimeout, console, document, fetch, process, setTimeout */
import { spawn } from 'node:child_process'
import { Buffer } from 'node:buffer'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { chromium } from 'playwright-core'
import { processTreeTermination, resolveBrowserExecutable } from './browser-runtime.mjs'

const evidence = new URL('./evidence/', import.meta.url)
await mkdir(evidence, { recursive: true })
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const port = await availablePort()
const url = `http://127.0.0.1:${port}/tests/primitives.html`
const server = spawn(npm, ['exec', 'vite', '--', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], { detached: process.platform !== 'win32', stdio: 'ignore' })
try {
  await waitFor(url)
  const browser = await chromium.launch({ executablePath: resolveBrowserExecutable(), headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: 'light', locale: 'en-US' })
    await page.goto(url)
    await page.getByRole('heading', { name: 'Shared primitive variants' }).waitFor()
    await assertScreenshot(page, 'desktop-primitives.png')
    await page.setViewportSize({ width: 390, height: 844 })
    await assertScreenshot(page, 'narrow-primitives.png')
    if (await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) throw new Error('Primitive variants overflow narrow viewport')
    if (await page.locator('img, script[src="x"]').count()) throw new Error('Untrusted article text became executable markup')
    const screenshotProof = process.env.SCREENSHOT_MODE === 'smoke' ? 'desktop/narrow screenshot smoke output' : 'deterministic desktop/narrow screenshots'
    console.log(`Primitive browser proof passed: ${screenshotProof}, Unicode wrapping, and malicious-text containment.`)
  } finally { await browser.close() }
} finally { await stop(server) }

async function availablePort() { const listener = createServer(); await new Promise((resolve, reject) => listener.once('error', reject).listen(0, '127.0.0.1', resolve)); const address = listener.address(); if (!address || typeof address === 'string') throw new Error('Could not reserve primitive proof port'); await new Promise((resolve, reject) => listener.close(error => error ? reject(error) : resolve())); return address.port }
async function assertScreenshot(page, name) {
  const actual = await page.screenshot({ fullPage: true, animations: 'disabled', caret: 'hide' })
  if (process.env.SCREENSHOT_MODE === 'smoke') {
    const pngSignature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
    if (actual.length < 1024 || !actual.subarray(0, pngSignature.length).equals(pngSignature)) throw new Error(`Screenshot smoke proof failed: ${name}`)
    return
  }
  const path = new URL(name, evidence)
  if (process.env.UPDATE_SCREENSHOTS === '1') { await writeFile(path, actual); return }
  let expected
  try { expected = await readFile(path) } catch { throw new Error(`Screenshot baseline missing: ${name}. Run with UPDATE_SCREENSHOTS=1 after reviewing the output.`) }
  if (!actual.equals(expected)) throw new Error(`Screenshot regression: ${name}. Run with UPDATE_SCREENSHOTS=1 only after reviewing the change.`)
}

async function waitFor(url) { for (let i = 0; i < 50; i += 1) { try { if ((await fetch(url)).ok) return } catch { /* retry */ } await new Promise(resolve => setTimeout(resolve, 100)) } throw new Error('Vite did not start') }
async function stop(child) { if (child.exitCode !== null) return; const exited = new Promise(resolve => child.once('exit', resolve)); const termination = processTreeTermination(process.platform, child.pid); if ('command' in termination) { const killer = spawn(termination.command, termination.args, { stdio: 'ignore' }); await new Promise(resolve => killer.once('exit', resolve)) } else process.kill(termination.pid, termination.signal); let timer; const done = await Promise.race([exited.then(() => true), new Promise(resolve => { timer = setTimeout(() => resolve(false), 5000) })]); clearTimeout(timer); if (!done && process.platform !== 'win32') { process.kill(-child.pid, 'SIGKILL'); await exited } }
