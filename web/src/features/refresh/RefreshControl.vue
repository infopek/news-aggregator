<script setup lang="ts">
/* global localStorage */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { RefreshRun } from '../../api/generated/models'
import type { ServerApi } from '../../api/server-api'
import { ApiRequestError } from '../../api/client'
import RefreshStatus from '../../components/shared/RefreshStatus.vue'
import { toUserSafeError } from '../../state/errors'
import { RefreshRecoveryPoller } from './refresh-recovery-poller'
import type { RecoveryReason } from './refresh-recovery-poller'

const props = withDefaults(defineProps<{ server: ServerApi; sourceNames?: Record<string, string>; compact?: boolean }>(), { sourceNames: () => ({}), compact: false })
const emit = defineEmits<{ started: []; completed: [RefreshRun]; stopped: [Exclude<RecoveryReason, RefreshRun['status']|'disposed'|'obsolete'>] }>()
const refresh = ref<RefreshRun>()
const loading = ref(false)
const error = ref('')
const statusError = ref('')
const pendingId = ref('')
const poller = new RefreshRecoveryPoller()
const storageKey = 'news-aggregator:last-refresh-id'
onMounted(recover)
onBeforeUnmount(() => poller.dispose())

async function start() {
  loading.value = true; error.value = '';statusError.value=''
  try {
    refresh.value = await props.server.startRefresh()
    emit('started')
    pendingId.value = refresh.value.id
    localStorage.setItem(storageKey, refresh.value.id)
    await poll(refresh.value.id)
  } catch (cause) {
    error.value = toUserSafeError(cause).message
  } finally { loading.value = false }
}

async function recover() {
  const id = localStorage.getItem(storageKey)
  if (!id) return
  pendingId.value = id
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
  else if (result.reason === 'missing') { refresh.value=undefined;pendingId.value='';statusError.value='The saved refresh status is no longer available. Start a new refresh to update the feed.';localStorage.removeItem(storageKey);emit('stopped','missing') }
  else if (result.reason === 'error') { statusError.value = toUserSafeError(result.error).message;emit('stopped','error') }
  else if (result.reason === 'timeout') { statusError.value = 'Refresh status timed out. Retry to check its saved status.';emit('stopped','timeout') }
}
async function retry(){if(!pendingId.value)return;loading.value=true;statusError.value='';try{await poll(pendingId.value)}finally{loading.value=false}}
</script>

<template>
  <section
    :class="['refresh-control', { 'refresh-control--compact': compact }]"
    aria-labelledby="manual-refresh-title"
  >
    <div
      v-if="!compact"
      class="refresh-control__intro"
    >
      <h2 id="manual-refresh-title">
        Refresh news
      </h2>
      <p>Checks all enabled sources while this local application is open.</p>
    </div>
    <h2
      v-else
      id="manual-refresh-title"
      class="sr-only"
    >
      Refresh news
    </h2>
    <button
      type="button"
      :disabled="loading"
      @click="start"
    >
      {{ loading ? 'Refreshing…' : 'Refresh news' }}
    </button>
    <RefreshStatus
      :refresh="refresh"
      :loading="loading"
      :error="error"
      :status-error="statusError"
      :source-names="sourceNames"
    />
    <button
      v-if="statusError && pendingId"
      type="button"
      :disabled="loading"
      @click="retry"
    >
      Retry refresh status
    </button>
  </section>
</template>
<style scoped>
.refresh-control { display: grid; gap: var(--space-3); }
.refresh-control__intro { display: grid; gap: var(--space-1); }
.refresh-control__intro p { color: var(--color-muted); }
.refresh-control--compact { justify-items: end; }
.refresh-control--compact :deep(.refresh-status) { max-width: 32rem; justify-items: end; text-align: end; }
@media (max-width: 40rem) { .refresh-control--compact { width: 100%; justify-items: stretch; } .refresh-control--compact :deep(.refresh-status) { justify-items: stretch; text-align: start; } }
</style>
