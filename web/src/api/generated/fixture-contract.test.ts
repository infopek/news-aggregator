// Generated from api/openapi.yaml (dd8be64ea54d6638). Do not edit.
import { describe, expect, it } from 'vitest'
import type * as Models from './models'

const fixture0: Models.Health = {
  "status": "ready",
  "version": "0.1.0"
}

const fixture1: Models.Profile = {
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

const fixture2: Models.Profile = {
  "id": "local-profile",
  "interests": [],
  "preferredSourceIds": [],
  "location": {
    "present": false,
    "enabled": false
  },
  "age": {
    "present": false,
    "enabled": false
  },
  "gender": {
    "present": false,
    "enabled": false
  },
  "updatedAt": "2026-08-01T12:00:00Z"
}

const fixture3: Models.RankingConfiguration = {
  "recency": {
    "enabled": true,
    "weight": 0.25
  },
  "interest": {
    "enabled": true,
    "weight": 0.25
  },
  "sourcePreference": {
    "enabled": true,
    "weight": 0.1
  },
  "behavior": {
    "enabled": true,
    "weight": 0.1
  },
  "location": {
    "enabled": false,
    "weight": 0.05
  },
  "age": {
    "enabled": false,
    "weight": 0.05
  },
  "gender": {
    "enabled": false,
    "weight": 0.05
  },
  "textSimilarity": {
    "enabled": true,
    "weight": 0.15
  },
  "perDemographicCap": 0.1,
  "totalDemographicCap": 0.2,
  "normalizationVersion": "v1"
}

const fixture4: Models.Source = {
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

const fixture5: Models.SourceList = {
  "items": [
    {
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
    },
    {
      "id": "source-2",
      "name": "Fictional Official API",
      "url": "https://api.example.test/news",
      "kind": "api",
      "enabled": true,
      "contentPermission": "metadata_only",
      "adapterConfig": {
        "provider": "fictional-official-api",
        "pageSize": 20
      },
      "scraperPolicy": {
        "status": "not_applicable",
        "termsUrl": null,
        "robotsUrl": null,
        "reviewedAt": null,
        "reviewNotes": null
      },
      "credentialConfigured": false,
      "lastSuccessAt": null,
      "lastError": null,
      "retryAfter": null
    },
    {
      "id": "source-3",
      "name": "Approved Fictional Scraper",
      "url": "https://scrape.example.test/news",
      "kind": "scraper",
      "enabled": true,
      "contentPermission": "metadata_only",
      "adapterConfig": {
        "articleSelector": "article",
        "titleSelector": "h1",
        "excerptSelector": "p.excerpt",
        "contentSelector": ""
      },
      "scraperPolicy": {
        "status": "approved",
        "termsUrl": "https://scrape.example.test/terms",
        "robotsUrl": "https://scrape.example.test/robots.txt",
        "reviewedAt": "2026-08-01T09:00:00Z",
        "reviewNotes": "Fixture-only approval."
      },
      "credentialConfigured": false,
      "lastSuccessAt": null,
      "lastError": null,
      "retryAfter": null
    }
  ]
}

const fixture6: Models.RefreshRun = {
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

const fixture7: Models.FeedPage = {
  "items": [],
  "nextCursor": null
}

const fixture8: Models.FeedPage = {
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

const fixture9: Models.ArticleDetail = {
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

const fixture10: Models.ArticleDetail = {
  "article": {
    "id": "article-2",
    "sourceId": "source-3",
    "canonicalUrl": "https://public-domain.example/articles/allowed",
    "title": "Permitted fixture article",
    "publishedAt": null,
    "fetchedAt": "2026-08-01T10:05:00Z",
    "contentPermission": "full_content_allowed",
    "topics": [],
    "library": {
      "articleId": "article-2",
      "readAt": null,
      "savedAt": null,
      "hiddenAt": null
    },
    "ranking": {
      "score": 0.2,
      "contributions": [
        {
          "signal": "recency",
          "rawScore": 0.4,
          "weight": 0.5,
          "weightedScore": 0.2,
          "reasonCode": "FUTURE_REASON_CODE",
          "reasonValues": {}
        }
      ],
      "algorithmVersion": "hybrid-v1",
      "calculatedAt": "2026-08-01T10:05:01Z"
    }
  },
  "fullContent": "Fictional content released for this contract fixture."
}

const fixture11: Models.LibraryState = {
  "articleId": "article-1",
  "readAt": "2026-08-01T13:00:00Z",
  "savedAt": null,
  "hiddenAt": null
}

const fixture12: Models.APIError = {
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

const fixture13: Models.CredentialStatus = {
  "configured": true
}

describe('generated API fixture bindings', () => {
  it('compiles every runtime-validated fixture against its generated model', () => {
    expect(fixture0).toBeTruthy() // health.json -> Health
    expect(fixture1).toBeTruthy() // profile.json -> Profile
    expect(fixture2).toBeTruthy() // profile-optional-absent.json -> Profile
    expect(fixture3).toBeTruthy() // ranking-configuration.json -> RankingConfiguration
    expect(fixture4).toBeTruthy() // source.json -> Source
    expect(fixture5).toBeTruthy() // source-list.json -> SourceList
    expect(fixture6).toBeTruthy() // refresh-partial.json -> RefreshRun
    expect(fixture7).toBeTruthy() // feed-empty.json -> FeedPage
    expect(fixture8).toBeTruthy() // feed-page.json -> FeedPage
    expect(fixture9).toBeTruthy() // article-metadata.json -> ArticleDetail
    expect(fixture10).toBeTruthy() // article-full-content.json -> ArticleDetail
    expect(fixture11).toBeTruthy() // library-state.json -> LibraryState
    expect(fixture12).toBeTruthy() // validation-error.json -> APIError
    expect(fixture13).toBeTruthy() // credential-status.json -> CredentialStatus
  })
})
