<script setup lang="ts">
/* global localStorage */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { RefreshRun } from '../../api/generated/models'
import type { ServerApi } from '../../api/server-api'
import { ApiRequestError } from '../../api/client'
import RefreshStatus from '../../components/shared/RefreshStatus.vue'
import { toUserSafeError } from '../../state/errors'
import { RefreshRecoveryPoller } from './refresh-recovery-poller'

const props = defineProps<{ server: ServerApi; sourceNames?: Record<string, string> }>()
const emit = defineEmits<{ started: []; completed: [RefreshRun] }>()
const refresh = ref<RefreshRun>()
const loading = ref(false)
const error = ref('')
const poller = new RefreshRecoveryPoller()
const storageKey = 'news-aggregator:last-refresh-id'
onMounted(recover)
onBeforeUnmount(() => poller.dispose())

async function start() {
  loading.value = true; error.value = ''
  try {
    refresh.value = await props.server.startRefresh()
    emit('started')
    localStorage.setItem(storageKey, refresh.value.id)
    await poll(refresh.value.id)
  } catch (cause) {
    error.value = toUserSafeError(cause).message
  } finally { loading.value = false }
}

async function recover() {
  const id = localStorage.getItem(storageKey)
  if (!id) return
  loading.value = true
  try { await poll(id) } catch (cause) { error.value = toUserSafeError(cause).message } finally { loading.value = false }
}

async function poll(id: string) {
  const result = await poller.poll(async (signal) => {
    try { return await props.server.refresh(id, signal) } catch (cause) {
      if (cause instanceof ApiRequestError && cause.status === 404) return undefined
      throw cause
    }
  })
  if (result.refresh) { refresh.value = result.refresh; emit('completed', result.refresh) }
  else if (result.reason === 'missing') localStorage.removeItem(storageKey)
  else if (result.reason === 'error') error.value = toUserSafeError(result.error).message
  else if (result.reason === 'timeout') error.value = 'Refresh status timed out. Reload to check its saved status.'
}
</script>

<template>
  <section aria-labelledby="manual-refresh-title">
    <h2 id="manual-refresh-title">
      Manual refresh
    </h2>
    <p>Refresh runs only while the local application process is open. Starting another refresh while one is active will be refused.</p>
    <button
      type="button"
      :disabled="loading"
      @click="start"
    >
      {{ loading ? 'Refresh running…' : 'Refresh all enabled sources' }}
    </button>
    <RefreshStatus
      :refresh="refresh"
      :loading="loading"
      :error="error"
      :source-names="sourceNames"
    />
  </section>
</template>
