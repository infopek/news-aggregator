<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import type { RefreshRun } from '../../api/generated/models'
import type { ServerApi } from '../../api/server-api'
import RefreshStatus from '../../components/shared/RefreshStatus.vue'
import { toUserSafeError } from '../../state/errors'
import { RefreshPoller } from '../../state/refresh-poller'

const props = defineProps<{ server: ServerApi }>()
const refresh = ref<RefreshRun>()
const loading = ref(false)
const error = ref('')
const poller = new RefreshPoller()
onBeforeUnmount(() => poller.dispose())

async function start() {
  loading.value = true; error.value = ''
  try {
    refresh.value = await props.server.startRefresh()
    const result = await poller.poll((signal) => props.server.refresh(refresh.value!.id, signal))
    if (result.refresh) refresh.value = result.refresh
    else if (result.reason === 'timeout') error.value = 'Refresh status timed out. Reload to check its saved status.'
  } catch (cause) {
    error.value = toUserSafeError(cause).message
  } finally { loading.value = false }
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
    />
  </section>
</template>
