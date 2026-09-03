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
    let refreshPolls = 0
    let feedRequests = 0
    let browserReadAt = null
    let browserSavedAt = null
    let browserHiddenAt = null
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'geolocation', { configurable: true, get() { throw new Error('Browser geolocation must not be accessed') } })
      for (const storage of [localStorage, sessionStorage]) {
        const setItem = storage.setItem.bind(storage)
        storage.setItem = (key, value) => {
          if (storage === localStorage && ['news-aggregator:last-refresh-id', 'news-aggregator:library-view'].includes(key)) return setItem(key, value)
          throw new Error('Private application state must not be stored in browser storage')
        }
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
      if (url.pathname === '/api/v1/sources' && request.method() === 'GET') return json({ items: [starterSource()] })
      if (url.pathname === '/api/v1/feed') { const article=browserArticle(browserReadAt,browserSavedAt,browserHiddenAt),includeHidden=url.searchParams.get('includeHidden')==='true',saved=url.searchParams.get('saved')==='true',read=url.searchParams.get('read');const visible=(includeHidden||!browserHiddenAt)&&(!saved||browserSavedAt)&&(read===null||String(Boolean(browserReadAt))===read);feedRequests += 1; return json({ items: visible?[article]:[], nextCursor: null }) }
      if (url.pathname === '/api/v1/articles/browser-article') return json({ article: browserArticle(browserReadAt,browserSavedAt,browserHiddenAt), fullContent: null })
      if (url.pathname === '/api/v1/articles/browser-article/library-state' && request.method() === 'PATCH') { const patch=request.postDataJSON();if(patch.read!==undefined)browserReadAt=patch.read?'2026-08-14T09:00:00Z':null;if(patch.saved!==undefined)browserSavedAt=patch.saved?'2026-08-14T09:01:00Z':null;if(patch.hidden!==undefined)browserHiddenAt=patch.hidden?'2026-08-14T09:02:00Z':null;return json({ articleId: 'browser-article', readAt: browserReadAt, savedAt: browserSavedAt, hiddenAt: browserHiddenAt }) }
      if (url.pathname === '/api/v1/refresh' && request.method() === 'POST') return json(refreshRun('running'))
      if (url.pathname === '/api/v1/refresh/browser-refresh') return json(refreshRun(refreshPolls++ === 0 ? 'running' : 'partial_success'))
      return route.fulfill({ status: 404 })
    })

    await page.goto(`${baseUrl}/`)
    await page.getByRole('heading', { name: 'Make your feed yours' }).waitFor()
    await page.screenshot({ path: new URL('desktop-onboarding-interests.png', evidenceDirectory).pathname, fullPage: true })
    await page.setViewportSize({ width: 390, height: 844 }); await page.screenshot({ path: new URL('narrow-onboarding-interests.png', evidenceDirectory).pathname, fullPage: true })
    if (await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) throw new Error('Onboarding overflows at 390px')
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.getByLabel('Add an interest').fill('technology'); await page.getByLabel('Add an interest').press('Enter')
    await page.getByLabel('Add an interest').fill('science'); await page.getByLabel('Add an interest').press('Enter')
    await page.getByRole('button', { name: 'Continue' }).click()
    await page.getByLabel('Use my location when ranking stories').check()
    await page.getByLabel('Country').selectOption('HU'); await page.getByLabel('City or area').fill('Budapest')
    await page.getByText('Additional personalization').click()
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').check()
    await page.getByLabel('Age value').fill('35')
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').uncheck()
    await page.getByRole('button', { name: 'Continue' }).click()
    await page.getByText('Starter feed', { exact: true }).click()
    await page.getByRole('button', { name: 'Continue' }).click()
    await page.getByRole('button', { name: 'Finish setup' }).click()
    await page.waitForURL(`${baseUrl}/`)
    await page.goto(`${baseUrl}/settings`)
    await page.reload()
    await page.getByRole('heading', { name: 'Settings' }).waitFor()
    await page.screenshot({ path: new URL('desktop-settings.png', evidenceDirectory).pathname, fullPage: true })
    if (!await page.getByRole('button', { name: 'Remove technology' }).count() || !await page.getByRole('button', { name: 'Remove science' }).count()) throw new Error('Profile did not reload from authoritative API state')
    if (!profile.preferredSourceIds.includes('starter-1')) throw new Error('Preferred starter source did not reload from authoritative API state')
    if (await page.getByLabel('Country').inputValue() !== 'HU' || await page.getByLabel('City or area').inputValue() !== 'Budapest') throw new Error('Manual location did not reload from authoritative API state')
    await page.getByText('Additional personalization').click()
    if (await page.getByLabel('Age value').inputValue() !== '35' || await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').isChecked()) throw new Error('Disabled demographic value did not reload from authoritative API state')

    await page.goto(`${baseUrl}/`)
    await page.getByRole('heading', { name: 'Ranked feed' }).waitFor()
    await page.locator('.refresh-control > button').click()
    await page.getByText('Refresh complete with some source issues.').waitFor()
    await page.getByRole('heading', { name: 'Browser-ranked story' }).waitFor()
    if (feedRequests < 2) throw new Error('Terminal refresh did not reload authoritative ranked feed')
    await page.getByRole('button', { name: 'Mark read' }).click()
    await page.getByRole('button', { name: 'Mark unread' }).waitFor()
    if (feedRequests < 3) throw new Error('Inline action did not reload authoritative ranked feed')
    if (await page.getByText('0.45', { exact: true }).isVisible()) throw new Error('Raw ranking contribution is visible in the ordinary article card')
    await page.getByText('Why this story?').click()
    await page.getByText('Matches one of your interests').waitFor()
    await page.screenshot({ path: new URL('desktop-ranked-feed.png', evidenceDirectory).pathname, fullPage: true })
    await page.getByRole('link', { name: 'Open article' }).click()
    await page.getByRole('heading', { name: 'Browser-ranked story' }).waitFor()
    await page.getByText(/full text is not stored/).waitFor()
    const publisherLink = page.getByRole('link', { name: 'Read the full article' })
    if (await publisherLink.getAttribute('href') !== 'https://example.com/story') throw new Error('Reader did not expose the canonical publisher destination')
    await page.getByRole('button',{name:'Save'}).click();await page.getByRole('button',{name:'Unsave'}).waitFor()
    await page.goto(`${baseUrl}/settings`)

    await page.getByRole('button', { name: 'Remove technology' }).click(); await page.getByRole('button', { name: 'Remove science' }).click()
    await page.getByLabel('Country').selectOption(''); await page.getByLabel('City or area').fill(''); await page.getByText('Administrative region — optional advanced detail').click(); await page.getByLabel('Region').fill('')
    await page.getByText('Additional personalization').click()
    await page.getByRole('group', { name: 'Age' }).getByRole('checkbox').check(); await page.getByLabel('Age value').fill('')
    await page.getByRole('button', { name: 'Save changes' }).click()
    await page.getByText('Profile and ranking settings saved.').waitFor()
    if (profile.interests.length || !profile.preferredSourceIds.includes('starter-1') || profile.location.present || profile.age.present) throw new Error('Returning-user profile changes did not reach authoritative API state')
    if (privacyRequests.some((url) => !url.startsWith(`${baseUrl}/api/v1/`))) throw new Error('Profile traffic escaped the local same-origin API')

    await page.goto(`${baseUrl}/library`)
    await page.getByRole('heading',{name:'Browser-ranked story'}).waitFor()
    await page.reload();await page.getByRole('heading',{name:'Browser-ranked story'}).waitFor()
    await page.screenshot({ path: new URL('desktop-library.png', evidenceDirectory).pathname, fullPage: true })
    await page.getByRole('button',{name:'Hide'}).click();await page.getByText('No saved stories').waitFor()
    await page.getByRole('button',{name:'Hidden'}).click();await page.getByRole('button',{name:'Restore'}).focus();await page.keyboard.press('Enter');await page.getByText('No hidden stories').waitFor()
    const storedKeys=await page.evaluate(()=>Array.from({length:localStorage.length},(_,index)=>localStorage.key(index)).filter(Boolean));if(storedKeys.some(key=>!['news-aggregator:last-refresh-id','news-aggregator:library-view'].includes(key)))throw new Error(`Private authoritative state leaked to browser storage: ${storedKeys.join(', ')}`)
    await page.getByRole('link', { name: 'Sources' }).focus()
    await page.keyboard.press('Enter')
    await page.getByRole('heading', { name: 'Sources and refresh' }).waitFor()
    await page.locator('.refresh-control > button').click()
    await page.getByRole('link', { name: 'Library' }).click()
    await page.getByRole('heading', { name: 'Library', exact: true }).waitFor()
    await page.getByRole('link', { name: 'Sources' }).click()
    await page.getByText('Refresh complete with some source issues.').waitFor()
    await page.getByText('Refresh complete with source issues').click()
    await page.getByText('Rate limited — Retry later.').waitFor()
    await page.reload()
    await page.getByText('Refresh complete with some source issues.').waitFor()
    await page.screenshot({ path: new URL('desktop-sources-mixed-refresh.png', evidenceDirectory).pathname, fullPage: true })
    await page.goBack()
    await page.getByRole('heading', { name: 'Library', exact: true }).waitFor()

    await page.setViewportSize({ width: 390, height: 844 })
    await page.screenshot({ path: new URL('narrow-library.png', evidenceDirectory).pathname, fullPage: true })
    await page.getByRole('link', { name: 'Sources' }).click()
    await page.getByRole('heading', { name: 'Sources and refresh' }).waitFor()
    await page.screenshot({ path: new URL('narrow-sources.png', evidenceDirectory).pathname, fullPage: true })
    if (await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)) throw new Error('Narrow Sources viewport has horizontal page overflow')
    await page.goto(`${baseUrl}/articles/direct-link`)
    await page.screenshot({ path: new URL('narrow-article.png', evidenceDirectory).pathname, fullPage: true })
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
    if (overflow) throw new Error('Narrow viewport has horizontal page overflow')
    const labels = await page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link').allTextContents()
    if (labels.length !== 4) throw new Error(`Navigation lost items at narrow width: ${labels.join(', ')}`)
    for (const label of ['Ranked feed', 'Library', 'Sources', 'Settings']) {
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

function refreshRun(status) {
  return { id: 'browser-refresh', status, startedAt: '2026-08-13T10:00:00Z', finishedAt: status === 'running' ? null : '2026-08-13T10:00:01Z', outcomes: status === 'running' ? [] : [{ sourceId: 'starter-1', fetched: 3, inserted: 2, updated: 0, skipped: 0, failed: 1, errorCode: 'rate_limited', errorSummary: 'Retry later.' }] }
}

function browserArticle(browserReadAt = null,browserSavedAt=null,browserHiddenAt=null) {
  return { id: 'browser-article', sourceId: 'starter-1', canonicalUrl: 'https://example.com/story', title: 'Browser-ranked story', author: 'Fixture Reporter', publishedAt: '2026-08-14T08:00:00Z', fetchedAt: '2026-08-14T08:01:00Z', excerpt: 'A permission-aware browser fixture.', contentPermission: 'metadata_only', language: 'en', topics: ['technology'], library: { articleId: 'browser-article', readAt: browserReadAt, savedAt: browserSavedAt, hiddenAt: browserHiddenAt }, ranking: { score: .9, algorithmVersion: 'v1', calculatedAt: '2026-08-14T08:02:00Z', contributions: [{ signal: 'interest', rawScore: .9, weight: .5, weightedScore: .45, reasonCode: 'explicit_interest_match', reasonValues: { matched_interests: 'technology' } }] } }
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
