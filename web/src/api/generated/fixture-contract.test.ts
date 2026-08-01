// Generated from api/openapi.yaml (ee0ab217f61d00a7). Do not edit.
import { describe, expect, it } from 'vitest'
import type * as Models from './models'

const fixture0: Models.Profile = {
  "id": "local-profile",
  "interests": [
    {
      "name": "technology",
      "weight": 0.9
    }
  ],
  "preferredSourceIds": [
    "source-1"
  ],
  "location": {
    "present": true,
    "enabled": true,
    "value": {
      "country": "HU",
      "region": "Central Hungary",
      "city": {
        "present": true,
        "enabled": true,
        "value": "Budapest"
      }
    }
  },
  "age": {
    "present": true,
    "enabled": false,
    "value": 35
  },
  "gender": {
    "present": false,
    "enabled": false
  },
  "updatedAt": "2026-08-01T12:00:00Z"
}

const fixture1: Models.Source = {
  "id": "source-1",
  "name": "Example Feed",
  "url": "https://example.com/feed.xml",
  "kind": "feed",
  "enabled": true,
  "contentPermission": "metadata_only",
  "adapterConfig": {
    "format": "auto"
  },
  "scraperPolicy": {
    "status": "not_applicable",
    "termsUrl": null,
    "robotsUrl": null,
    "reviewedAt": null,
    "reviewNotes": null
  },
  "credentialConfigured": false,
  "lastSuccessAt": "2026-08-01T11:59:00Z",
  "lastError": null,
  "retryAfter": null
}

const fixture2: Models.RefreshRun = {
  "id": "refresh-1",
  "status": "partial_success",
  "startedAt": "2026-08-01T12:00:00Z",
  "finishedAt": "2026-08-01T12:00:04Z",
  "outcomes": [
    {
      "sourceId": "source-1",
      "fetched": 10,
      "inserted": 8,
      "updated": 1,
      "skipped": 1,
      "failed": 0,
      "errorCode": null,
      "errorSummary": null
    },
    {
      "sourceId": "source-2",
      "fetched": 0,
      "inserted": 0,
      "updated": 0,
      "skipped": 0,
      "failed": 1,
      "errorCode": "SOURCE_UNAVAILABLE",
      "errorSummary": "The source could not be reached."
    }
  ]
}

const fixture3: Models.FeedPage = {
  "items": [],
  "nextCursor": null
}

const fixture4: Models.FeedPage = {
  "items": [
    {
      "id": "article-1",
      "sourceId": "source-1",
      "canonicalUrl": "https://example.com/articles/local-ranking",
      "title": "Local ranking explained",
      "author": "Example Author",
      "publishedAt": "2026-08-01T10:00:00Z",
      "fetchedAt": "2026-08-01T10:05:00Z",
      "excerpt": "How an explainable local ranker orders the feed.",
      "contentPermission": "metadata_only",
      "language": "en",
      "topics": [
        "technology"
      ],
      "library": {
        "articleId": "article-1",
        "readAt": null,
        "savedAt": "2026-08-01T11:00:00Z",
        "hiddenAt": null
      },
      "ranking": {
        "score": 0.84,
        "contributions": [
          {
            "signal": "interest",
            "rawScore": 0.9,
            "weight": 0.5,
            "weightedScore": 0.45,
            "reasonCode": "INTEREST_MATCH",
            "reasonValues": {
              "topic": "technology"
            }
          }
        ],
        "algorithmVersion": "hybrid-v1",
        "calculatedAt": "2026-08-01T10:05:01Z"
      }
    }
  ],
  "nextCursor": "eyJvZmZzZXQiOjF9"
}

const fixture5: Models.ArticleDetail = {
  "article": {
    "id": "article-1",
    "sourceId": "source-1",
    "canonicalUrl": "https://example.com/articles/local-ranking",
    "title": "Local ranking explained",
    "author": "Example Author",
    "publishedAt": "2026-08-01T10:00:00Z",
    "fetchedAt": "2026-08-01T10:05:00Z",
    "excerpt": "How an explainable local ranker orders the feed.",
    "contentPermission": "metadata_only",
    "language": "en",
    "topics": [
      "technology"
    ],
    "library": {
      "articleId": "article-1",
      "readAt": null,
      "savedAt": null,
      "hiddenAt": null
    },
    "ranking": {
      "score": 0.84,
      "contributions": [],
      "algorithmVersion": "hybrid-v1",
      "calculatedAt": "2026-08-01T10:05:01Z"
    }
  },
  "fullContent": null
}

const fixture6: Models.APIError = {
  "code": "validation_failed",
  "message": "The request contains invalid fields.",
  "correlationId": "request-123",
  "fields": [
    {
      "path": "location.value.country",
      "code": "INVALID_COUNTRY",
      "message": "Use an ISO 3166-1 alpha-2 country code."
    }
  ]
}

const fixture7: Models.CredentialStatus = {
  "configured": true
}

describe('generated API fixture bindings', () => {
  it('compiles every runtime-validated fixture against its generated model', () => {
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
