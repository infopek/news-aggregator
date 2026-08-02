import type { CredentialWrite, FeedQuery, LibraryStateWrite, ProfileWrite, RankingConfigurationWrite, SourceWrite } from '../api/generated/models'
import type { ServerApi } from '../api/server-api'
import { toUserSafeError, type UserSafeError } from './errors'
import { queryKeys } from './query-keys'
import type { ServerStateClient } from './query-client'

export type MutationResult<T> = { status: 'success'; data: T } | { status: 'error'; error: UserSafeError }

export class ServerMutations {
  private readonly active = new Map<string, AbortController>()

  constructor(private readonly api: ServerApi, private readonly cache: ServerStateClient) {}

  updateProfile(body: ProfileWrite) {
    return this.run('profile', (signal) => this.api.updateProfile(body, signal), (profile) => this.cache.set(queryKeys.profile(), profile), [queryKeys.profile(), queryKeys.feeds()])
  }

  updateRanking(body: RankingConfigurationWrite) {
    return this.run('ranking', (signal) => this.api.updateRanking(body, signal), (ranking) => this.cache.set(queryKeys.ranking(), ranking), [queryKeys.ranking(), queryKeys.feeds()])
  }

  createSource(body: SourceWrite) {
    return this.run('sources', (signal) => this.api.createSource(body, signal), (source) => this.cache.set(queryKeys.source(source.id), source), [queryKeys.sources(), queryKeys.feeds()])
  }

  updateSource(sourceId: string, body: SourceWrite) {
    return this.run(`source:${sourceId}`, (signal) => this.api.updateSource(sourceId, body, signal), (source) => this.cache.set(queryKeys.source(source.id), source), [queryKeys.sources(), queryKeys.feeds()])
  }

  deleteSource(sourceId: string) {
    return this.run(`source:${sourceId}`, (signal) => this.api.deleteSource(sourceId, signal), () => undefined, [queryKeys.source(sourceId), queryKeys.sources(), queryKeys.feeds()])
  }

  writeCredential(sourceId: string, credential: CredentialWrite) {
    return this.run(`credential:${sourceId}`, (signal) => this.api.writeCredential(sourceId, credential, signal), () => undefined, [queryKeys.source(sourceId), queryKeys.sources()], true)
  }

  deleteCredential(sourceId: string) {
    return this.run(`credential:${sourceId}`, (signal) => this.api.deleteCredential(sourceId, signal), () => undefined, [queryKeys.source(sourceId), queryKeys.sources()])
  }

  updateLibrary(articleId: string, body: LibraryStateWrite, activeFeed: FeedQuery = {}) {
    return this.run(`library:${articleId}`, (signal) => this.api.updateLibrary(articleId, body, signal), () => undefined, [queryKeys.article(articleId), queryKeys.feed(activeFeed)])
  }

  private async run<T>(sequenceKey: string, operation: (signal: AbortSignal) => Promise<T>, reconcile: (value: T) => void, invalidate: ReturnType<typeof queryKeys.profile>[], redactError = false): Promise<MutationResult<T>> {
    this.active.get(sequenceKey)?.abort()
    const controller = new AbortController()
    this.active.set(sequenceKey, controller)
    try {
      const value = await operation(controller.signal)
      if (this.active.get(sequenceKey) !== controller) return { status: 'error', error: { family: 'conflict', message: 'A newer change replaced this one.', fields: [] } }
      this.cache.invalidate(...invalidate)
      reconcile(value)
      return { status: 'success', data: value }
    } catch (error) {
      if (this.active.get(sequenceKey) !== controller) return { status: 'error', error: { family: 'conflict', message: 'A newer change replaced this one.', fields: [] } }
      if (this.active.get(sequenceKey) === controller) this.cache.invalidate(...invalidate)
      return { status: 'error', error: redactError ? { family: 'unexpected', message: 'The credential could not be saved. Try again.', fields: [] } : toUserSafeError(error) }
    } finally {
      if (this.active.get(sequenceKey) === controller) this.active.delete(sequenceKey)
    }
  }
}
