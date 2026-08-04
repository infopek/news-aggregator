import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const sources = [
  '../src/features/profile/ProfileSettings.vue', '../src/features/profile/profile-form.ts',
  '../src/features/setup/FirstRunSetup.vue', '../src/features/ranking-settings/RankingSignalSettings.vue'
].map((path) => readFileSync(new URL(path, import.meta.url), 'utf8')).join('\n')

describe('profile privacy boundaries', () => {
  it('has no geolocation, IP lookup, external fetch, storage, account, inference, or client ranking implementation', () => {
    expect(sources).not.toMatch(/navigator\.geolocation|ipify|ipinfo|localStorage|sessionStorage|indexedDB|document\.cookie|https?:\/\/|createAccount|signIn|infer(?:Age|Gender)|calculateScore|weightedScore/)
    expect(sources).toContain('createServerApi(client)')
  })
})
