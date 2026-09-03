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
const sourceNames = computed(() => Object.fromEntries(sources.value.map((item) => [item.id, item.name])))
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
function kindLabel(kind: Source['kind']) { return kind === 'feed' ? 'News feed' : kind === 'api' ? 'Official API' : 'Approved website' }
</script>

<template>
  <section
    class="sources-workflow"
    aria-labelledby="sources-title"
  >
    <header class="sources-header">
      <p class="eyebrow">
        Local ingestion
      </p><h1
        id="sources-title"
        tabindex="-1"
      >
        Sources and refresh
      </h1>
      <p>Choose where your local feed gets its stories. Credentials stay write-only and are never displayed.</p>
    </header>
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
      <section
        class="source-section"
        aria-labelledby="configured-title"
      >
        <h2 id="configured-title">
          Configured sources
        </h2>
        <p class="section-description">
          Sources currently checked when you refresh.
        </p><div
          v-if="!sources.length"
          class="source-empty"
        >
          <strong>No sources yet</strong><p>Add a starter below or connect your own public feed.</p>
        </div>
        <article
          v-for="source in sources"
          :key="source.id"
          class="source-card"
        >
          <header class="source-card__header">
            <div><h3>{{ source.name }}</h3><p>{{ kindLabel(source.kind) }}</p></div><strong :class="['source-state',{'source-state--disabled':!source.enabled}]">{{ source.enabled ? 'Enabled' : 'Disabled' }}</strong>
          </header><div class="source-card__badges">
            <PermissionBadge :permission="source.contentPermission" /><span class="badge">Credential {{ source.credentialConfigured ? 'ready' : 'not configured' }}</span>
          </div>
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
          <p v-if="source.retryAfter">
            Rate limited; retry after {{ new Date(source.retryAfter).toLocaleString() }}.
          </p><p v-else-if="source.lastError">
            Last refresh: {{ source.lastError }}
          </p>
          <div class="source-actions">
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
      <section
        class="source-section starter-section"
        aria-labelledby="starters-title"
      >
        <h2 id="starters-title">
          Starter catalog
        </h2><p class="section-description">
          Quick, pre-filled choices you can add with one click.
        </p><ul class="starter-grid">
          <li
            v-for="starter in starters"
            :key="starter.id"
          >
            <div><strong>{{ starter.name }}</strong><span>{{ kindLabel(starter.kind) }}</span></div><button
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
        class="add-source"
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
        <header><h2>{{ form.id ? 'Edit source' : 'Add custom source' }}</h2><p>Connect a public feed or explicitly reviewed source. Required details depend on its type.</p></header><div
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
        <label>Name <span class="field-help">A short name you will recognize in filters and refresh results.</span><input
          v-model="form.name"
          required
          placeholder="For example, Local technology news"
        ></label><label>Source address <span class="field-help">Paste the public feed, API, or reviewed website URL.</span><input
          v-model="form.url"
          type="url"
          required
          placeholder="https://example.com/feed.xml"
        ></label>
        <label>Source type <span class="field-help">Most publishers offer an RSS or Atom news feed.</span><select
          v-model="form.kind"
          :disabled="Boolean(form.id)"
        ><option value="feed">RSS/Atom feed</option><option value="api">Official API</option><option value="scraper">Approved scraper</option></select></label>
        <label><input
          v-model="form.enabled"
          type="checkbox"
        > Include this source when refreshing</label><label>Article access <span class="field-help">Choose full content only when the publisher explicitly permits local storage.</span><select v-model="form.contentPermission"><option value="metadata_only">Headlines and publisher links only</option><option value="full_content_allowed">Full articles may be stored</option></select></label>
        <label v-if="form.kind === 'feed'">Feed format <span class="field-help">Auto-detect is recommended unless the publisher specifies a format.</span><select v-model="form.feedFormat"><option value="auto">Auto-detect (recommended)</option><option value="rss">RSS</option><option value="atom">Atom</option></select></label>
        <template v-if="form.kind === 'api'">
          <label>API provider name <span class="field-help">Use the provider identifier from its documentation.</span><input
            v-model="form.apiProvider"
            placeholder="provider-name"
          ></label><label>Stories per request <span class="field-help">50 is a sensible default.</span><input
            v-model="form.apiPageSize"
            type="number"
            min="0"
            step="1"
          ></label>
        </template>
        <fieldset v-if="form.kind === 'scraper'">
          <legend>Reviewed website details</legend>
          <p class="field-help">
            Only configure a website after reviewing its terms and robots policy. These details preserve that local approval record.
          </p>
          <label>Story container <span class="field-help">CSS selector for one story, for example <code>article.story</code>.</span><input
            v-model="form.articleSelector"
            placeholder="article.story"
          ></label><label>Headline <span class="field-help">CSS selector for the headline inside each story, for example <code>h2</code>.</span><input
            v-model="form.titleSelector"
            placeholder="h2"
          ></label><label>Article text — optional <span class="field-help">Only used when full article storage is permitted, for example <code>.article-body</code>.</span><input
            v-model="form.contentSelector"
            placeholder=".article-body"
          ></label><label>Review decision <span class="field-help">An enabled website must have a complete, approved review.</span><select v-model="form.policyStatus"><option value="pending">Review pending</option><option value="approved">Approved</option><option value="rejected">Not approved</option></select></label><label>Publisher terms <span class="field-help">Public HTTP(S) page containing the publisher's terms.</span><input
            v-model="form.termsUrl"
            type="url"
            placeholder="https://example.com/terms"
          ></label><label>Robots policy <span class="field-help">Public HTTP(S) robots.txt address checked during review.</span><input
            v-model="form.robotsUrl"
            type="url"
            placeholder="https://example.com/robots.txt"
          ></label><label>Review date and time <span class="field-help">When the policy review was completed on this computer.</span><input
            v-model="form.reviewedAt"
            type="datetime-local"
          ></label><label>Review notes <span class="field-help">Briefly record what permits this use and any important limits.</span><textarea
            v-model="form.reviewNotes"
            placeholder="For example, public headlines may be fetched at a low rate; article text is not stored."
          /></label>
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
      <RefreshControl
        :server="server"
        :source-names="sourceNames"
      />
    </template>
  </section>
