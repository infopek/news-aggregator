import { readFile } from 'node:fs/promises'

const path = process.argv[2]
if (!path) throw new Error('Usage: node scripts/assert-go-tests-ran.mjs <go-test-json>')
const lines = (await readFile(path, 'utf8')).split(/\r?\n/).filter(Boolean)
let tests = 0
for (const line of lines) {
  let event
  try { event = JSON.parse(line) } catch { continue }
  if (event.Action === 'pass' && typeof event.Test === 'string' && !event.Test.includes('/')) tests += 1
}
if (tests === 0) throw new Error('Required Go integration suite executed zero top-level tests')
console.log(`RESULT OK go_integration_tests_executed=${tests}`)
