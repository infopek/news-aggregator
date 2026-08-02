<script setup lang="ts">
import type { RefreshRun } from '../../api/generated/models'
import { useId } from 'vue'
import LiveRegion from './LiveRegion.vue'
const props = defineProps<{ refresh?: RefreshRun; loading?: boolean; error?: string }>()
const titleId = `refresh-title-${useId()}`
const statusText = () => props.loading ? 'Refreshing sources…' : props.error ? `Refresh failed: ${props.error}` : !props.refresh ? 'Refresh has not run yet.' : props.refresh.status === 'partial_success' ? 'Refresh completed with some source failures.' : `Refresh ${props.refresh.status.replace('_', ' ')}.`
</script>
<template>
  <div
    class="status"
    :class="{ 'status--error': error || refresh?.status === 'failed' }"
    :aria-labelledby="titleId"
  >
    <h3 :id="titleId">
      Source refresh
    </h3><LiveRegion
      :message="statusText()"
      :urgent="Boolean(error)"
    />
    <ul v-if="refresh?.outcomes.length">
      <li
        v-for="outcome in refresh.outcomes"
        :key="outcome.sourceId"
      >
        {{ outcome.sourceId }}: {{ outcome.failed ? `${outcome.failed} failed${outcome.errorSummary ? ` — ${outcome.errorSummary}` : ''}` : `${outcome.inserted} new` }}
      </li>
    </ul>
  </div>
</template>
