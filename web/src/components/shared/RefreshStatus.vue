<script setup lang="ts">
import type { RefreshRun } from '../../api/generated/models'
import LiveRegion from './LiveRegion.vue'

const props = defineProps<{ refresh?: RefreshRun; loading?: boolean; error?: string; statusError?: string; sourceNames?: Record<string, string> }>()
const statusText = () => props.loading ? 'Refreshing sources…' : props.statusError ? `Refresh status unavailable: ${props.statusError}` : props.error ? `Refresh failed: ${props.error}` : !props.refresh ? 'Refresh has not run yet.' : props.refresh.status === 'partial_success' ? 'Refresh complete with some source issues.' : props.refresh.status === 'succeeded' ? 'Refresh complete.' : `Refresh ${props.refresh.status.replace('_', ' ')}.`
function outcomeText(outcome: RefreshRun['outcomes'][number]) {
  if (outcome.errorCode === 'cancelled') return `Cancelled${outcome.errorSummary ? ` — ${outcome.errorSummary}` : ''}`
  if (outcome.errorCode === 'rate_limited') return `Rate limited${outcome.errorSummary ? ` — ${outcome.errorSummary}` : ''}`
  if (outcome.failed) return `Failed${outcome.errorSummary ? ` — ${outcome.errorSummary}` : ''}`
  if (!outcome.inserted && !outcome.updated) return 'No new stories'
  return `${outcome.inserted} new${outcome.updated ? `, ${outcome.updated} updated` : ''}`
}
function sourceLabel(sourceId: string) { return props.sourceNames?.[sourceId] ?? `Deleted or unavailable source (${sourceId})` }
const totalNew = () => props.refresh?.outcomes.reduce((sum, item) => sum + item.inserted, 0) ?? 0
</script>
<template>
  <div
    class="refresh-status"
    :class="{ 'refresh-status--error': error || statusError || refresh?.status === 'failed', 'refresh-status--warning': refresh?.status === 'partial_success' }"
  >
    <LiveRegion
      :message="statusText()"
      :urgent="Boolean(error || statusError)"
    />
    <p
      v-if="refresh?.status === 'succeeded'"
      class="refresh-status__summary"
    >
      <span aria-hidden="true">✓</span> Refresh complete<span v-if="totalNew()"> · {{ totalNew() }} new stories</span>
    </p>
    <p
      v-else-if="refresh?.status === 'failed'"
      class="refresh-status__summary"
    >
      Refresh failed.
    </p>
    <details
      v-if="refresh?.outcomes.length"
      class="refresh-status__details"
    >
      <summary>{{ refresh.status === 'partial_success' ? 'Refresh complete with source issues' : 'Refresh details' }}</summary>
      <ul>
        <li
          v-for="outcome in refresh.outcomes"
          :key="outcome.sourceId"
        >
          <strong>{{ sourceLabel(outcome.sourceId) }}</strong><span>{{ outcomeText(outcome) }}</span><small>{{ outcome.fetched }} fetched · {{ outcome.inserted }} new · {{ outcome.updated }} updated · {{ outcome.skipped }} skipped · {{ outcome.failed }} failed</small>
        </li>
      </ul>
    </details>
  </div>
</template>
<style scoped>
.refresh-status { display: grid; gap: var(--space-2); }
.refresh-status__summary { color: var(--color-success); font-weight: 720; }
.refresh-status--error .refresh-status__summary { color: var(--color-danger); }
.refresh-status__details { border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-surface-soft); }
.refresh-status__details summary { min-height: 2.75rem; display: flex; align-items: center; padding: var(--space-2) var(--space-3); color: var(--color-brand); cursor: pointer; font-weight: 720; }
.refresh-status__details ul { display: grid; gap: var(--space-3); margin: 0; padding: 0 var(--space-3) var(--space-3); list-style: none; }
.refresh-status__details li { display: grid; gap: var(--space-1); }
.refresh-status__details small { color: var(--color-muted); }
</style>
