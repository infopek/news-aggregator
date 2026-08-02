<script setup lang="ts">
defineProps<{ id: string; label: string; modelValue: string; options: Array<{ value: string; label: string }>; loading?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>
<template>
  <label
    class="filter"
    :for="id"
  >{{ label }}<select
    :id="id"
    :value="modelValue"
    :disabled="loading"
    @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  ><option
    v-for="option in options"
    :key="option.value"
    :value="option.value"
  >{{ option.label }}</option></select></label>
</template>