</template>

<style scoped>
.sources-workflow,.sources-header,.source-section{display:grid;gap:var(--space-5)}.sources-header{gap:var(--space-2)}.sources-header>p:last-child,.section-description,.source-form header p{color:var(--color-muted)}.source-section{padding:var(--space-5);border:1px solid var(--color-border);border-radius:var(--radius-lg);background:var(--color-surface-soft)}.source-empty{display:grid;gap:var(--space-1);padding:var(--space-5);border:1px dashed var(--color-border-strong);border-radius:var(--radius-md);background:var(--color-surface)}.source-empty p{color:var(--color-muted)}.source-card{display:grid;gap:var(--space-3);padding:var(--space-5);border:1px solid var(--color-border);border-radius:var(--radius-lg);background:var(--color-surface);box-shadow:var(--shadow-sm)}.source-card__header{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-4)}.source-card__header div{display:grid;gap:var(--space-1)}.source-card__header p,.source-card>p{color:var(--color-muted)}.source-state{padding:.3rem .65rem;border-radius:var(--radius-pill);background:var(--color-success-soft);color:var(--color-success);font-size:.82rem}.source-state--disabled{background:var(--color-surface-soft);color:var(--color-muted)}.source-card__badges,.source-actions,.actions{display:flex;align-items:center;flex-wrap:wrap;gap:var(--space-2)}.source-card a{overflow-wrap:anywhere}.starter-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-3);margin:0;padding:0;list-style:none}.starter-grid li{display:flex;align-items:center;justify-content:space-between;gap:var(--space-3);padding:var(--space-4);border:1px solid var(--color-border);border-radius:var(--radius-md);background:var(--color-surface)}.starter-grid li div{display:grid;gap:var(--space-1)}.starter-grid span{color:var(--color-muted);font-size:.9rem}.add-source{justify-self:start}.source-form{display:grid;gap:var(--space-4);padding:var(--space-5);border:1px solid var(--color-border);border-radius:var(--radius-lg);background:var(--color-surface);box-shadow:var(--shadow-sm)}.source-form header{display:grid;gap:var(--space-2)}.source-form label{display:grid;gap:var(--space-2);max-width:42rem;font-weight:720}.field-help{color:var(--color-muted);font-size:.9rem;font-weight:400}.source-form code{padding:.08rem .25rem;border-radius:var(--radius-sm);background:var(--color-surface-soft);font-size:.85em}.source-form input:not([type=checkbox]),.source-form select,.source-form textarea{width:100%;min-height:var(--control-height);padding:.65rem .75rem;border:1px solid var(--color-border-strong);border-radius:var(--radius-md);background:var(--color-surface)}.source-form textarea{min-height:7rem}.source-form fieldset{display:grid;gap:var(--space-4);padding:var(--space-4);border:1px solid var(--color-border);border-radius:var(--radius-md)}.sources-workflow :deep(.refresh-control>button){justify-self:start}@media(max-width:42rem){.source-section,.source-card,.source-form{padding:var(--space-4)}.starter-grid{grid-template-columns:1fr}.starter-grid li,.source-card__header{align-items:stretch;flex-direction:column}.source-actions button,.source-form .actions button,.add-source{width:100%;flex:1 1 auto}.sources-workflow :deep(.refresh-control>button){justify-self:stretch}}
</style>
