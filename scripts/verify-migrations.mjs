#!/usr/bin/env node

import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { DatabaseSync } from 'node:sqlite'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const migrationDir = join(root, 'db', 'migrations')
const inventoryPath = join(root, 'test', 'fixtures', 'db', 'schema_inventory.json')

function migrationFiles() {
  const files = readdirSync(migrationDir).filter((name) => /^\d{4}_.+\.sql$/.test(name)).sort()
  assert(files.length > 0, 'at least one migration is required')
  assert.deepEqual(files.map((name) => Number(name.slice(0, 4))), files.map((_, index) => index + 1), 'migration versions must be contiguous from 0001')
  return files
}

function prepare(db) {
  db.exec('PRAGMA foreign_keys = ON')
  db.exec('CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at_ms INTEGER NOT NULL)')
}

function validateHistory(db, files) {
  const rows = db.prepare('SELECT version, name FROM schema_migrations ORDER BY version').all()
  assert(rows.length <= files.length, `database schema version ${rows.length} is newer than supported ${files.length}`)
  for (let index = 0; index < rows.length; index += 1) {
    const expectedVersion = index + 1
    assert.equal(rows[index].version, expectedVersion, `migration history is missing or malformed at version ${expectedVersion}`)
    assert.equal(rows[index].name, files[index], `migration filename mismatch at version ${expectedVersion}`)
  }
  return rows.length
}

function apply(db, files) {
  prepare(db)
  const current = validateHistory(db, files)
  for (let index = current; index < files.length; index += 1) {
    const name = files[index]
    db.exec('BEGIN IMMEDIATE')
    try {
      db.exec(readFileSync(join(migrationDir, name), 'utf8'))
      db.prepare('INSERT INTO schema_migrations(version, name, applied_at_ms) VALUES (?, ?, 0)').run(index + 1, name)
      db.exec('COMMIT')
    } catch (error) {
      if (db.isTransaction) db.exec('ROLLBACK')
      throw error
    }
  }
  return validateHistory(db, files)
}

function expectConstraint(db, sql, ...values) {
  assert.throws(() => db.prepare(sql).run(...values), /constraint|foreign key/i)
}

function expectHistoryRejected(files, rows) {
  const db = new DatabaseSync(':memory:')
  prepare(db)
  for (const row of rows) db.prepare('INSERT INTO schema_migrations(version,name,applied_at_ms) VALUES (?,?,0)').run(...row)
  assert.throws(() => apply(db, files), /migration|schema version|filename/i)
  db.close()
}

function insertProfile(db) {
  db.prepare("INSERT INTO profiles(id, updated_at_ms) VALUES ('local-profile', 0)").run()
  db.prepare("UPDATE profiles SET location_present=1, location_enabled=1, country='HU', region='Budapest', city_present=1, city_enabled=1, city='Budapest', age_present=1, age_enabled=0, age=30, gender_present=1, gender_enabled=1, gender='unspecified' WHERE id='local-profile'").run()
}

function assertProfileSignals(db) {
  expectConstraint(db, "INSERT INTO profiles(id,location_present,country,updated_at_ms) VALUES ('local-profile',0,'HU',0)")
  expectConstraint(db, "INSERT INTO profiles(id,location_present,location_enabled,country,region,city_present,city,updated_at_ms) VALUES ('local-profile',1,1,'HU','BP',0,'leak',0)")
  expectConstraint(db, "INSERT INTO profiles(id,age_present,age,updated_at_ms) VALUES ('local-profile',0,25,0)")
  expectConstraint(db, "INSERT INTO profiles(id,age_present,updated_at_ms) VALUES ('local-profile',1,0)")
  expectConstraint(db, "INSERT INTO profiles(id,gender_present,gender,updated_at_ms) VALUES ('local-profile',0,'x',0)")
}

