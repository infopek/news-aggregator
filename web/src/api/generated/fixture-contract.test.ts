// Generated from api/openapi.yaml (b6c730e56578ce87). Do not edit.
import { describe, expect, it } from 'vitest'
import fixture0 from '../../../../test/fixtures/api/profile.json'
import fixture1 from '../../../../test/fixtures/api/source.json'
import fixture2 from '../../../../test/fixtures/api/refresh-partial.json'
import fixture3 from '../../../../test/fixtures/api/feed-empty.json'
import fixture4 from '../../../../test/fixtures/api/feed-page.json'
import fixture5 from '../../../../test/fixtures/api/article-metadata.json'
import fixture6 from '../../../../test/fixtures/api/validation-error.json'
import fixture7 from '../../../../test/fixtures/api/credential-status.json'

describe('generated API fixture bindings', () => {
  it('imports every runtime-validated fixture', () => {
    expect(fixture0).toBeTruthy() // profile.json -> Profile
    expect(fixture1).toBeTruthy() // source.json -> Source
    expect(fixture2).toBeTruthy() // refresh-partial.json -> RefreshRun
    expect(fixture3).toBeTruthy() // feed-empty.json -> FeedPage
    expect(fixture4).toBeTruthy() // feed-page.json -> FeedPage
    expect(fixture5).toBeTruthy() // article-metadata.json -> ArticleDetail
    expect(fixture6).toBeTruthy() // validation-error.json -> APIError
    expect(fixture7).toBeTruthy() // credential-status.json -> CredentialStatus
  })
})
