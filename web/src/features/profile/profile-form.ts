import type { Profile, ProfileWrite, RankingConfiguration, RankingConfigurationWrite, Source } from '../../api/generated/models'

export const signalNames = ['recency', 'interest', 'sourcePreference', 'behavior', 'location', 'age', 'gender', 'textSimilarity'] as const
export type SignalName = typeof signalNames[number]

export interface ProfileForm {
  interests: string[]
  interestDraft: string
  interestWeights: Record<string, number>
  preferredSourceIds: string[]
  locationEnabled: boolean
  country: string
  region: string
  city: string
  ageEnabled: boolean
  age: string
  genderEnabled: boolean
  gender: string
}

export type RankingForm = Record<SignalName, { enabled: boolean; weight: string }>
export type RankingPreset = 'balanced' | 'personalized' | 'recent' | 'custom'

export const defaultInterestWeight = 0.8
export const rankingPresets: Record<Exclude<RankingPreset, 'custom'>, Record<SignalName, number>> = {
  balanced: { recency: .25, interest: .25, sourcePreference: .1, behavior: .1, location: .05, age: .05, gender: .05, textSimilarity: .15 },
  personalized: { recency: .15, interest: .35, sourcePreference: .15, behavior: .1, location: .05, age: .05, gender: .05, textSimilarity: .1 },
  recent: { recency: .45, interest: .2, sourcePreference: .08, behavior: .07, location: .03, age: .03, gender: .03, textSimilarity: .11 }
}

export function profileToForm(profile: Profile): ProfileForm {
  const location = profile.location.value
  return {
    interests: profile.interests.map((item) => item.name),
    interestDraft: '',
    interestWeights: Object.fromEntries(profile.interests.map((item) => [item.name, item.weight])),
    preferredSourceIds: [...profile.preferredSourceIds],
    locationEnabled: profile.location.enabled,
    country: location?.country ?? '', region: location?.region ?? '', city: location?.city.value ?? '',
    ageEnabled: profile.age.enabled, age: profile.age.value == null ? '' : String(profile.age.value),
    genderEnabled: profile.gender.enabled, gender: profile.gender.value ?? ''
  }
}

export function rankingToForm(ranking: RankingConfiguration): RankingForm {
  return Object.fromEntries(signalNames.map((name) => [name, { enabled: ranking[name].enabled, weight: String(ranking[name].weight) }])) as RankingForm
}

export function profileWrite(form: ProfileForm): ProfileWrite {
  const names = form.interests.map((value) => value.trim()).filter(Boolean)
  const hasLocation = Boolean(form.country.trim() || form.region.trim() || form.city.trim())
  const city = form.city.trim()
  const age = form.age.trim()
  const gender = form.gender.trim()
  return {
    interests: names.map((name) => ({ name, weight: form.interestWeights[name] ?? defaultInterestWeight })),
    preferredSourceIds: [...new Set(form.preferredSourceIds)],
    location: hasLocation ? { present: true, enabled: form.locationEnabled, value: {
      country: form.country.trim().toUpperCase(), region: form.region.trim() || city,
      city: city ? { present: true, enabled: true, value: city } : { present: false, enabled: false }
    } } : { present: false, enabled: false },
    age: age ? { present: true, enabled: form.ageEnabled, value: Number(age) } : { present: false, enabled: false },
    gender: gender ? { present: true, enabled: form.genderEnabled, value: gender } : { present: false, enabled: false }
  }
}

export function detectRankingPreset(form: RankingForm): RankingPreset {
  for (const [name, weights] of Object.entries(rankingPresets) as [Exclude<RankingPreset, 'custom'>, Record<SignalName, number>][]) {
    if (signalNames.every((signal) => Number(form[signal].weight) === weights[signal])) return name
  }
  return 'custom'
}

export function applyRankingPreset(form: RankingForm, preset: Exclude<RankingPreset, 'custom'>): RankingForm {
  return Object.fromEntries(signalNames.map((name) => [name, {
    enabled: form[name].enabled,
    weight: String(rankingPresets[preset][name])
  }])) as RankingForm
}

export function rankingWrite(form: RankingForm): RankingConfigurationWrite {
  return Object.fromEntries(signalNames.map((name) => [name, { enabled: form[name].enabled, weight: Number(form[name].weight) }])) as RankingConfigurationWrite
}

export function isFirstRun(profile: Profile): boolean {
  return profile.interests.length === 0 && profile.preferredSourceIds.length === 0 && !profile.location.present
}

export function starterLabel(source: Source): string {
  return `${source.kind === 'feed' ? 'News feed' : source.kind === 'api' ? 'Official API' : 'Approved website'} · ${source.contentPermission === 'metadata_only' ? 'Opens stories at the publisher' : 'Full content allowed'}`
}
