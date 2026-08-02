<script setup lang="ts">
import type { ShellStatus } from '../../app/shell-status'

defineProps<{ status: ShellStatus }>()
</script>

<template>
  <section
    v-if="status.kind === 'loading'"
    class="status"
    aria-live="polite"
    aria-busy="true"
  >
    <h2>Loading</h2><p>{{ status.message }}</p>
  </section>
  <section
    v-else-if="status.kind === 'api-down'"
    class="status status--error"
    role="alert"
  >
    <h2>Local app service unavailable</h2>
    <p>News Aggregator cannot reach the service running on this computer.</p>
    <div class="actions">
      <button
        v-if="status.retry"
        type="button"
        @click="status.retry"
      >
        Retry connection
      </button><p>Close this window, relaunch News Aggregator, then retry.</p>
    </div>
  </section>
  <section
    v-else-if="status.kind === 'error'"
    class="status status--error"
    role="alert"
  >
    <h2>Something went wrong</h2><p>{{ status.message }}</p>
    <button
      v-if="status.retry"
      type="button"
      @click="status.retry"
    >
      Try again
    </button>
  </section>
</template>
