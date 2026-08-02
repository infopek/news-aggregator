<script setup lang="ts">
defineProps<{ id: string; label: string; description?: string; error?: string }>()
</script>

<template>
  <div
    class="field"
    :class="{ 'field--invalid': error }"
  >
    <label :for="id">{{ label }}</label>
    <p
      v-if="description"
      :id="`${id}-description`"
      class="field__description"
    >
      {{ description }}
    </p>
    <slot :describedby="[description && `${id}-description`, error && `${id}-error`].filter(Boolean).join(' ')" />
    <p
      v-if="error"
      :id="`${id}-error`"
      class="field__error"
      role="alert"
    >
      {{ error }}
    </p>
  </div>
</template>