function insertSources(db) {
  const sql = 'INSERT INTO sources(id,name,url,kind,enabled,content_permission,feed_format,api_provider,api_page_size,scraper_article_selector,scraper_title_selector,scraper_policy_status,scraper_reviewed_at_ms) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)'
  db.prepare(sql).run('feed', 'Feed', 'https://example.test/feed', 'feed', 1, 'metadata_only', 'auto', null, null, null, null, 'not_applicable', null)
  db.prepare(sql).run('api', 'API', 'https://example.test/api', 'api', 1, 'metadata_only', null, 'example', 50, null, null, 'not_applicable', null)
  db.prepare(sql).run('scraper', 'Scraper', 'https://example.test/site', 'scraper', 1, 'full_content_allowed', null, null, null, 'article', 'h1', 'approved', 1)
  expectConstraint(db, sql, 'bad-feed', 'Bad', 'https://bad.test/feed', 'feed', 1, 'metadata_only', 'auto', 'leak', 10, null, null, 'not_applicable', null)
  expectConstraint(db, sql, 'bad-api', 'Bad', 'https://bad.test/api', 'api', 1, 'metadata_only', null, 'provider', null, null, null, 'not_applicable', null)
  expectConstraint(db, sql, 'bad-scraper', 'Bad', 'https://bad.test/site', 'scraper', 1, 'metadata_only', null, null, null, 'article', 'h1', 'pending', null)
  expectConstraint(db, "INSERT INTO sources(id,name,url,kind,enabled,content_permission,feed_format,scraper_policy_status,scraper_terms_url) VALUES ('policy-leak','Bad','https://bad.test/policy','feed',1,'metadata_only','auto','not_applicable','https://terms.test')")
}

function insertArticle(db, id = 'article-1', fingerprint = 'fp-1', canonical = 'https://example.test/a') {
  db.prepare('INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,fetched_at_ms,content_permission) VALUES (?,?,?,?,?,?,?)').run(id, fingerprint, 'feed', canonical, 'Article', 1, 'metadata_only')
}

function assertPersistenceConstraints(db) {
  insertArticle(db)
  expectConstraint(db, 'INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,fetched_at_ms,content_permission) VALUES (?,?,?,?,?,?,?)', 'canonical-dup', 'fp-2', 'api', 'https://example.test/a', 'Dup', 1, 'metadata_only')
  expectConstraint(db, 'INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,fetched_at_ms,content_permission) VALUES (?,?,?,?,?,?,?)', 'fingerprint-dup', 'fp-1', 'api', 'https://example.test/b', 'Dup', 1, 'metadata_only')
  expectConstraint(db, 'INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,fetched_at_ms,full_content,content_permission) VALUES (?,?,?,?,?,?,?,?)', 'content-leak', 'fp-3', 'api', 'https://example.test/c', 'Leak', 1, 'unpermitted body', 'metadata_only')
  db.prepare('INSERT INTO article_sources(article_id,source_id,external_id,first_seen_at_ms,last_seen_at_ms) VALUES (?,?,?,?,?)').run('article-1', 'feed', 'external-1', 1, 1)
  insertArticle(db, 'article-2', 'fp-2', 'https://example.test/b')
  expectConstraint(db, 'INSERT INTO article_sources(article_id,source_id,external_id,first_seen_at_ms,last_seen_at_ms) VALUES (?,?,?,?,?)', 'article-2', 'feed', 'external-1', 1, 1)
  expectConstraint(db, "INSERT INTO library_states(article_id) VALUES ('missing')")
  db.prepare("INSERT INTO library_states(article_id,read_at_ms) VALUES ('article-1',1)").run()
  db.prepare("INSERT INTO feed_filter_state(profile_id,source_id,read_filter,saved_only,include_hidden,updated_at_ms) VALUES ('local-profile','feed','unread',1,0,1)").run()
  expectConstraint(db, "INSERT INTO feed_filter_state(profile_id,read_filter,updated_at_ms) VALUES ('other','all',1)")

  const config = "INSERT INTO ranking_configurations(profile_id,recency_enabled,recency_weight,interest_enabled,interest_weight,source_enabled,source_weight,behavior_enabled,behavior_weight,location_enabled,location_weight,age_enabled,age_weight,gender_enabled,gender_weight,text_similarity_enabled,text_similarity_weight,per_demographic_cap,total_demographic_cap,normalization_version) VALUES ('local-profile',1,.2,1,.2,1,.1,1,.1,0,.1,0,.1,0,.1,1,.2,.1,.2,'v1')"
  db.exec(config)
  db.prepare("INSERT INTO ranking_results(article_id,score,algorithm_version,calculated_at_ms) VALUES ('article-1',.8,'v1',1)").run()
  db.prepare("INSERT INTO ranking_contributions(article_id,ordinal,signal,raw_score,weight,weighted_score,reason_code) VALUES ('article-1',0,'recency',1,.2,.2,'recent')").run()
  expectConstraint(db, "INSERT INTO ranking_contributions(article_id,ordinal,signal,raw_score,weight,weighted_score,reason_code) VALUES ('article-1',1,'unknown',1,.2,.2,'bad')")
  expectConstraint(db, "INSERT INTO ranking_results(article_id,score,algorithm_version,calculated_at_ms) VALUES ('article-2',.5,'',1)")

  db.prepare("INSERT INTO refresh_runs(id,started_at_ms,status) VALUES ('run-1',1,'running')").run()
  expectConstraint(db, "INSERT INTO refresh_runs(id,started_at_ms,status) VALUES ('run-2',2,'running')")
  expectConstraint(db, "INSERT INTO refresh_runs(id,started_at_ms,status) VALUES ('bad-status',2,'unknown')")
  db.prepare("INSERT INTO refresh_outcomes(refresh_run_id,source_id,fetched,inserted,updated,skipped,failed) VALUES ('run-1','feed',1,1,0,0,0)").run()
  expectConstraint(db, "INSERT INTO refresh_outcomes(refresh_run_id,source_id,fetched,inserted,updated,skipped,failed) VALUES ('run-1','api',-1,0,0,0,0)")
  db.prepare("UPDATE refresh_runs SET status='partial_success',finished_at_ms=3 WHERE id='run-1'").run()
  for (const status of ['succeeded', 'failed', 'cancelled']) {
    db.prepare('INSERT INTO refresh_runs(id,started_at_ms,finished_at_ms,status) VALUES (?,?,?,?)').run(`run-${status}`, 4, 5, status)
  }
}

