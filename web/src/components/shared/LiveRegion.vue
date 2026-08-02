<script setup lang="ts">
withDefaults(defineProps<{ message: string; urgent?: boolean }>(), { urgent: false })
</script>
<template>
  <p
    class="live-region"
    :role="urgent ? 'alert' : 'status'"
    :aria-live="urgent ? 'assertive' : 'polite'"
  >
    {{ message }}
  </p>
</template>
