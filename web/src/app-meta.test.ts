import { describe, expect, it } from 'vitest'

import { appName } from './app-meta'

describe('application metadata', () => {
  it('has the accepted product name', () => {
    expect(appName).toBe('News Aggregator')
  })
})
