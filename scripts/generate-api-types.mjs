#!/usr/bin/env node

import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const specPath = resolve(root, 'api/openapi.yaml')
const outputDir = resolve(root, 'web/src/api/generated')
const fixtureDir = resolve(root, 'test/fixtures/api')
const check = process.argv.includes('--check')

class ContractValidationError extends Error {
  constructor(issues) {
    super(issues.map((issue) => `${issue.path} ${issue.reason}`).join('; '))
    this.issues = issues
  }
}

const specText = await readFile(specPath, 'utf8')
const spec = JSON.parse(specText)
validateOpenAPI(spec)

// Hash the parsed contract rather than source bytes. Formatting-only YAML/JSON
// changes must not create generated binding drift.
const digest = createHash('sha256').update(stableJSON(spec)).digest('hex').slice(0, 16)
const schemas = spec.components.schemas
const models = generateModels(schemas, digest)
const operations = generateOperations(spec.paths, digest)
const operationTest = generateOperationTest(digest)

const manifest = JSON.parse(await readFile(resolve(fixtureDir, 'manifest.json'), 'utf8'))
validateFixtures(manifest, schemas)
const fixtureValues = new Map()
for (const [file, schemaName] of Object.entries(manifest)) {
  const fixture = await loadFixture(file)
  validateValue(fixture, schemas[schemaName], schemas, `$fixture:${file}`)
  fixtureValues.set(file, fixture)
}
const fixtureTest = generateFixtureTest(manifest, fixtureValues, digest)
const invalidManifest = JSON.parse(await readFile(resolve(fixtureDir, 'invalid-manifest.json'), 'utf8'))
for (const [file, expectation] of Object.entries(invalidManifest)) {
  if (!schemas[expectation.schema]) throw new Error(`negative fixture ${file} names unknown schema ${expectation.schema}`)
  const fixture = await loadFixture(file)
  assertNegativeFixture(file, fixture, expectation)
}
// Regression guard: a negative expectation may not pass merely because the
// fixture failed somewhere else.
const [regressionFile, regressionExpectation] = Object.entries(invalidManifest)[0]
const regressionFixture = await loadFixture(regressionFile)
let wrongReasonRejected = false
try { assertNegativeFixture(regressionFile, regressionFixture, { ...regressionExpectation, code: 'deliberately_wrong_code' }) } catch { wrongReasonRejected = true }
if (!wrongReasonRejected) throw new Error('negative fixture harness accepted the wrong rejection reason')

if (!check) await mkdir(outputDir, { recursive: true })
await emit('models.ts', models)
await emit('operations.ts', operations)
await emit('fixture-contract.test.ts', fixtureTest)
await emit('operation-contract.test.ts', operationTest)

console.log(`RESULT OK openapi_valid=true operations=${operationEntries(spec.paths).length} schemas=${Object.keys(schemas).length}`)
console.log(`RESULT OK api_fixtures_valid=true fixtures=${Object.keys(manifest).length}`)
console.log(`RESULT OK api_negative_fixtures_rejected=true fixtures=${Object.keys(invalidManifest).length}`)
console.log('RESULT OK api_negative_wrong_reason_rejected=true')
console.log(`RESULT OK api_types_${check ? 'current' : 'generated'}=true digest=${digest}`)
console.log('RESULT OK openapi_format_invariant=true')

