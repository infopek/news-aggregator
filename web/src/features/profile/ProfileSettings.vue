<script setup lang="ts">
/* global HTMLElement, document */
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { Profile, RankingConfiguration, Source } from '../../api/generated/models'
import AccessibleField from '../../components/shared/AccessibleField.vue'
import ActionGroup from '../../components/shared/ActionGroup.vue'
import DemographicSignalField from '../../components/shared/DemographicSignalField.vue'
import DisclosurePanel from '../../components/shared/DisclosurePanel.vue'
import LiveRegion from '../../components/shared/LiveRegion.vue'
import StatusBanner from '../../components/shared/StatusBanner.vue'
import SurfaceCard from '../../components/shared/SurfaceCard.vue'
import { navigate } from '../../router/router'
import AppLink from '../../router/AppLink.vue'
import type { UserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { queryKeys } from '../../state/query-keys'
import { ServerStateClient, type QueryState } from '../../state/query-client'
import RankingSignalSettings from '../ranking-settings/RankingSignalSettings.vue'
import InterestChipInput from './InterestChipInput.vue'
import { isFirstRun, profileToForm, profileWrite, rankingToForm, rankingWrite, starterLabel, type ProfileForm, type RankingForm } from './profile-form'

const props = withDefaults(defineProps<{ mode?: 'setup' | 'settings'; serverApi?: ServerApi; serverState?: ServerStateClient; serverMutations?: ServerMutations }>(), { mode: 'settings', serverApi: undefined, serverState: undefined, serverMutations: undefined })
const emit = defineEmits<{ saved: [firstRun: boolean] }>()
const server = props.serverApi ?? createServerApi(client)
const cache = props.serverState ?? new ServerStateClient()
const mutations = props.serverMutations ?? new ServerMutations(server, cache)
const state = ref<'loading' | 'ready' | 'error'>('loading')
const profile = ref<Profile>()
const ranking = ref<RankingConfiguration>()
const starters = ref<Source[]>([])
const form = ref<ProfileForm>()
const rankingForm = ref<RankingForm>()
const loadMessage = ref('')
const saveMessage = ref('')
const saveError = ref<UserSafeError>()
const saving = ref(false)
const partialSave = ref(false)
const errorSummary = ref<HTMLElement>()
const setupStep = ref(1)
const setupSteps = ['Interests', 'Location', 'Sources', 'Finish']
const countries = [
  { code: '', name: 'No country selected' }, { code: 'HU', name: 'Hungary' }, { code: 'AT', name: 'Austria' },
  { code: 'DE', name: 'Germany' }, { code: 'GB', name: 'United Kingdom' }, { code: 'US', name: 'United States' }
]
let loadGeneration = 0
let disposed = false
const firstRun = computed(() => profile.value ? isFirstRun(profile.value) : false)
const fieldErrors = computed(() => Object.fromEntries((saveError.value?.fields ?? []).map((field) => [field.path.replace(/^profile\./, ''), field.message])))
const unknownCountry = computed(() => form.value?.country && !countries.some((item) => item.code === form.value?.country) ? form.value.country : '')
const countryName = computed(() => countries.find((item) => item.code === form.value?.country)?.name || form.value?.country || 'No location')

onMounted(load)
defineExpose({ load })
onBeforeUnmount(() => {
  disposed = true
  loadGeneration += 1
  cache.cancel(queryKeys.profile())
  cache.cancel(queryKeys.ranking())
  cache.cancel(queryKeys.starterSources())
})
async function load() {
  const generation = ++loadGeneration
  state.value = 'loading'; loadMessage.value = ''; saveError.value = undefined; partialSave.value = false
  const [profileState, rankingState, starterState] = await Promise.all([
    cache.query(queryKeys.profile(), (signal) => server.profile(signal)),
    cache.query(queryKeys.ranking(), (signal) => server.ranking(signal)),
    cache.query(queryKeys.starterSources(), (signal) => server.starterSources(signal))
  ])
  if (disposed || generation !== loadGeneration) return
  const failed = [profileState, rankingState, starterState].find((item) => item.status === 'error') as QueryState<unknown> | undefined
  if (failed?.status === 'error') { state.value = 'error'; loadMessage.value = failed.error.message; return }
  if (!profileState.data || !rankingState.data || !starterState.data) { state.value = 'error'; loadMessage.value = 'Saved settings could not be loaded.'; return }
  profile.value = profileState.data; ranking.value = rankingState.data; starters.value = starterState.data.items
  if (props.mode === 'setup' && !isFirstRun(profileState.data)) { navigate('/settings'); return }
  form.value = profileToForm(profileState.data); rankingForm.value = rankingToForm(rankingState.data); state.value = 'ready'
}
async function save() {
  if (!form.value || !rankingForm.value) return
  saving.value = true; saveMessage.value = ''; saveError.value = undefined; partialSave.value = false
  const localFields = validate(form.value, rankingForm.value)
  if (localFields.length) {
    saveError.value = { family: 'validation', message: 'Check the highlighted values.', fields: localFields }
    saving.value = false
    await focusError()
    return
  }
  const wasFirstRun = firstRun.value
  const profileResult = await mutations.updateProfile(profileWrite(form.value))
  if (profileResult.status === 'error') { saveError.value = profileResult.error; saving.value = false; await focusError(); return }
  profile.value = profileResult.data
  form.value = profileToForm(profileResult.data)
  const rankingPayload = rankingWrite(rankingForm.value)
  rankingPayload.location.enabled = form.value.locationEnabled && Boolean(form.value.country || form.value.region || form.value.city)
  rankingPayload.age.enabled = form.value.ageEnabled && Boolean(form.value.age)
  rankingPayload.gender.enabled = form.value.genderEnabled && Boolean(form.value.gender)
  const rankingResult = await mutations.updateRanking(rankingPayload)
  if (rankingResult.status === 'error') {
    partialSave.value = true
    saveError.value = { ...rankingResult.error, message: `${rankingResult.error.message} Your profile was saved, but ranking settings were not. Reload the authoritative settings before retrying.` }
    saving.value = false
    await focusError()
    return
  }
  ranking.value = rankingResult.data; rankingForm.value = rankingToForm(rankingResult.data)
  saveMessage.value = wasFirstRun ? 'Setup saved on this computer.' : 'Profile and ranking settings saved.'
  saving.value = false
  emit('saved', wasFirstRun)
}

async function focusError() {
  await nextTick()
  errorSummary.value?.focus()
}

function validate(profileForm: ProfileForm, weights: RankingForm) {
  const fields: { path: string; code: string; message: string }[] = []
  const add = (path: string, message: string) => fields.push({ path, code: 'INVALID_VALUE', message })
  const hasLocation = Boolean(profileForm.country.trim() || profileForm.region.trim() || profileForm.city.trim())
  if (hasLocation && !/^[A-Za-z]{2}$/.test(profileForm.country.trim())) add('location.value.country', 'Choose a country for this location.')
  if (hasLocation && !profileForm.region.trim() && !profileForm.city.trim()) add('location.value.region', 'Enter a city, area, or administrative region.')
  if (profileForm.age.trim() && (!Number.isInteger(Number(profileForm.age)) || Number(profileForm.age) < 0 || Number(profileForm.age) > 130)) add('age.value', 'Age must be a whole number from 0 to 130.')
  for (const name of Object.keys(weights) as (keyof RankingForm)[]) {
    const value = Number(weights[name].weight)
    if (!Number.isFinite(value) || value < 0 || value > 1) add(`${name}.weight`, `${name} weight must be from 0 to 1.`)
  }
  return fields
}
function toggleSource(id: string, checked: boolean) {
  if (!form.value) return
  form.value.preferredSourceIds = checked ? [...form.value.preferredSourceIds, id] : form.value.preferredSourceIds.filter((value) => value !== id)
}
function goToStep(step: number) { setupStep.value = Math.max(1, Math.min(4, step)); nextTick(() => document.querySelector<HTMLElement>('#profile-title')?.focus()) }
</script>

<template>
  <section
    class="profile-workflow"
    aria-labelledby="profile-title"
  >
    <p class="eyebrow">
      Local personalization
    </p>
    <h1
      id="profile-title"
      tabindex="-1"
    >
      {{ mode === 'setup' ? 'Make your feed yours' : 'Settings' }}
    </h1>
    <p class="page-intro">
      {{ mode === 'setup' ? 'A few choices help order your news. You can change everything later.' : 'Manage personalization, ranking, and local privacy choices.' }}
    </p>
    <nav
      v-if="mode==='settings'"
      aria-label="Settings sections"
      class="settings-navigation"
    >
      <a href="#personalization-title">Personalization</a><a href="#ranking-settings-title">Ranking</a><a href="#privacy-title">Privacy &amp; local data</a>
    </nav>
    <p
      v-if="state === 'loading'"
      role="status"
    >
      Loading saved settings…
    </p>
    <div
      v-else-if="state === 'error'"
      role="alert"
    >
      <p>{{ loadMessage }}</p><button
        type="button"
        @click="load"
      >
        Try again
      </button>
    </div>
    <form
      v-else-if="form && ranking && rankingForm"
      class="settings-form"
      novalidate
      @submit.prevent="save"
    >
      <ol
        v-if="mode === 'setup'"
        class="stepper"
        aria-label="Setup progress"
      >
        <li
          v-for="(label,index) in setupSteps"
          :key="label"
          :class="{ current: setupStep === index + 1, complete: setupStep > index + 1 }"
          :aria-current="setupStep === index + 1 ? 'step' : undefined"
        >
          <span>{{ index + 1 }}</span>{{ label }}
        </li>
      </ol>

      <SurfaceCard
        v-if="mode === 'settings' || setupStep === 1"
        :title="mode === 'setup' ? 'What are you interested in?' : 'Personalization'"
        :description="mode === 'setup' ? 'Add a few topics to shape your feed. There are no numeric weights to manage.' : 'Choose the topics and place that matter to you.'"
      >
        <h2
          v-if="mode === 'settings'"
          id="personalization-title"
          class="section-anchor"
        >
          Topics you care about
        </h2>
        <InterestChipInput
          v-model="form.interests"
          :error="fieldErrors['interests'] || fieldErrors['interests.name']"
        />
      </SurfaceCard>

      <SurfaceCard
        v-if="mode === 'settings' || setupStep === 2"
        title="Location — optional"
        description="Used only to prioritize relevant local stories. Your location is entered manually and stays on this computer."
      >
        <label class="choice-toggle"><input
          v-model="form.locationEnabled"
          type="checkbox"
        > Use my location when ranking stories</label>
        <div class="field-grid">
          <AccessibleField
            id="country"
            label="Country"
            description="Choose the country you care about for local reporting."
            :error="fieldErrors['location.value.country']"
          >
            <template #default="{ describedby }">
              <select
                id="country"
                v-model="form.country"
                autocomplete="country"
                :aria-describedby="describedby"
                :aria-invalid="Boolean(fieldErrors['location.value.country'])"
              >
                <option
                  v-if="unknownCountry"
                  :value="unknownCountry"
                >
                  Other ({{ unknownCountry }})
                </option><option
                  v-for="country in countries"
                  :key="country.code"
                  :value="country.code"
                >
                  {{ country.name }}
                </option>
              </select>
            </template>
          </AccessibleField>
          <AccessibleField
            id="city"
            label="City or area"
            description="For example, Budapest. You do not need to know an administrative region."
            :error="fieldErrors['location.value.city.value'] || fieldErrors['location.value.region']"
          >
            <template #default="{ describedby }">
              <input
                id="city"
                v-model="form.city"
                placeholder="Budapest"
                autocomplete="address-level2"
                :aria-describedby="describedby"
                :aria-invalid="Boolean(fieldErrors['location.value.city.value'] || fieldErrors['location.value.region'])"
              >
            </template>
          </AccessibleField>
        </div>
        <DisclosurePanel summary="Administrative region — optional advanced detail">
          <AccessibleField
            id="region"
            label="Region"
            description="Only add this when it differs from your city or area, or your local news sources use it."
            :error="fieldErrors['location.value.region']"
          >
            <template #default="{ describedby }">
              <input
                id="region"
                v-model="form.region"
                placeholder="Pest"
                autocomplete="address-level1"
                :aria-describedby="describedby"
                :aria-invalid="Boolean(fieldErrors['location.value.region'])"
              >
            </template>
          </AccessibleField>
        </DisclosurePanel>
        <DisclosurePanel summary="Additional personalization">
          <p class="field__description">
            These optional values can only help when a publisher explicitly labels an article for that audience. They are never inferred.
          </p>
          <div class="optional-grid">
            <div>
              <p><strong>Age — optional</strong></p><p class="field__description">
                Can slightly boost articles explicitly identified as relevant to your age group.
              </p><DemographicSignalField
                id="age"
                v-model="form.age"
                v-model:enabled="form.ageEnabled"
                label="Age"
                :error="fieldErrors['age.value']"
              />
            </div>
            <div>
              <p><strong>Gender — optional</strong></p><p class="field__description">
                Can slightly boost articles explicitly identified as relevant to that audience.
              </p><DemographicSignalField
                id="gender"
                v-model="form.gender"
                v-model:enabled="form.genderEnabled"
                label="Gender"
                :error="fieldErrors['gender.value']"
              />
            </div>
          </div>
        </DisclosurePanel>
      </SurfaceCard>

      <SurfaceCard
        v-if="mode === 'setup' && setupStep === 3"
        title="Choose starter sources"
        description="Start with any of these reviewed sources. Selecting none is fine; you can add sources later."
      >
        <fieldset class="source-choices">
          <legend class="sr-only">
            Starter sources
          </legend><label
            v-for="source in starters"
            :key="source.id"
            class="source-choice"
          ><input
            type="checkbox"
            :checked="form.preferredSourceIds.includes(source.id)"
            @change="toggleSource(source.id,($event.target as HTMLInputElement).checked)"
          ><span><strong>{{ source.name }}</strong><small>{{ starterLabel(source) }}</small></span></label>
        </fieldset>
      </SurfaceCard>

      <SurfaceCard
        v-if="mode === 'settings'"
        title="Ranking"
        description="Choose an understandable ranking style. Precise controls are available only when you want them."
      >
        <RankingSignalSettings
          id="ranking-settings"
          v-model="rankingForm"
          :limits="ranking"
          :errors="fieldErrors"
        />
      </SurfaceCard>

      <SurfaceCard
        v-if="mode === 'setup' && setupStep === 4"
        title="Ready to begin"
        description="Review the essentials, choose a ranking style, then finish setup."
      >
        <ul class="setup-summary">
          <li><strong>{{ form.interests.length }}</strong> topics selected</li><li><strong>{{ countryName }}</strong><span v-if="form.city"> · {{ form.city }}</span></li><li><strong>{{ form.preferredSourceIds.length }}</strong> starter sources selected</li>
        </ul>
        <RankingSignalSettings
          id="ranking-settings"
          v-model="rankingForm"
          :limits="ranking"
          :errors="fieldErrors"
        />
        <StatusBanner
          title="Private by design"
          tone="success"
        >
          <p>No account, automatic location, demographic inference, tracking, or cloud profile. These choices stay in the local app.</p>
        </StatusBanner>
      </SurfaceCard>

      <SurfaceCard
        v-if="mode === 'settings'"
        title="Privacy & local data"
        description="Your profile, ranking preferences, filters, and sources are authoritative in the local app database."
      >
        <h2
          id="privacy-title"
          class="section-anchor"
        >
          Local-only behavior
        </h2><p>No account, cloud sync, automatic location, tracking, or external profile processing is used.</p><p>Credentials remain write-only behind the operating system credential store.</p><AppLink to="/sources">
          Manage sources and credentials
        </AppLink>
      </SurfaceCard>
      <div
        v-if="saveError"
        ref="errorSummary"
        class="error-summary"
        role="alert"
        tabindex="-1"
      >
        <strong>{{ partialSave ? 'Profile saved; ranking settings not saved.' : 'Settings were not saved.' }}</strong><p>{{ saveError.message }}</p><ul v-if="saveError.fields.length">
          <li
            v-for="item in saveError.fields"
            :key="`${item.path}-${item.code}`"
          >
            {{ item.message }}
          </li>
        </ul>
        <button
          v-if="partialSave"
          type="button"
          @click="load"
        >
          Reload saved settings
        </button>
      </div>
      <ActionGroup
        v-if="mode === 'setup'"
        label="Setup navigation"
      >
        <button
          v-if="setupStep > 1"
          type="button"
          class="secondary"
          @click="goToStep(setupStep - 1)"
        >
          Back
        </button>
        <button
          v-if="setupStep < 4"
          type="button"
          @click="goToStep(setupStep + 1)"
        >
          Continue
        </button>
        <button
          v-else
          type="submit"
          :disabled="saving"
        >
          {{ saving ? 'Saving…' : 'Finish setup' }}
        </button>
      </ActionGroup>
      <button
        v-else
        type="submit"
        :disabled="saving"
      >
        {{ saving ? 'Saving…' : 'Save changes' }}
      </button>
      <LiveRegion :message="saveMessage" />
    </form>
  </section>
</template>

<style scoped>
.profile-workflow,.settings-form{display:grid;gap:var(--space-5)}.profile-workflow{max-width:var(--content-form)}.page-intro{max-width:42rem;color:var(--color-muted);font-size:1.08rem}.settings-navigation{display:flex;flex-wrap:wrap;gap:var(--space-2);padding:var(--space-2);border:1px solid var(--color-border);border-radius:var(--radius-md);background:var(--color-surface-soft)}.settings-navigation a{min-height:2.75rem;display:inline-flex;align-items:center;padding:var(--space-2) var(--space-3);border-radius:var(--radius-sm);font-weight:680;text-decoration:none}.section-anchor{font-size:1.05rem}.stepper{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--space-2);margin:0;padding:0;list-style:none}.stepper li{display:flex;align-items:center;gap:var(--space-2);color:var(--color-muted);font-size:.88rem;font-weight:680}.stepper span{width:1.8rem;height:1.8rem;display:grid;place-items:center;border:1px solid var(--color-border-strong);border-radius:50%;background:var(--color-surface)}.stepper .current{color:var(--color-brand)}.stepper .current span,.stepper .complete span{border-color:var(--color-brand);background:var(--color-brand);color:#fff}.choice-toggle{display:flex;align-items:center;gap:var(--space-2);font-weight:700}.field-grid,.optional-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-4)}.source-choices{display:grid;gap:var(--space-3);padding:0;border:0}.source-choice{display:flex;align-items:flex-start;gap:var(--space-3);padding:var(--space-4);border:1px solid var(--color-border);border-radius:var(--radius-md);cursor:pointer}.source-choice:hover{border-color:var(--color-brand);background:var(--color-brand-soft)}.source-choice span{display:grid;gap:var(--space-1)}.source-choice small{color:var(--color-muted)}.setup-summary{display:grid;gap:var(--space-2);margin:0;padding:var(--space-4);border-radius:var(--radius-md);background:var(--color-surface-soft);list-style:none}.error-summary{display:grid;gap:var(--space-2);padding:var(--space-4);border-inline-start:.3rem solid var(--color-danger);border-radius:var(--radius-md);background:var(--color-danger-soft)}@media(max-width:42rem){.stepper{grid-template-columns:repeat(2,minmax(0,1fr));row-gap:var(--space-3)}.field-grid,.optional-grid{grid-template-columns:1fr}.settings-navigation{display:grid;grid-template-columns:1fr}}
</style>
