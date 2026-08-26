<script setup lang="ts">
withDefaults(defineProps<{ summary: string; open?: boolean }>(), { open: false })
</script>

<template>
  <details
    class="disclosure"
    :open="open"
  >
    <summary>{{ summary }}</summary>
    <div class="disclosure__content">
      <slot />
    </div>
  </details>
</template>
