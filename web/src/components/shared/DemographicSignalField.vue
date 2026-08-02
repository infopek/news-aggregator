<script setup lang="ts">
import AccessibleField from './AccessibleField.vue'
defineProps<{ id: string; label: string; modelValue: string; enabled: boolean; error?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; 'update:enabled': [value: boolean] }>()
</script>
<template>
  <fieldset class="privacy-field">
    <legend>{{ label }}</legend>
    <p :id="`${id}-privacy`">
      Optional and stored only on this device. It has low influence on ranking and can be disabled independently.
    </p>
    <label><input
      :checked="enabled"
      type="checkbox"
      @change="emit('update:enabled', ($event.target as HTMLInputElement).checked)"
    > Use this signal</label>
    <AccessibleField
      :id="id"
      :label="`${label} value`"
      :error="error"
    >
      <template #default="{ describedby }">
        <input
          :id="id"
          :value="modelValue"
          :disabled="!enabled"
          :aria-describedby="`${id}-privacy ${describedby}`.trim()"
          :aria-invalid="Boolean(error)"
          @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        >
      </template>
    </AccessibleField>
  </fieldset>
</template>
