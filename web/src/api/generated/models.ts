// Generated from api/openapi.yaml (1fe1b3d388739840). Do not edit.
export type Health = {
  "status": "ready"
  "version": string
}
export type OptionalStringSignal = {
  "value"?: string | null
  "present": boolean
  "enabled": boolean
}
export type OptionalIntegerSignal = {
  "value"?: number | null
  "present": boolean
  "enabled": boolean
}
export type LocationValue = {
  "country": string
  "region": string
  "city": OptionalStringSignal
}
export type OptionalLocationSignal = {
  "value"?: LocationValue | null
  "present": boolean
  "enabled": boolean
}
export type WeightedInterest = {
  "name": string
  "weight": number
}
export type ProfileWrite = {
  "interests": Array<WeightedInterest>
  "preferredSourceIds": Array<string>
  "location": OptionalLocationSignal
  "age": OptionalIntegerSignal
  "gender": OptionalStringSignal
}
export type Profile = {
  "id": "local-profile"
  "interests": Array<WeightedInterest>
  "preferredSourceIds": Array<string>
  "location": OptionalLocationSignal
  "age": OptionalIntegerSignal
  "gender": OptionalStringSignal
  readonly "updatedAt": string
}
export type SignalWeight = {
  "enabled": boolean
  "weight": number
}
export type RankingConfigurationWrite = {
  "recency": SignalWeight
  "interest": SignalWeight
  "sourcePreference": SignalWeight
  "behavior": SignalWeight
  "location": SignalWeight
  "age": SignalWeight
  "gender": SignalWeight
  "textSimilarity": SignalWeight
}
export type RankingConfiguration = {
  "recency": SignalWeight
  "interest": SignalWeight
  "sourcePreference": SignalWeight
  "behavior": SignalWeight
  "location": SignalWeight
  "age": SignalWeight
  "gender": SignalWeight
  "textSimilarity": SignalWeight
  readonly "perDemographicCap": number
  readonly "totalDemographicCap": number
  readonly "normalizationVersion": string
}
export type FeedAdapterConfiguration = {
  "format": "auto" | "rss" | "atom"
}
export type APIAdapterConfiguration = {
  "provider": string
  "pageSize": number
}
export type ScraperAdapterConfiguration = {
  "articleSelector": string
  "titleSelector": string
  "excerptSelector"?: string
  "contentSelector"?: string
}
export type ScraperPolicy = {
  "status": "not_applicable" | "pending" | "approved" | "rejected"
  "termsUrl": string | null
  "robotsUrl": string | null
  "reviewedAt": string | null
  "reviewNotes": string | null
}
export type NotApplicableScraperPolicy = ScraperPolicy & {
  "status": "not_applicable"
  "termsUrl": null
  "robotsUrl": null
  "reviewedAt": null
  "reviewNotes": null
}
export type ApprovedScraperPolicy = ScraperPolicy & {
  "status": "approved"
  "reviewedAt": string
}
export type SourceWrite = FeedSourceWrite | APISourceWrite | DisabledScraperSourceWrite | EnabledScraperSourceWrite
export type SourceWriteBase = {
  "name": string
  "url": string
  "kind": "feed" | "api" | "scraper"
  "enabled": boolean
  "contentPermission": "metadata_only" | "full_content_allowed"
  "adapterConfig": FeedAdapterConfiguration | APIAdapterConfiguration | ScraperAdapterConfiguration
  "scraperPolicy": ScraperPolicy
}
export type FeedSourceWrite = SourceWriteBase & {
  "kind"?: "feed"
  "adapterConfig"?: FeedAdapterConfiguration
  "scraperPolicy"?: NotApplicableScraperPolicy
}
export type APISourceWrite = SourceWriteBase & {
  "kind"?: "api"
  "adapterConfig"?: APIAdapterConfiguration
  "scraperPolicy"?: NotApplicableScraperPolicy
}
export type DisabledScraperSourceWrite = SourceWriteBase & {
  "kind"?: "scraper"
  "enabled"?: false
  "adapterConfig"?: ScraperAdapterConfiguration
}
export type EnabledScraperSourceWrite = SourceWriteBase & {
  "kind"?: "scraper"
  "enabled"?: true
  "adapterConfig"?: ScraperAdapterConfiguration
  "scraperPolicy"?: ApprovedScraperPolicy
}
export type Source = {
  "id": string
  "name": string
  "url": string
  "kind": "feed" | "api" | "scraper"
  "enabled": boolean
  "contentPermission": "metadata_only" | "full_content_allowed"
  "adapterConfig": FeedAdapterConfiguration | APIAdapterConfiguration | ScraperAdapterConfiguration
  "scraperPolicy": ScraperPolicy
  readonly "credentialConfigured": boolean
  readonly "lastSuccessAt": string | null
  readonly "lastError": string | null
  readonly "retryAfter": string | null
}
export type SourceList = {
  "items": Array<Source>
}
export type CredentialWrite = {
  "secret": string
}
export type CredentialStatus = {
  readonly "configured": boolean
}
export type SourceRefreshOutcome = {
  "sourceId": string
  "fetched": number
  "inserted": number
  "updated": number
  "skipped": number
  "failed": number
  "errorCode": string | null
  "errorSummary": string | null
}
export type RefreshRun = {
  "id": string
  "status": "running" | "succeeded" | "partial_success" | "failed" | "cancelled"
  "startedAt": string
  "finishedAt": string | null
  "outcomes": Array<SourceRefreshOutcome>
}
export type LibraryState = {
  "articleId": string
  "readAt": string | null
  "savedAt": string | null
  "hiddenAt": string | null
}
export type LibraryStateWrite = {
  "read"?: boolean
  "saved"?: boolean
  "hidden"?: boolean
}
export type ScoreContribution = {
  "signal": "recency" | "interest" | "source_preference" | "behavior" | "location" | "age" | "gender" | "text_similarity"
  "rawScore": number
  "weight": number
  "weightedScore": number
  "reasonCode": string
  "reasonValues": Record<string, string>
}
export type RankingResult = {
  "score": number
  "contributions": Array<ScoreContribution>
  "algorithmVersion": string
  "calculatedAt": string
}
export type ArticleSummary = {
  "id": string
  "sourceId": string
  "canonicalUrl": string
  "title": string
  "author"?: string
  "publishedAt": string | null
  "fetchedAt": string
  "excerpt"?: string
  "contentPermission": "metadata_only" | "full_content_allowed"
  "language"?: string
  "topics": Array<string>
  "library": LibraryState
  "ranking": RankingResult
}
export type ArticleDetail = MetadataOnlyArticleDetail | FullContentArticleDetail
export type MetadataOnlyArticleDetail = {
  "article": ArticleSummary & {
    "contentPermission"?: "metadata_only"
  }
  "fullContent": null
}
export type FullContentArticleDetail = {
  "article": ArticleSummary & {
    "contentPermission"?: "full_content_allowed"
  }
  "fullContent": string
}
export type FeedPage = {
  "items": Array<ArticleSummary>
  "nextCursor": string | null
}
export type FeedQuery = {
  "cursor"?: string
  "limit"?: number
  "sourceId"?: Array<string>
  "read"?: boolean
  "saved"?: boolean
  "includeHidden"?: boolean
  "text"?: string
  "publishedAfter"?: string
  "publishedBefore"?: string
}
export type SourceIDRequest = {
  "sourceId": string
}
export type ArticleIDRequest = {
  "articleId": string
}
export type RefreshIDRequest = {
  "refreshId": string
}
export type APIErrorField = {
  "path": string
  "code": string
  "message": string
}
export type APIError = {
  "code": "validation_failed" | "not_found" | "conflict" | "refresh_active" | "unsupported_platform" | "unavailable" | "internal_error"
  "message": string
  "correlationId": string
  "fields": Array<APIErrorField>
}
