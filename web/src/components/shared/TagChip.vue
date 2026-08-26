<script setup lang="ts">
withDefaults(defineProps<{ label: string; removable?: boolean; disabled?: boolean }>(), { removable: false, disabled: false })
defineEmits<{ remove: [] }>()
</script>

<template>
  <button
    v-if="removable"
    type="button"
    class="chip chip--button"
    :disabled="disabled"
    :aria-label="`Remove ${label}`"
    @click="$emit('remove')"
  >
    <span>{{ label }}</span><span aria-hidden="true">×</span>
  </button>
  <span
    v-else
    class="chip"
  ><slot>{{ label }}</slot></span>
</template>
