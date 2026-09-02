/* global process */
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import playwright from '../../web/node_modules/playwright-core/index.js'
import { processTreeTermination, resolveBrowserExecutable } from '../../web/tests/browser-runtime.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const { chromium } = playwright
const root = resolve(here, '../..')
const evidence = join(here, 'evidence')
const temporary = await mkdtemp(join(tmpdir(), 'news-aggregator-e2e-'))
const data = join(temporary, 'user-data')
const binary = join(temporary, process.platform === 'win32' ? 'news-aggregator-e2e.exe' : 'news-aggregator-e2e')
const transcript = []
let app
let browser
let context
let page
let traceStopped = false

try {
  await mkdir(data, { recursive: true }); await mkdir(evidence, { recursive: true })
  await command('npm', ['--prefix', 'web', 'run', 'build'])
  await command('go', ['build', '-o', binary, './tests/e2e/cmd'])
  const port = await freePort(); const origin = `http://127.0.0.1:${port}`
  app = await launchApp(port, origin, 'initial launch')
  browser = await chromium.launch({ executablePath: resolveBrowserExecutable(), headless: true })
  context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true })
  page = await context.newPage()
  const browserRequests = []
  page.on('request', request => browserRequests.push(request.url()))

  await page.goto(origin); await page.getByRole('heading', { name: 'Make your feed yours' }).waitFor()
  await navigate(page, 'Sources'); await page.getByRole('button', { name: 'Add starter' }).click(); await page.getByText('Unavailable fixture saved.').waitFor()
  await page.goto(`${origin}/setup`); await page.getByRole('heading', { name: 'Make your feed yours' }).waitFor()
  for (const topic of ['technology', 'climate', 'science']) { await page.getByLabel('Add an interest').fill(topic); await page.getByLabel('Add an interest').press('Enter') }
  await page.getByRole('button', { name: 'Continue' }).click()
  await page.getByLabel('Use my location when ranking stories').check()
  await page.getByLabel('Country').selectOption('HU'); await page.getByLabel('City or area').fill('Budapest')
  await page.getByText('Additional personalization').click()
  await page.getByRole('group', { name: 'Age' }).getByLabel('Use this signal').check(); await page.getByLabel('Age value').fill('37')
  await page.getByRole('group', { name: 'Gender' }).getByLabel('Use this signal').check(); await page.getByLabel('Gender value').fill('nonbinary')
  await page.getByRole('button', { name: 'Continue' }).click()
  await page.getByText('Unavailable fixture', { exact: true }).click()
  await page.getByRole('button', { name: 'Continue' }).click()
  await screenshot(page, '01-first-run.png')
  await page.getByRole('button', { name: 'Finish setup' }).click()
  await page.waitForURL(`${origin}/`)

  await navigate(page, 'Ranked feed')
  await page.getByRole('heading', { name: 'No stories found' }).waitFor()
  await screenshot(page, '01-empty-feed.png')

  await navigate(page, 'Sources')
  await addFeed(page, 'Metadata News', 'http://metadata.fixture.test/feed.xml', 'Metadata only')
  await addFeed(page, 'Full Content News', 'http://full.fixture.test/feed.xml', 'Full content allowed')
  await page.getByRole('button', { name: /refresh/i }).last().click()
  await page.getByText('Refresh complete with some source issues.').waitFor({ timeout: 30000 })
  await page.getByText('Refresh complete with source issues').click()
  await page.getByText('Metadata News').last().waitFor(); await page.getByText('Unavailable fixture').last().waitFor()
  await screenshot(page, '02-partial-refresh.png')

  await navigate(page, 'Ranked feed')
  await page.getByRole('link', { name: 'Technology advances in local science', exact: true }).waitFor()
  await page.getByText('Why this story?').first().click()
  await page.getByText('Similar to topics you follow', { exact: true }).first().waitFor(); await page.getByText('Matches an optional publisher-declared age range', { exact: true }).first().waitFor()
  assert(!await page.getByText(/0\.\d{8,}/).first().isVisible(), 'raw ranking contribution is visible by default')
  const ranked = await page.evaluate(async () => (await (await fetch('/api/v1/feed?limit=20')).json()).items)
  for (const signal of ['location', 'age', 'gender']) assert(ranked.some(item => item.ranking.contributions.some(value => value.signal === signal && value.weightedScore > 0)), `enabled ${signal} contribution missing from production feed`)
  await screenshot(page, '03-ranked-feed.png')
  await auditA11y(page, 'ranked-feed')

  const fullCard = page.locator('.ranked-list > li').filter({ hasText: 'Technology advances in local science' })
  await fullCard.getByRole('link', { name: 'Open article' }).click()
  await page.getByText('Permitted full article body about technology').waitFor()
  await page.getByRole('button', { name: 'Mark read' }).click(); await page.getByText('Article state updated.').waitFor()
  await screenshot(page, '04-full-reader.png')
  await page.getByRole('link', { name: 'Back to ranked feed' }).click()

  const metadataCard = page.locator('.ranked-list > li').filter({ hasText: 'Climate science briefing' })
  await metadataCard.getByRole('button', { name: 'Save' }).click(); await metadataCard.getByText('Article state updated.').waitFor()
  await metadataCard.getByRole('link', { name: 'Open article' }).click()
  await page.getByText(/full text is not stored/).waitFor()
  const publisher = page.getByRole('link', { name: 'Read the full article' })
  assert((await publisher.getAttribute('href')) === 'http://publisher.fixture.test/metadata-climate', 'canonical publisher link was not retained')
  await screenshot(page, '05-metadata-reader.png')

  await navigate(page, 'Ranked feed')
  await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor()
  await page.locator('#saved-filter').selectOption('saved'); await page.getByRole('button', { name: 'Apply filters' }).click()
  await page.getByRole('link', { name: 'Technology advances in local science', exact: true }).waitFor({ state: 'detached' })
  await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor()
  assert(await page.getByRole('link', { name: 'Technology advances in local science', exact: true }).count() === 0, 'saved filter included an unsaved article')
  await page.getByRole('button', { name: 'Clear filters' }).click()
  await page.getByRole('link', { name: 'Technology advances in local science', exact: true }).waitFor()
  await page.getByText('Updating ranked stories… Current results remain visible.').waitFor({ state: 'hidden' })
  const hideCard = page.locator('.ranked-list > li').filter({ hasText: 'Climate science briefing' })
  await keyboardActivate(page, hideCard.getByRole('button', { name: 'Hide' })); await hideCard.waitFor({ state: 'detached' })
  await keyboardActivate(page, page.getByRole('link', { name: 'Library', exact: true })); await page.getByRole('heading', { name: 'Library', exact: true }).waitFor()
  await keyboardActivate(page, page.getByRole('button', { name: 'Hidden' }))
  const hiddenState = await page.evaluate(async () => ({ body: await (await fetch('/api/v1/feed?includeHidden=true&limit=100')).json(), text: document.body.innerText }))
  assert(hiddenState.body.items?.some(item => item.title === 'Climate science briefing'), `hidden article missing from authoritative API: ${JSON.stringify(hiddenState)}`)
  await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor(); await keyboardActivate(page, page.getByRole('button', { name: 'Restore' }))
  await page.getByText('No hidden stories').waitFor()
  await page.getByRole('button', { name: 'Saved' }).click(); await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor()
  await screenshot(page, '06-library.png')

  await auditA11y(page, 'library')
  await page.setViewportSize({ width: 390, height: 844 })
  await screenshot(page, '07-narrow-library.png'); await auditA11y(page, 'narrow-library'); await assertNoOverflow(page, 'narrow library')
  await navigate(page, 'Sources'); await screenshot(page, '08-narrow-sources.png'); await auditA11y(page, 'narrow-sources'); await assertNoOverflow(page, 'narrow sources')
  await navigate(page, 'Ranked feed')
  await page.getByRole('heading', { name: 'Ranked feed' }).waitFor(); await screenshot(page, '09-narrow-feed.png'); await auditA11y(page, 'narrow-feed')
  await page.locator('#source-filter').selectOption({ label: 'Metadata News' }); await page.locator('#saved-filter').selectOption('saved'); await page.locator('#feed-search').fill('climate'); await page.getByText('More filters', { exact: true }).click(); await page.locator('#include-hidden').check(); await page.getByRole('button', { name: 'Apply filters' }).click(); await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor()

  const storage = await page.evaluate(() => ({ local: { ...localStorage }, session: { ...sessionStorage } }))
  for (const serialized of Object.values({ ...storage.local, ...storage.session })) assert(!/technology|climate|Budapest|metadata\.fixture|full\.fixture/i.test(String(serialized)), 'authoritative profile/source data leaked into browser storage')
  const unexpected = browserRequests.filter(value => !value.startsWith(origin))
  assert(unexpected.length === 0, `unexpected browser outbound requests: ${unexpected.join(', ')}`)

  await stopApp(app, 'stopped Go process'); app = undefined
  await navigate(page, 'Settings')
  await page.getByRole('alert').getByText('The local service is temporarily unavailable.').waitFor()
  await screenshot(page, '10-api-unavailable.png')
  app = await launchApp(port, origin, 'restarted Go process with same SQLite database')
  await page.getByRole('button', { name: 'Try again' }).click()
  await page.getByRole('button', { name: 'Remove technology' }).waitFor()
  await page.goto(`${origin}/library`); await page.getByRole('button', { name: 'Saved' }).click(); await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor()
  await navigate(page, 'Settings'); await page.getByRole('button', { name: 'Remove technology' }).waitFor(); assert(await page.getByRole('button', { name: 'Remove climate' }).count(), 'profile did not survive restart')
  await navigate(page, 'Sources'); await page.getByRole('heading', { name: 'Metadata News' }).waitFor()
  await navigate(page, 'Ranked feed'); await page.getByRole('link', { name: 'Climate science briefing', exact: true }).waitFor();assert((await page.locator('#source-filter').inputValue())!=='','source filter did not survive restart');assert((await page.locator('#saved-filter').inputValue())==='saved','saved filter did not survive restart');assert((await page.locator('#feed-search').inputValue())==='climate','search filter did not survive restart');assert(await page.locator('#include-hidden').isChecked(),'include-hidden filter did not survive restart')
  await screenshot(page, '11-restart-persistence.png')
  await context.tracing.stop({ path: join(evidence, 'daily-workflow-trace.zip') }); traceStopped = true

  const report = { decision: 'APPROVE', revision: await output('git', ['rev-parse', 'HEAD']), generatedAt: new Date().toISOString(), processRestart: transcript, accessibility: ['ranked-feed: 0 serious/critical', 'library: 0 serious/critical', 'narrow-library: 0 serious/critical', 'narrow-sources: 0 serious/critical', 'narrow-feed: 0 serious/critical'], browserStorage: storage, browserOutboundRequests: [...new Set(browserRequests)], assertions: 'complete daily-use workflow passed against real Go/SQLite/Vue stack, including enabled optional ranking contributions, persisted source/saved/search/include-hidden filters, empty feed, keyboard-only hide/restore navigation, responsive Sources and Library workflows, and API interruption recovery' }
  await writeFile(join(evidence, 'verification-report.json'), JSON.stringify(report, null, 2) + '\n')
  await writeFile(join(evidence, 'restart-transcript.txt'), transcript.join('\n') + '\n')
  console.log('VERIFY-002 APPROVED; evidence written to tests/e2e/evidence')
} catch (error) {
  const failure = error instanceof Error ? error.stack ?? error.message : String(error)
  if (page) await page.screenshot({ path: join(evidence, 'failure.png'), fullPage: true }).catch(() => {})
  if (context && !traceStopped) { await context.tracing.stop({ path: join(evidence, 'daily-workflow-trace.zip') }).catch(() => {}); traceStopped = true }
  await writeFile(join(evidence, 'verification-report.json'), JSON.stringify({ decision: 'REJECT', revision: await output('git', ['rev-parse', 'HEAD']).catch(() => 'unknown'), generatedAt: new Date().toISOString(), failure, processRestart: transcript }, null, 2) + '\n').catch(() => {})
  await writeFile(join(evidence, 'restart-transcript.txt'), transcript.join('\n') + '\n').catch(() => {})
  console.error(error); process.exitCode = 1
} finally {
  if (app) await stopApp(app, 'cleanup stop').catch(() => {})
  if (browser) await browser.close().catch(() => {})
  await rm(temporary, { recursive: true, force: true })
}

