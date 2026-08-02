/* global URL, clearTimeout, console, document, fetch, process, setTimeout */
import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright-core'
import { processTreeTermination, resolveBrowserExecutable } from './browser-runtime.mjs'

const evidence = new URL('./evidence/', import.meta.url)
await mkdir(evidence, { recursive: true })
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const server = spawn(npm, ['exec', 'vite', '--', '--host', '127.0.0.1', '--port', '4174'], { detached: process.platform !== 'win32', stdio: 'ignore' })
try {
  await waitFor('http://127.0.0.1:4174/tests/primitives.html')
  const browser = await chromium.launch({ executablePath: resolveBrowserExecutable(), headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: 'light', locale: 'en-US' })
    await page.goto('http://127.0.0.1:4174/tests/primitives.html')
    await page.getByRole('heading', { name: 'Shared primitive variants' }).waitFor()
    await page.screenshot({ path: new URL('desktop-primitives.png', evidence).pathname, fullPage: true })
    await page.setViewportSize({ width: 360, height: 740 })
    await page.screenshot({ path: new URL('narrow-primitives.png', evidence).pathname, fullPage: true })
    if (await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) throw new Error('Primitive variants overflow narrow viewport')
    if (await page.locator('img, script[src="x"]').count()) throw new Error('Untrusted article text became executable markup')
    console.log('Primitive browser proof passed: deterministic desktop/narrow screenshots, Unicode wrapping, and malicious-text containment.')
  } finally { await browser.close() }
} finally { await stop(server) }

async function waitFor(url) { for (let i = 0; i < 50; i += 1) { try { if ((await fetch(url)).ok) return } catch { /* retry */ } await new Promise(resolve => setTimeout(resolve, 100)) } throw new Error('Vite did not start') }
async function stop(child) { if (child.exitCode !== null) return; const exited = new Promise(resolve => child.once('exit', resolve)); const termination = processTreeTermination(process.platform, child.pid); if ('command' in termination) { const killer = spawn(termination.command, termination.args, { stdio: 'ignore' }); await new Promise(resolve => killer.once('exit', resolve)) } else process.kill(termination.pid, termination.signal); let timer; const done = await Promise.race([exited.then(() => true), new Promise(resolve => { timer = setTimeout(() => resolve(false), 5000) })]); clearTimeout(timer); if (!done && process.platform !== 'win32') { process.kill(-child.pid, 'SIGKILL'); await exited } }
