<script setup lang="ts">
withDefaults(defineProps<{ label?: string }>(), { label: 'Actions' })
</script>

<template>
  <div
    class="action-group"
    role="group"
    :aria-label="label"
  >
    <slot />
  </div>
</template>
