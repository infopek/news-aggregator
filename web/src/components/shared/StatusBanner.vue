<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title?: string
  tone?: 'info' | 'success' | 'warning' | 'danger'
  live?: boolean
}>(), { title: '', tone: 'info', live: false })

const role = computed(() => props.tone === 'danger' ? 'alert' : props.live ? 'status' : undefined)
</script>

<template>
  <section
    :class="['status-banner', `status-banner--${tone}`]"
    :role="role"
    :aria-live="live && tone !== 'danger' ? 'polite' : undefined"
  >
    <div class="status-banner__body">
      <strong v-if="title">{{ title }}</strong>
      <slot />
    </div>
    <slot name="actions" />
  </section>
</template>