async function addFeed(page, name, url, permission) {
  await page.getByRole('button', { name: 'Add custom source' }).click()
  await page.getByLabel('Name').fill(name); await page.getByLabel('Source address').fill(url); await page.getByLabel('Include this source when refreshing').check()
  await page.getByLabel('Article access').selectOption({ label: permission }); await page.getByLabel('Feed format').selectOption('rss')
  await page.getByRole('button', { name: 'Save source' }).click(); await page.getByText(`${name} saved.`).waitFor()
}
async function navigate(page, name) {
  const headings = { Sources: 'Sources and refresh', Settings: 'Settings', Library: 'Library', 'Ranked feed': 'Ranked feed' }
  await page.getByRole('link', { name, exact: true }).click(); await page.getByRole('heading', { name: headings[name], exact: true }).waitFor()
}
async function screenshot(page, name) { await page.screenshot({ path: join(evidence, name), fullPage: true }) }
async function assertUnchecked(locator) { assert(!(await locator.isChecked()), 'optional demographic signal was unexpectedly enabled') }
function assert(value, message) { if (!value) throw new Error(message) }
async function auditA11y(page, name) {
  await page.addScriptTag({ path: join(root, 'web/node_modules/axe-core/axe.min.js') })
  const result = await page.evaluate(() => globalThis.axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa'] } }))
  const blocking = result.violations.filter(item => ['serious', 'critical'].includes(item.impact))
  await writeFile(join(evidence, `accessibility-${name}.json`), JSON.stringify(result, null, 2))
  assert(blocking.length === 0, `${name} accessibility violations: ${blocking.map(item => item.id).join(', ')}`)
}
async function assertNoOverflow(page, name) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
  assert(!overflow, `${name} has horizontal page overflow`)
}
async function keyboardActivate(page, target) {
  await target.waitFor()
  for (let index=0; index<120; index++) {
    if (await target.evaluate(element => element === document.activeElement)) {
      await page.keyboard.press('Enter')
      return
    }
    await page.keyboard.press('Tab')
  }
  throw new Error('keyboard navigation did not reach the required core-path control')
}
async function freePort() { return await new Promise((resolvePort, reject) => { const server=createServer(); server.once('error',reject); server.listen(0,'127.0.0.1',()=>{const address=server.address();server.close(error=>error?reject(error):resolvePort(address.port))}) }) }
async function launchApp(port, origin, event) {
  const child=spawn(binary,[],{cwd:root,detached:process.platform!=='win32',env:{...process.env,NEWS_AGGREGATOR_PORT:String(port),NEWS_AGGREGATOR_E2E_ROOT:root,NEWS_AGGREGATOR_E2E_DATA:data},stdio:['ignore','pipe','pipe']})
  let logs=''; child.stdout.on('data',chunk=>logs+=chunk); child.stderr.on('data',chunk=>logs+=chunk)
  await waitHealth(origin, child, ()=>logs); transcript.push(`${new Date().toISOString()} ${event} pid=${child.pid}`); return child
}
async function waitHealth(origin, child, logs) { for(let attempt=0;attempt<100;attempt++){if(child.exitCode!==null)throw new Error(`Go process exited early: ${logs()}`);try{const response=await fetch(`${origin}/api/v1/health`);if(response.ok)return}catch{}await new Promise(resolveWait=>setTimeout(resolveWait,100))}throw new Error(`Go process readiness timed out: ${logs()}`) }
async function stopApp(child,event){const termination=processTreeTermination(process.platform,child.pid);if(termination.command)await command(termination.command,termination.args);else process.kill(termination.pid,termination.signal);await new Promise(resolveWait=>child.once('exit',resolveWait));transcript.push(`${new Date().toISOString()} ${event} pid=${child.pid}`)}
async function command(commandName,args){return await new Promise((resolveCommand,reject)=>{const child=spawn(commandName,args,{cwd:root,stdio:'inherit',shell:process.platform==='win32'});child.once('error',reject);child.once('exit',code=>code===0?resolveCommand():reject(new Error(`${commandName} ${args.join(' ')} exited ${code}`)))})}
async function output(commandName,args){return await new Promise((resolveOutput,reject)=>{const child=spawn(commandName,args,{cwd:root});let value='';child.stdout.on('data',chunk=>value+=chunk);child.once('error',reject);child.once('exit',code=>code===0?resolveOutput(value.trim()):reject(new Error(`${commandName} exited ${code}`)))})}