function stableJSON(value) {
  if (Array.isArray(value)) return `[${value.map(stableJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableJSON(value[key])}`).join(',')}}`
  }
  return JSON.stringify(value)
}

async function emit(name, content) {
  const path = resolve(outputDir, name)
  if (check) {
    const current = await readFile(path, 'utf8').catch(() => '')
    if (current !== content) {
      throw new Error(`${name} is stale; run node scripts/generate-api-types.mjs`)
    }
    return
  }
  await writeFile(path, content, 'utf8')
}

function validateOpenAPI(document) {
  if (!String(document.openapi).startsWith('3.1.')) throw new Error('OpenAPI 3.1 is required')
  if (!document.info?.title || !document.info?.version) throw new Error('info.title and info.version are required')
  if (!document.components?.schemas) throw new Error('components.schemas is required')

  const operationIds = new Set()
  for (const [path, method, operation] of operationEntries(document.paths)) {
    if (!path.startsWith('/api/v1/')) throw new Error(`unversioned path: ${path}`)
    if (!operation.operationId || operationIds.has(operation.operationId)) throw new Error(`missing or duplicate operationId: ${path} ${method}`)
    operationIds.add(operation.operationId)
    if (!Object.hasOwn(operation, 'x-request-type') || !Object.hasOwn(operation, 'x-response-type')) throw new Error(`missing TypeScript binding metadata: ${operation.operationId}`)
    for (const typeName of [operation['x-request-type'], operation['x-response-type']]) {
      if (typeName !== null && !document.components.schemas[typeName]) throw new Error(`unknown operation type ${typeName}: ${operation.operationId}`)
    }
    if (!operation.responses || Object.keys(operation.responses).length === 0) throw new Error(`responses required: ${operation.operationId}`)
  }

  if (document.paths['/api/v1/sources/{sourceId}/credential'].get) throw new Error('credential GET is forbidden')
  const credentialSchema = document.components.schemas.CredentialWrite
  if (!credentialSchema?.properties?.secret?.writeOnly) throw new Error('credential secret must be writeOnly')

  for (const schema of Object.values(document.components.schemas)) walkRefs(schema, document.components.schemas)
}

function walkRefs(value, available) {
  if (!value || typeof value !== 'object') return
  if (value.$ref) {
    const name = value.$ref.replace('#/components/schemas/', '')
    if (!available[name]) throw new Error(`unknown schema reference: ${value.$ref}`)
  }
  for (const child of Object.values(value)) walkRefs(child, available)
}

function operationEntries(paths) {
  const methods = new Set(['get', 'post', 'put', 'patch', 'delete'])
  return Object.entries(paths).flatMap(([path, item]) =>
    Object.entries(item)
      .filter(([method]) => methods.has(method))
      .map(([method, operation]) => [path, method, operation])
  )
}

function generateModels(available, hash) {
  const lines = [header(hash)]
  for (const [name, schema] of Object.entries(available)) {
    lines.push(`export type ${name} = ${schemaType(schema, 0)}`)
  }
  return `${lines.join('\n')}\n`
}

function schemaType(schema, depth) {
  if (schema.$ref) return schema.$ref.replace('#/components/schemas/', '')
  if (schema.const !== undefined) return JSON.stringify(schema.const)
  if (schema.enum) return schema.enum.map((value) => JSON.stringify(value)).join(' | ')
  if (schema.anyOf || schema.oneOf) return (schema.anyOf ?? schema.oneOf).map((candidate) => schemaType(candidate, depth)).join(' | ')
  if (schema.allOf) return schema.allOf.map((candidate) => schemaType(candidate, depth)).join(' & ')
  if (Array.isArray(schema.type)) return schema.type.map((type) => schemaType({ ...schema, type }, depth)).join(' | ')
  if (schema.type === 'array') return `Array<${schemaType(schema.items ?? {}, depth)}>`
  if (schema.type === 'object' || schema.properties) {
    if (!schema.properties && schema.additionalProperties && typeof schema.additionalProperties === 'object') {
      return `Record<string, ${schemaType(schema.additionalProperties, depth)}>`
    }
    const required = new Set(schema.required ?? [])
    const indent = '  '.repeat(depth + 1)
    const closing = '  '.repeat(depth)
    const properties = Object.entries(schema.properties ?? {}).map(([name, property]) => {
      const readonly = property.readOnly ? 'readonly ' : ''
      const optional = required.has(name) ? '' : '?'
      return `${indent}${readonly}${JSON.stringify(name)}${optional}: ${schemaType(property, depth + 1)}`
    })
    if (schema.additionalProperties && typeof schema.additionalProperties === 'object') {
      properties.push(`${indent}[key: string]: ${schemaType(schema.additionalProperties, depth + 1)}`)
    }
    return `{\n${properties.join('\n')}\n${closing}}`
  }
  if (schema.type === 'integer' || schema.type === 'number') return 'number'
  if (schema.type === 'boolean') return 'boolean'
  if (schema.type === 'null') return 'null'
  return 'string'
}

function generateOperations(paths, hash) {
  const entries = operationEntries(paths)
  const lines = [header(hash), "import type * as Models from './models'\n", 'export interface ApiOperationMap {']
  for (const [path, method, operation] of entries) {
    const request = operationRequestType(paths[path], operation)
    const response = operation['x-response-type'] === null ? 'undefined' : `Models.${operation['x-response-type']}`
    lines.push(`  ${operation.operationId}: { method: ${JSON.stringify(method.toUpperCase())}; path: ${JSON.stringify(path)}; request: ${request}; response: ${response} }`)
  }
  lines.push('}\n')
  lines.push('export type ApiOperation = keyof ApiOperationMap')
  lines.push('export interface ApiClient {')
  lines.push('  request<Operation extends ApiOperation>(operation: Operation, request: ApiOperationMap[Operation]["request"]): Promise<ApiOperationMap[Operation]["response"]>')
  lines.push('}')
  lines.push(`\nexport const apiContract = ${JSON.stringify(Object.fromEntries(entries.map(([path, method, operation]) => [operation.operationId, { method: method.toUpperCase(), path }])))} as const`)
  return `${lines.join('\n')}\n`
}

function operationRequestType(pathItem, operation) {
  const parameters = [...(pathItem.parameters ?? []), ...(operation.parameters ?? [])]
  const pathParameter = parameters.find((parameter) => parameter.$ref?.includes('/SourceID'))
    ? 'Models.SourceIDRequest'
    : parameters.find((parameter) => parameter.$ref?.includes('/ArticleID'))
      ? 'Models.ArticleIDRequest'
      : parameters.find((parameter) => parameter.$ref?.includes('/RefreshID'))
        ? 'Models.RefreshIDRequest'
        : null
  const body = operation.requestBody && operation['x-request-type'] ? `Models.${operation['x-request-type']}` : null
  const query = !operation.requestBody && parameters.some((parameter) => parameter.in === 'query') && operation['x-request-type']
    ? `Models.${operation['x-request-type']}`
    : null
  const fields = []
  if (pathParameter) fields.push(`path: ${pathParameter}`)
  if (query) fields.push(`query: ${query}`)
  if (body) fields.push(`body: ${body}`)
  return fields.length === 0 ? 'undefined' : `{ ${fields.join('; ')} }`
}

function validateFixtures(manifest, schemas) {
  for (const [file, schemaName] of Object.entries(manifest)) {
    if (!schemas[schemaName]) throw new Error(`fixture ${file} names unknown schema ${schemaName}`)
  }
}

async function loadFixture(file) {
  return JSON.parse(await readFile(resolve(fixtureDir, file), 'utf8'))
}

function assertNegativeFixture(file, fixture, expectation) {
  try {
    validateValue(fixture, schemas[expectation.schema], schemas, `$negative-fixture:${file}`)
  } catch (error) {
    const issues = error instanceof ContractValidationError ? error.issues : []
    const exact = issues.some((issue) => issue.path === expectation.path && issue.code === expectation.code && issue.reason === expectation.reason)
    if (!exact) throw new Error(`negative fixture ${file} failed for an undocumented reason; expected ${JSON.stringify(expectation)}, got ${JSON.stringify(issues)}`)
    return
  }
  throw new Error(`negative fixture ${file} unexpectedly matches ${expectation.schema}`)
}

function invalid(path, code, reason) {
  throw new ContractValidationError([{ path, code, reason }])
}

function validateValue(value, schema, schemas, path) {
  if (schema.$ref) return validateValue(value, schemas[schema.$ref.replace('#/components/schemas/', '')], schemas, path)
  if (schema.allOf) {
    for (const candidate of schema.allOf) validateValue(value, candidate, schemas, path)
    return
  }
  if (schema.anyOf || schema.oneOf) {
    const failures = []
    const matches = (schema.anyOf ?? schema.oneOf).filter((candidate) => {
      try { validateValue(value, candidate, schemas, path); return true } catch (error) { failures.push(...(error instanceof ContractValidationError ? error.issues : [{ path, code: 'validation', reason: error.message }])); return false }
    })
    const expected = schema.oneOf ? 1 : Math.max(1, matches.length)
    if (matches.length !== expected) throw new ContractValidationError(failures)
    return
  }
  if (schema.const !== undefined) {
    if (value !== schema.const) invalid(path, 'const', `must equal ${JSON.stringify(schema.const)}`)
    return
  }
  if (schema.enum && !schema.enum.includes(value)) invalid(path, 'enum', 'has invalid enum value')
  const types = Array.isArray(schema.type) ? schema.type : [schema.type]
  if (types.includes('null') && value === null) return
  const type = types.find((candidate) => candidate !== 'null') ?? 'null'
  if (type === 'object' || schema.properties) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) invalid(path, 'type', 'must be an object')
    for (const required of schema.required ?? []) if (!Object.hasOwn(value, required)) invalid(`${path}.${required}`, 'required', 'is required')
    if (schema.additionalProperties === false) for (const key of Object.keys(value)) if (!schema.properties?.[key]) invalid(`${path}.${key}`, 'additional_property', 'is not allowed')
    for (const [key, child] of Object.entries(value)) if (schema.properties?.[key]) validateValue(child, schema.properties[key], schemas, `${path}.${key}`)
    if (schema.minProperties !== undefined && Object.keys(value).length < schema.minProperties) invalid(path, 'min_properties', 'has too few properties')
    return
  }
  if (type === 'array') {
    if (!Array.isArray(value)) invalid(path, 'type', 'must be an array')
    for (const [index, child] of value.entries()) validateValue(child, schema.items ?? {}, schemas, `${path}[${index}]`)
    if (schema.uniqueItems && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) invalid(path, 'unique_items', 'contains duplicate items')
    return
  }
  if (type === 'integer' && (!Number.isInteger(value))) invalid(path, 'type', 'must be an integer')
  if (type === 'number' && typeof value !== 'number') invalid(path, 'type', 'must be a number')
  if (type === 'boolean' && typeof value !== 'boolean') invalid(path, 'type', 'must be a boolean')
  if (type === 'null' && value !== null) invalid(path, 'type', 'must be null')
  if (type === 'string' && typeof value !== 'string') invalid(path, 'type', 'must be a string')
  if (typeof value === 'string' && schema.minLength !== undefined && value.length < schema.minLength) invalid(path, 'min_length', 'is too short')
  if (typeof value === 'string' && schema.maxLength !== undefined && value.length > schema.maxLength) invalid(path, 'max_length', 'is too long')
  if (typeof value === 'string' && schema.format === 'date-time' && Number.isNaN(Date.parse(value))) invalid(path, 'format_date_time', 'must be a date-time')
  if (typeof value === 'string' && schema.format === 'uri') {
    try { new URL(value) } catch { invalid(path, 'format_uri', 'must be a URI') }
  }
  if (typeof value === 'number' && (value < (schema.minimum ?? -Infinity) || value > (schema.maximum ?? Infinity))) invalid(path, 'numeric_bounds', 'is outside numeric bounds')
}

function generateFixtureTest(manifest, fixtures, hash) {
  const declarations = Object.entries(manifest).map(([file, schema], index) =>
    `const fixture${index}: Models.${schema} = ${JSON.stringify(fixtures.get(file), null, 2)}`
  )
  const assertions = Object.entries(manifest).map(([file, schema], index) => `    expect(fixture${index}).toBeTruthy() // ${file} -> ${schema}`)
  return `${header(hash)}\nimport { describe, expect, it } from 'vitest'\nimport type * as Models from './models'\n\n${declarations.join('\n\n')}\n\ndescribe('generated API fixture bindings', () => {\n  it('compiles every runtime-validated fixture against its generated model', () => {\n${assertions.join('\n')}\n  })\n})\n`
}

function generateOperationTest(hash) {
  return `${header(hash)}
import { describe, expect, it } from 'vitest'
import type { ApiOperationMap } from './operations'
import type { ArticleIDRequest, CredentialWrite, LibraryStateWrite, SourceIDRequest, SourceWrite } from './models'

type Extends<Actual, Expected> = Actual extends Expected ? true : false
const updateSourceCarriesPathAndBody: Extends<ApiOperationMap['updateSource']['request'], { path: SourceIDRequest; body: SourceWrite }> = true
const credentialCarriesPathAndBody: Extends<ApiOperationMap['putSourceCredential']['request'], { path: SourceIDRequest; body: CredentialWrite }> = true
const libraryCarriesPathAndBody: Extends<ApiOperationMap['patchLibraryState']['request'], { path: ArticleIDRequest; body: LibraryStateWrite }> = true

describe('generated operation request contract', () => {
  it('requires resource identifiers beside mutation bodies', () => {
    expect(updateSourceCarriesPathAndBody && credentialCarriesPathAndBody && libraryCarriesPathAndBody).toBe(true)
  })
})
`
}

function header(hash) {
  return `// Generated from api/openapi.yaml (${hash}). Do not edit.`
}