function assertInventory(db) {
  const expected = JSON.parse(readFileSync(inventoryPath, 'utf8'))
  const tables = db.prepare("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").all().map((row) => row.name)
  const indexes = db.prepare("SELECT name FROM sqlite_schema WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name").all().map((row) => row.name)
  assert.deepEqual(tables, [...expected.tables].sort())
  assert.deepEqual(indexes, [...expected.indexes].sort())
  const schema = db.prepare('SELECT sql FROM sqlite_schema').all().map((row) => row.sql ?? '').join('\n').toLowerCase()
  for (const term of expected.forbidden_schema_terms) assert(!schema.includes(term.toLowerCase()), `forbidden schema term: ${term}`)
}

function assertQueryPlans(db) {
  const cases = [
    ['ranking_results_feed', 'SELECT article_id FROM ranking_results ORDER BY score DESC, calculated_at_ms DESC, article_id LIMIT 20'],
    ['articles_primary_source_feed', "SELECT id FROM articles WHERE primary_source_id='feed' ORDER BY published_at_ms DESC, id LIMIT 20"],
    ['article_tokens_term', "SELECT article_id FROM article_tokens WHERE term='news'"],
  ]
  for (const [index, query] of cases) {
    const plan = db.prepare(`EXPLAIN QUERY PLAN ${query}`).all().map((row) => row.detail).join(' ')
    assert(plan.includes(index), `expected query plan to use ${index}: ${plan}`)
  }
}

function assertInterrupted(files) {
  const db = new DatabaseSync(':memory:')
  prepare(db)
  db.exec('BEGIN IMMEDIATE')
  try {
    db.exec('CREATE TABLE interrupted(value TEXT); INSERT INTO missing_table VALUES (1)')
  } catch {
    db.exec('ROLLBACK')
  }
  assert.equal(db.prepare("SELECT count(*) AS count FROM sqlite_schema WHERE name='interrupted'").get().count, 0)
  assert.equal(apply(db, files), files.length)
  db.close()
}

function main() {
  const files = migrationFiles()
  const directory = mkdtempSync(join(tmpdir(), 'news-aggregator-migrations-'))
  try {
    const db = new DatabaseSync(join(directory, 'contract.db'))
    assert.equal(apply(db, files), files.length)
    const changes = db.prepare('SELECT total_changes() AS count').get().count
    assert.equal(apply(db, files), files.length)
    assert.equal(db.prepare('SELECT total_changes() AS count').get().count, changes)
    assertInventory(db)
    assertProfileSignals(db)
    insertProfile(db)
    insertSources(db)
    assertPersistenceConstraints(db)
    assertQueryPlans(db)
    db.close()
  } finally {
    rmSync(directory, { recursive: true, force: true })
  }
  assertInterrupted(files)
  expectHistoryRejected(files, [[1, files[0]], [2, 'future.sql']])
  expectHistoryRejected(files, [[2, 'gap.sql']])
  expectHistoryRejected(files, [[0, 'malformed.sql']])
  expectHistoryRejected(files, [[1, 'wrong-name.sql']])
  console.log(`RESULT OK migrations=${files.length} history=strict signals=closed adapters=closed constraints=complete query_plans=indexed secrets_absent=true`)
}

main()
