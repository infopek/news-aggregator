<script setup lang="ts">defineProps<{ state: 'loading' | 'empty' | 'success' | 'error'; error?: string }>()</script>
<template>
  <div
    class="async-state"
    :aria-busy="state === 'loading'"
  >
    <p
      v-if="state === 'loading'"
      role="status"
    >
      Loading…
    </p><p v-else-if="state === 'empty'">
      Nothing to show yet.
    </p><p
      v-else-if="state === 'error'"
      role="alert"
    >
      {{ error ?? 'Something went wrong.' }}
    </p><slot v-else />
  </div>
</template>
