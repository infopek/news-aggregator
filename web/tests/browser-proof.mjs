/* global URL, clearTimeout, document, console, fetch, localStorage, navigator, process, sessionStorage, setTimeout */
import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { createServer } from 'node:net'
import { chromium } from 'playwright-core'

import { processTreeTermination, resolveBrowserExecutable } from './browser-runtime.mjs'

const evidenceDirectory = new URL('./evidence/', import.meta.url)
await mkdir(evidenceDirectory, { recursive: true })
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const port = await availablePort()
const baseUrl = `http://127.0.0.1:${port}`
const preview = spawn(npmCommand, ['exec', 'vite', '--', 'preview', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
  detached: process.platform !== 'win32',
  stdio: 'ignore'
})
try {
  await waitForServer(`${baseUrl}/`)
  const browser = await chromium.launch({ executablePath: resolveBrowserExecutable(), headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
    const privacyRequests = []
    let profile = emptyProfile()
    let ranking = rankingConfiguration()
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'geolocation', { configurable: true, get() { throw new Error('Browser geolocation must not be accessed') } })
      for (const storage of [localStorage, sessionStorage]) {
        storage.setItem = () => { throw new Error('Profile state must not be stored in browser storage') }
      }
    })
    await page.route('**/api/v1/**', async (route) => {
      const request = route.request(); const url = new URL(request.url())
      privacyRequests.push(url.origin + url.pathname)
      if (url.origin !== baseUrl) throw new Error(`Profile request escaped same origin: ${request.url()}`)
      const json = (value) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(value) })
      if (url.pathname === '/api/v1/profile' && request.method() === 'GET') return json(profile)
      if (url.pathname === '/api/v1/profile' && request.method() === 'PUT') { profile = { ...request.postDataJSON(), id: 'local-profile', updatedAt: '2026-08-03T12:00:00Z' }; return json(profile) }
      if (url.pathname === '/api/v1/ranking-config' && request.method() === 'GET') return json(ranking)
      if (url.pathname === '/api/v1/ranking-config' && request.method() === 'PUT') { ranking = { ...request.postDataJSON(), perDemographicCap: .1, totalDemographicCap: .2, normalizationVersion: 'v1' }; return json(ranking) }
      if (url.pathname === '/api/v1/starter-sources') return json({ items: [starterSource()] })
      return route.fulfill({ status: 404 })
    })

    await page.goto(`${baseUrl}/`)
    await page.getByRole('heading', { name: 'First-run setup' }).waitFor()
    await page.getByLabel('Interests', { exact: true }).fill('technology, science')
    await page.getByLabel('Starter feed (FEED, links to publisher)').check()
    await page.getByLabel('Use location for ranking').check()
    await page.getByLabel('Country code').fill('HU'); await page.getByLabel('Region').fill('Pest')
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').check()
    await page.getByLabel('Age value').fill('35')
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').uncheck()
    await page.getByRole('button', { name: 'Save setup' }).click()
    await page.getByText('Setup saved on this computer.').waitFor()
    await page.screenshot({ path: new URL('desktop-setup-saved.png', evidenceDirectory).pathname, fullPage: true })
    await page.reload()
    await page.waitForURL(`${baseUrl}/settings`)
    await page.getByRole('heading', { name: 'Profile and ranking' }).waitFor()
    if (await page.getByLabel('Interests', { exact: true }).inputValue() !== 'technology, science') throw new Error('Profile did not reload from authoritative API state')
    if (!await page.getByLabel('Starter feed (FEED, links to publisher)').isChecked()) throw new Error('Preferred starter source did not reload from authoritative API state')
    if (await page.getByLabel('Country code').inputValue() !== 'HU' || await page.getByLabel('Region').inputValue() !== 'Pest') throw new Error('Manual location did not reload from authoritative API state')
    if (await page.getByLabel('Age value').inputValue() !== '35' || await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').isChecked()) throw new Error('Disabled demographic value did not reload from authoritative API state')

    await page.getByLabel('Interests', { exact: true }).fill('')
    await page.getByLabel('Starter feed (FEED, links to publisher)').uncheck()
    await page.getByLabel('Country code').fill(''); await page.getByLabel('Region').fill('')
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').check(); await page.getByLabel('Age value').fill('')
    await page.getByRole('button', { name: 'Save changes' }).click()
    await page.getByText('Profile and ranking settings saved.').waitFor()
    if (profile.interests.length || profile.preferredSourceIds.length || profile.location.present || profile.age.present) throw new Error('Returning-user clear did not reach authoritative API state')
    if (privacyRequests.some((url) => !url.startsWith(`${baseUrl}/api/v1/`))) throw new Error('Profile traffic escaped the local same-origin API')

    await page.goto(`${baseUrl}/library`)
    await page.screenshot({ path: new URL('desktop-library.png', evidenceDirectory).pathname, fullPage: true })
    await page.getByRole('link', { name: 'Sources' }).focus()
    await page.keyboard.press('Enter')
    await page.getByRole('heading', { name: 'Sources and refresh' }).waitFor()
    await page.goBack()
    await page.getByRole('heading', { name: 'Personal library' }).waitFor()

    await page.setViewportSize({ width: 360, height: 740 })
    await page.goto(`${baseUrl}/articles/direct-link`)
    await page.screenshot({ path: new URL('narrow-article.png', evidenceDirectory).pathname, fullPage: true })
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
    if (overflow) throw new Error('Narrow viewport has horizontal page overflow')
    const labels = await page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link').allTextContents()
    if (labels.length !== 5) throw new Error(`Navigation lost items at narrow width: ${labels.join(', ')}`)
    for (const label of ['Setup', 'Ranked feed', 'Library', 'Sources', 'Settings']) {
      if (!labels.includes(label)) throw new Error(`Accepted route is not reachable from navigation: ${label}`)
    }
    console.log('Browser proof passed: authoritative first-run/save/reload/clear, same-origin privacy, deep links, keyboard navigation, screenshots, and no narrow overflow.')
  } finally {
    await browser.close()
  }
} finally {
  await stopProcessTree(preview)
}

function emptyProfile() {
  return { id: 'local-profile', interests: [], preferredSourceIds: [], location: { present: false, enabled: false }, age: { present: false, enabled: false }, gender: { present: false, enabled: false }, updatedAt: '2026-08-03T00:00:00Z' }
}

function rankingConfiguration() {
  return { recency: { enabled: true, weight: .25 }, interest: { enabled: true, weight: .25 }, sourcePreference: { enabled: true, weight: .1 }, behavior: { enabled: true, weight: .1 }, location: { enabled: false, weight: .05 }, age: { enabled: false, weight: .05 }, gender: { enabled: false, weight: .05 }, textSimilarity: { enabled: true, weight: .15 }, perDemographicCap: .1, totalDemographicCap: .2, normalizationVersion: 'v1' }
}

function starterSource() {
  return { id: 'starter-1', name: 'Starter feed', url: 'https://example.com/feed', kind: 'feed', enabled: true, contentPermission: 'metadata_only', adapterConfig: { format: 'rss' }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }
}

const primitiveProof = spawn(process.execPath, ['tests/primitive-browser-proof.mjs'], { stdio: 'inherit' })
const primitiveExit = await new Promise((resolve) => primitiveProof.once('exit', resolve))
if (primitiveExit !== 0) throw new Error(`Primitive browser proof failed with exit code ${primitiveExit}`)

async function availablePort() {
  const server = createServer()
  await new Promise((resolve, reject) => server.once('error', reject).listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('Could not reserve browser proof port')
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  return address.port
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
