<script setup lang="ts">
import type { ShellStatus } from '../../app/shell-status'
import StatusBanner from '../shared/StatusBanner.vue'

defineProps<{ status: ShellStatus }>()
</script>

<template>
  <StatusBanner
    v-if="status.kind === 'loading'"
    title="Loading"
    live
    aria-busy="true"
  >
    <p>{{ status.message }}</p>
  </StatusBanner>
  <StatusBanner
    v-else-if="status.kind === 'api-down'"
    title="Local app service unavailable"
    tone="danger"
  >
    <p>News Aggregator cannot reach the service running on this computer.</p>
    <div class="action-group">
      <button
        v-if="status.retry"
        type="button"
        @click="status.retry"
      >
        Retry connection
      </button><p>Close this window, relaunch News Aggregator, then retry.</p>
    </div>
  </StatusBanner>
  <StatusBanner
    v-else-if="status.kind === 'error'"
    title="Something went wrong"
    tone="danger"
  >
    <p>{{ status.message }}</p>
    <button
      v-if="status.retry"
      type="button"
      @click="status.retry"
    >
      Try again
    </button>
  </StatusBanner>
</template>
