<script setup lang="ts">
/* global AbortController, URL */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { Source } from '../../api/generated/models'
import ConfirmAction from '../../components/shared/ConfirmAction.vue'
import CredentialInput from '../../components/shared/CredentialInput.vue'
import LiveRegion from '../../components/shared/LiveRegion.vue'
import PermissionBadge from '../../components/shared/PermissionBadge.vue'
import RefreshControl from '../refresh/RefreshControl.vue'
import { toUserSafeError } from '../../state/errors'
import { emptySource, sourceForm, sourceWrite, validateSource, type SourceForm } from './source-form'

const props = withDefaults(defineProps<{ serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client)
const sources = ref<Source[]>([]), starters = ref<Source[]>([])
const state = ref<'loading'|'ready'|'error'>('loading'), message = ref(''), error = ref('')
const form = ref<SourceForm>(emptySource()), editing = ref(false), busy = ref(false), credentialFor = ref('')
const validation = ref<string[]>([])
const configuredStarterURLs = computed(() => new Set(sources.value.map((item) => normalizedURL(item.url))))
const controller = new AbortController()
onMounted(load)
onBeforeUnmount(() => controller.abort())

async function load() {
  state.value = 'loading'; error.value = ''
  try {
    const [saved, available] = await Promise.all([server.sources(controller.signal), server.starterSources(controller.signal)])
    sources.value = saved.items; starters.value = available.items; state.value = 'ready'
  } catch (cause) { state.value = 'error'; error.value = toUserSafeError(cause).message }
}
function edit(source?: Source) { form.value = source ? sourceForm(source) : emptySource(); editing.value = true; validation.value = []; message.value = '' }
function cancel() { editing.value = false; form.value = emptySource(); validation.value = [] }
async function save() {
  validation.value = validateSource(form.value)
  if (validation.value.length) return
  busy.value = true; error.value = ''
  try {
    const value = form.value.id ? await server.updateSource(form.value.id, sourceWrite(form.value)) : await server.createSource(sourceWrite(form.value))
    const index = sources.value.findIndex((item) => item.id === value.id)
    if (index < 0) sources.value.push(value); else sources.value[index] = value
    message.value = `${value.name} saved.`; cancel()
  } catch (cause) { error.value = toUserSafeError(cause).message } finally { busy.value = false }
}
async function addStarter(source: Source) {
  form.value = sourceForm(source); form.value.id = undefined; await save()
}
async function remove(source: Source) {
  try { await server.deleteSource(source.id); sources.value = sources.value.filter((item) => item.id !== source.id); message.value = `${source.name} deleted.` } catch (cause) { error.value = toUserSafeError(cause).message }
}
async function credential(source: Source, secret: string) {
  try { const status = await server.writeCredential(source.id, { secret }); replaceCredentialStatus(source.id, status.configured); credentialFor.value = ''; message.value = `Credential for ${source.name} saved and cleared from the form.` } catch { error.value = 'The credential could not be saved. Try again.' }
}
async function deleteCredential(source: Source) {
  try { const status = await server.deleteCredential(source.id); replaceCredentialStatus(source.id, status.configured); message.value = `Credential for ${source.name} deleted.` } catch { error.value = 'The credential could not be deleted. Try again.' }
}
function replaceCredentialStatus(id: string, configured: boolean) { sources.value = sources.value.map((item) => item.id === id ? { ...item, credentialConfigured: configured } : item) }
function normalizedURL(value: string) { try { const url = new URL(value); url.hash = ''; return url.toString().replace(/\/$/, '') } catch { return value.trim().replace(/\/$/, '') } }
</script>

<template>
  <section
    class="sources-workflow"
    aria-labelledby="sources-title"
  >
    <p class="eyebrow">
      Local ingestion
    </p><h1
      id="sources-title"
      tabindex="-1"
    >
      Sources and refresh
    </h1>
    <p>Manage public feeds, official APIs, and explicitly approved scrapers. Credentials are write-only and never displayed.</p>
    <p
      v-if="state === 'loading'"
      role="status"
    >
      Loading sources…
    </p>
    <div
      v-else-if="state === 'error'"
      role="alert"
    >
      <p>{{ error }}</p><button
        type="button"
        @click="load"
      >
        Try again
      </button>
    </div>
    <template v-else>
      <LiveRegion :message="message" /><p
        v-if="error"
        role="alert"
      >
        {{ error }}
      </p>
      <section aria-labelledby="configured-title">
        <h2 id="configured-title">
          Configured sources
        </h2>
        <p v-if="!sources.length">
          No sources are configured. Add a starter or custom source before refreshing.
        </p>
        <article
          v-for="source in sources"
          :key="source.id"
          class="source-card"
        >
          <h3>{{ source.name }}</h3><p><strong>{{ source.kind.toUpperCase() }}</strong> · {{ source.enabled ? 'Enabled' : 'Disabled' }} · <PermissionBadge :permission="source.contentPermission" /></p>
          <p>
            <a
              :href="source.url"
              rel="noreferrer"
            >{{ source.url }}</a>
          </p>
          <p v-if="source.kind === 'scraper'">
            Policy: <strong>{{ source.scraperPolicy.status }}</strong><template v-if="source.scraperPolicy.reviewedAt">
              · reviewed {{ new Date(source.scraperPolicy.reviewedAt).toLocaleDateString() }}
            </template>
          </p>
          <p>Credential: <strong>{{ source.credentialConfigured ? 'Configured' : 'Not configured' }}</strong></p>
          <p v-if="source.retryAfter">
            Rate limited; retry after {{ new Date(source.retryAfter).toLocaleString() }}.
          </p><p v-else-if="source.lastError">
            Last refresh: {{ source.lastError }}
          </p>
          <div class="actions">
            <button
              type="button"
              @click="edit(source)"
            >
              Edit
            </button><button
              type="button"
              @click="credentialFor = credentialFor === source.id ? '' : source.id"
            >
              {{ source.credentialConfigured ? 'Replace credential' : 'Configure credential' }}
            </button><button
              v-if="source.credentialConfigured"
              type="button"
              @click="deleteCredential(source)"
            >
              Delete credential
            </button><ConfirmAction
              label="Delete source"
              confirm-label="Delete source"
              @confirm="remove(source)"
            />
          </div>
          <CredentialInput
            v-if="credentialFor === source.id"
            :id="`credential-${source.id}`"
            @submit="credential(source, $event)"
            @cancel="credentialFor = ''"
          />
        </article>
      </section>
      <section aria-labelledby="starters-title">
        <h2 id="starters-title">
          Starter catalog
        </h2><ul>
          <li
            v-for="starter in starters"
            :key="starter.id"
          >
            {{ starter.name }} ({{ starter.kind }}) <button
              type="button"
              :disabled="configuredStarterURLs.has(normalizedURL(starter.url))"
              @click="addStarter(starter)"
            >
              {{ configuredStarterURLs.has(normalizedURL(starter.url)) ? 'Configured' : 'Add starter' }}
            </button>
          </li>
        </ul>
      </section>
      <button
        v-if="!editing"
        type="button"
        @click="edit()"
      >
        Add custom source
      </button>
      <form
        v-else
        class="source-form"
        novalidate
        @submit.prevent="save"
      >
        <h2>{{ form.id ? 'Edit source' : 'Add custom source' }}</h2><div
          v-if="validation.length"
          role="alert"
        >
          <strong>Source was not saved.</strong><ul>
            <li
              v-for="item in validation"
              :key="item"
            >
              {{ item }}
            </li>
          </ul>
        </div>
        <label>Name <input
          v-model="form.name"
          required
        ></label><label>URL <input
          v-model="form.url"
          type="url"
          required
        ></label>
        <label>Type <select
          v-model="form.kind"
          :disabled="Boolean(form.id)"
        ><option value="feed">RSS/Atom feed</option><option value="api">Official API</option><option value="scraper">Approved scraper</option></select></label>
        <label><input
          v-model="form.enabled"
          type="checkbox"
        > Enabled</label><label>Content permission <select v-model="form.contentPermission"><option value="metadata_only">Metadata only</option><option value="full_content_allowed">Full content allowed</option></select></label>
        <label v-if="form.kind === 'feed'">Feed format <select v-model="form.feedFormat"><option value="auto">Auto-detect</option><option value="rss">RSS</option><option value="atom">Atom</option></select></label>
        <template v-if="form.kind === 'api'">
          <label>Provider identifier <input v-model="form.apiProvider"></label><label>Page size <input
            v-model="form.apiPageSize"
            type="number"
            min="0"
            step="1"
          ></label>
        </template>
        <fieldset v-if="form.kind === 'scraper'">
          <legend>Scraper configuration and policy</legend><label>Article selector <input v-model="form.articleSelector"></label><label>Title selector <input v-model="form.titleSelector"></label><label>Content selector <input v-model="form.contentSelector"></label><label>Policy status <select v-model="form.policyStatus"><option value="pending">Pending</option><option value="approved">Approved</option><option value="rejected">Rejected</option></select></label><label>Terms URL <input
            v-model="form.termsUrl"
            type="url"
          ></label><label>Robots URL <input
            v-model="form.robotsUrl"
            type="url"
          ></label><label>Reviewed at <input
            v-model="form.reviewedAt"
            type="datetime-local"
          ></label><label>Review notes <textarea v-model="form.reviewNotes" /></label>
        </fieldset>
        <div class="actions">
          <button
            type="submit"
            :disabled="busy"
          >
            {{ busy ? 'Saving…' : 'Save source' }}
          </button><button
            type="button"
            @click="cancel"
          >
            Cancel
          </button>
        </div>
      </form>
      <RefreshControl :server="server" />
    </template>
  </section>
</template>

<style scoped>
.source-card,.source-form{border:1px solid var(--border,#777);border-radius:.5rem;padding:1rem;margin-block:1rem}.actions{display:flex;flex-wrap:wrap;gap:.5rem}.source-form label{display:block;margin-block:.75rem}.source-form input:not([type=checkbox]),.source-form select,.source-form textarea{display:block;width:min(100%,42rem)}
</style>
