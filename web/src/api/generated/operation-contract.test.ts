// Generated from api/openapi.yaml (ee0ab217f61d00a7). Do not edit.
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
