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

const specText = await readFile(specPath, 'utf8')
const spec = JSON.parse(specText)
validateOpenAPI(spec)

const digest = createHash('sha256').update(specText).digest('hex').slice(0, 16)
const schemas = spec.components.schemas
const models = generateModels(schemas, digest)
const operations = generateOperations(spec.paths, schemas, digest)

const manifest = JSON.parse(await readFile(resolve(fixtureDir, 'manifest.json'), 'utf8'))
validateFixtures(manifest, schemas)
for (const [file, schemaName] of Object.entries(manifest)) await validateFixture(file, schemaName, schemas)
const fixtureTest = generateFixtureTest(manifest, digest)

if (!check) await mkdir(outputDir, { recursive: true })
await emit('models.ts', models)
await emit('operations.ts', operations)
await emit('fixture-contract.test.ts', fixtureTest)

console.log(`RESULT OK openapi_valid=true operations=${operationEntries(spec.paths).length} schemas=${Object.keys(schemas).length}`)
console.log(`RESULT OK api_fixtures_valid=true fixtures=${Object.keys(manifest).length}`)
console.log(`RESULT OK api_types_${check ? 'current' : 'generated'}=true digest=${digest}`)

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

function generateOperations(paths, schemas, hash) {
  const entries = operationEntries(paths)
  const lines = [header(hash), "import type * as Models from './models'\n", 'export interface ApiOperationMap {']
  for (const [path, method, operation] of entries) {
    const request = operation['x-request-type'] === null ? 'undefined' : `Models.${operation['x-request-type']}`
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

function validateFixtures(manifest, schemas) {
  for (const [file, schemaName] of Object.entries(manifest)) {
    if (!schemas[schemaName]) throw new Error(`fixture ${file} names unknown schema ${schemaName}`)
  }
}

async function loadFixture(file) {
  return JSON.parse(await readFile(resolve(fixtureDir, file), 'utf8'))
}

async function validateFixture(file, schemaName, schemas) {
  validateValue(await loadFixture(file), schemas[schemaName], schemas, `$fixture:${file}`)
}

function validateValue(value, schema, schemas, path) {
  if (schema.$ref) return validateValue(value, schemas[schema.$ref.replace('#/components/schemas/', '')], schemas, path)
  if (schema.anyOf || schema.oneOf) {
    const matches = (schema.anyOf ?? schema.oneOf).filter((candidate) => {
      try { validateValue(value, candidate, schemas, path); return true } catch { return false }
    })
    const expected = schema.oneOf ? 1 : Math.max(1, matches.length)
    if (matches.length !== expected) throw new Error(`${path} does not match ${schema.oneOf ? 'exactly one' : 'any'} variant`)
    return
  }
  if (schema.const !== undefined && value !== schema.const) throw new Error(`${path} must equal ${schema.const}`)
  if (schema.enum && !schema.enum.includes(value)) throw new Error(`${path} has invalid enum value`)
  const types = Array.isArray(schema.type) ? schema.type : [schema.type]
  if (types.includes('null') && value === null) return
  const type = types.find((candidate) => candidate !== 'null')
  if (type === 'object' || schema.properties) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${path} must be an object`)
    for (const required of schema.required ?? []) if (!Object.hasOwn(value, required)) throw new Error(`${path}.${required} is required`)
    if (schema.additionalProperties === false) for (const key of Object.keys(value)) if (!schema.properties?.[key]) throw new Error(`${path}.${key} is not allowed`)
    for (const [key, child] of Object.entries(value)) if (schema.properties?.[key]) validateValue(child, schema.properties[key], schemas, `${path}.${key}`)
    return
  }
  if (type === 'array') {
    if (!Array.isArray(value)) throw new Error(`${path} must be an array`)
    for (const [index, child] of value.entries()) validateValue(child, schema.items ?? {}, schemas, `${path}[${index}]`)
    return
  }
  if (type === 'integer' && (!Number.isInteger(value))) throw new Error(`${path} must be an integer`)
  if (type === 'number' && typeof value !== 'number') throw new Error(`${path} must be a number`)
  if (type === 'boolean' && typeof value !== 'boolean') throw new Error(`${path} must be a boolean`)
  if (type === 'string' && typeof value !== 'string') throw new Error(`${path} must be a string`)
  if (typeof value === 'number' && (value < (schema.minimum ?? -Infinity) || value > (schema.maximum ?? Infinity))) throw new Error(`${path} is outside numeric bounds`)
}

function generateFixtureTest(manifest, hash) {
  const imports = Object.keys(manifest).map((file, index) => `import fixture${index} from '../../../../test/fixtures/api/${file}'`)
  const assertions = Object.entries(manifest).map(([file, schema], index) => `    expect(fixture${index}).toBeTruthy() // ${file} -> ${schema}`)
  return `${header(hash)}\nimport { describe, expect, it } from 'vitest'\n${imports.join('\n')}\n\ndescribe('generated API fixture bindings', () => {\n  it('imports every runtime-validated fixture', () => {\n${assertions.join('\n')}\n  })\n})\n`
}

function header(hash) {
  return `// Generated from api/openapi.yaml (${hash}). Do not edit.`
}
