<script setup lang="ts">
/* global HTMLElement */
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { Profile, RankingConfiguration, Source } from '../../api/generated/models'
import AccessibleField from '../../components/shared/AccessibleField.vue'
import DemographicSignalField from '../../components/shared/DemographicSignalField.vue'
import LiveRegion from '../../components/shared/LiveRegion.vue'
import { navigate } from '../../router/router'
import AppLink from '../../router/AppLink.vue'
import type { UserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { queryKeys } from '../../state/query-keys'
import { ServerStateClient, type QueryState } from '../../state/query-client'
import RankingSignalSettings from '../ranking-settings/RankingSignalSettings.vue'
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
let loadGeneration = 0
let disposed = false
const firstRun = computed(() => profile.value ? isFirstRun(profile.value) : false)
const fieldErrors = computed(() => Object.fromEntries((saveError.value?.fields ?? []).map((field) => [field.path.replace(/^profile\./, ''), field.message])))

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
  const rankingResult = await mutations.updateRanking(rankingWrite(rankingForm.value))
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
  const interestWeight = Number(profileForm.interestWeight)
  if (!Number.isFinite(interestWeight) || interestWeight < 0 || interestWeight > 1) add('interests.weight', 'Interest weight must be from 0 to 1.')
  const hasLocation = Boolean(profileForm.country.trim() || profileForm.region.trim() || profileForm.city.trim())
  if (hasLocation && !/^[A-Za-z]{2}$/.test(profileForm.country.trim())) add('location.value.country', 'Use a two-letter country code.')
  if (hasLocation && !profileForm.region.trim()) add('location.value.region', 'Enter a region when a location is present.')
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
      {{ mode === 'setup' ? 'First-run setup' : 'Profile and ranking' }}
    </h1>
    <p>Everything here stays with the same-origin local API. There is no account, automatic location, or demographic inference.</p>
    <nav
      v-if="mode==='settings'"
      aria-label="Settings sections"
      class="settings-navigation"
    >
      <a href="#preferences-title">Profile preferences</a><a href="#ranking-settings">Ranking signals</a><AppLink to="/sources">
        Sources, credentials, and refresh
      </AppLink>
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
      novalidate
      @submit.prevent="save"
    >
      <p
        v-if="mode === 'setup' && !firstRun"
        class="notice"
      >
        Setup is already complete. Your saved settings are shown below.
      </p>
      <section aria-labelledby="preferences-title">
        <h2 id="preferences-title">
          Your primary preferences
        </h2>
        <AccessibleField
          id="interests"
          label="Interests"
          description="Comma-separated topics, such as technology, local news."
          :error="fieldErrors['interests'] || fieldErrors['interests.name']"
        >
          <template #default="{ describedby }">
            <input
              id="interests"
              v-model="form.interests"
              :aria-describedby="describedby"
              :aria-invalid="Boolean(fieldErrors['interests'])"
            >
          </template>
        </AccessibleField>
        <AccessibleField
          id="interest-weight"
          label="New interest weight"
          description="Applied to newly entered interests; saved interests keep their individual weights. Enter 0 to 1."
          :error="fieldErrors['interests.weight']"
        >
          <template #default="{ describedby }">
            <input
              id="interest-weight"
              v-model="form.interestWeight"
              type="number"
              min="0"
              max="1"
              step="0.05"
              :aria-describedby="describedby"
              :aria-invalid="Boolean(fieldErrors['interests.weight'])"
            >
          </template>
        </AccessibleField>
        <fieldset>
          <legend>Starter sources</legend><p>Choose any starter sources you prefer. Selecting none is allowed.</p>
          <label
            v-for="source in starters"
            :key="source.id"
          ><input
            type="checkbox"
            :checked="form.preferredSourceIds.includes(source.id)"
            @change="toggleSource(source.id, ($event.target as HTMLInputElement).checked)"
          > {{ starterLabel(source) }}</label>
        </fieldset>
      </section>
      <section aria-labelledby="location-title">
        <h2 id="location-title">
          Manual location
        </h2><p>Enter country and region yourself. This app never requests browser or IP location.</p>
        <label><input
          v-model="form.locationEnabled"
          type="checkbox"
        > Use location for ranking</label>
        <AccessibleField
          id="country"
          label="Country code"
          description="Two-letter code, for example HU."
          :error="fieldErrors['location.value.country']"
        >
          <template #default="{ describedby }">
            <input
              id="country"
              v-model="form.country"
              maxlength="2"
              autocomplete="country"
              :aria-describedby="describedby"
              :aria-invalid="Boolean(fieldErrors['location.value.country'])"
            >
          </template>
        </AccessibleField>
        <AccessibleField
          id="region"
          label="Region"
          :error="fieldErrors['location.value.region']"
        >
          <template #default="{ describedby }">
            <input
              id="region"
              v-model="form.region"
              autocomplete="address-level1"
              :aria-describedby="describedby"
              :aria-invalid="Boolean(fieldErrors['location.value.region'])"
            >
          </template>
        </AccessibleField>
        <AccessibleField
          id="city"
          label="City (optional)"
          :error="fieldErrors['location.value.city.value']"
        >
          <template #default="{ describedby }">
            <input
              id="city"
              v-model="form.city"
              autocomplete="address-level2"
              :aria-describedby="describedby"
              :aria-invalid="Boolean(fieldErrors['location.value.city.value'])"
            >
          </template>
        </AccessibleField>
      </section>
      <section aria-labelledby="optional-title">
        <h2 id="optional-title">
          Optional demographic signals
        </h2><p>Values are explicit, local, individually disableable, and subject to the server caps shown below. Disabling keeps an entered value but gives it no ranking effect; clearing removes it on save.</p>
        <DemographicSignalField
          id="age"
          v-model="form.age"
          v-model:enabled="form.ageEnabled"
          label="Age"
          :error="fieldErrors['age.value']"
        />
        <DemographicSignalField
          id="gender"
          v-model="form.gender"
          v-model:enabled="form.genderEnabled"
          label="Gender"
          :error="fieldErrors['gender.value']"
        />
      </section>
      <RankingSignalSettings
        id="ranking-settings"
        v-model="rankingForm"
        :limits="ranking"
        :errors="fieldErrors"
      />
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
      <button
        type="submit"
        :disabled="saving"
      >
        {{ saving ? 'Saving…' : mode === 'setup' && firstRun ? 'Save setup' : 'Save changes' }}
      </button>
      <LiveRegion :message="saveMessage" />
    </form>
  </section>
</template>

<style scoped>
.profile-workflow,.profile-workflow form,.profile-workflow section,.profile-workflow fieldset{display:grid;gap:1rem}.profile-workflow{max-width:58rem}.settings-navigation{display:flex;flex-wrap:wrap;gap:1rem}.profile-workflow section,.profile-workflow fieldset{border:0;padding:1rem 0;border-top:1px solid #c7cbd1}.profile-workflow fieldset label{display:block}.profile-workflow input:not([type=checkbox]){box-sizing:border-box;width:100%;max-width:32rem;padding:.65rem}.notice,.error-summary{padding:1rem;border-inline-start:.3rem solid #8a4b00;background:#fff5e6}.error-summary{border-color:#a40000;background:#fff0f0}button{justify-self:start;padding:.7rem 1rem}
</style>
