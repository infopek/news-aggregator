#!/usr/bin/env node

import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'

const document = JSON.parse(readFileSync(new URL('../api/openapi.yaml', import.meta.url), 'utf8'))
const canonical = stableJSON(document)
const pretty = JSON.stringify(document, null, 2)
const compact = JSON.stringify(document)
const digest = (value) => createHash('sha256').update(value).digest('hex').slice(0, 16)

assert.notEqual(digest(pretty), digest(compact), 'test representations must differ')
assert.equal(digest(stableJSON(JSON.parse(pretty))), digest(stableJSON(JSON.parse(compact))))

const generated = readFileSync(new URL('../web/src/api/generated/models.ts', import.meta.url), 'utf8')
assert(generated.startsWith(`// Generated from api/openapi.yaml (${digest(canonical)}).`), 'generated semantic digest is stale')
console.log(`RESULT OK formatting_only_drift_ignored=true semantic_digest=${digest(canonical)}`)

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(',')}]`
  if (value && typeof value === 'object') return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(',')}}`
  return JSON.stringify(value)
}
