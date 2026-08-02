<script setup lang="ts">
defineProps<{ permission: 'metadata_only' | 'full_content_allowed' }>()
</script>
<template><span class="badge">{{ permission === 'full_content_allowed' ? 'Full article available' : 'Publisher link only' }}</span></template>
