<script setup lang="ts">
import { nextTick, ref } from 'vue'
import LiveRegion from './LiveRegion.vue'
defineProps<{ id: string; label?: string; submitting?: boolean }>()
const emit = defineEmits<{ submit: [secret: string]; cancel: [] }>()
const secret = ref('')
const input = ref<globalThis.HTMLInputElement>()
const announcement = ref('')
async function clear(message: string) { secret.value = ''; announcement.value = message; await nextTick(); input.value?.focus() }
function submit() { const value = secret.value; if (!value) return; emit('submit', value); void clear('Credential submitted and cleared from this form.') }
function cancel() { emit('cancel'); void clear('Credential entry cancelled and cleared.') }
</script>
<template>
  <form
    class="credential"
    autocomplete="off"
    @submit.prevent="submit"
    @reset.prevent="cancel"
  >
    <label :for="id">{{ label ?? 'Credential' }}</label>
    <p :id="`${id}-help`">
      Write-only. The value is sent once and is not displayed or stored by this form.
    </p>
    <input
      :id="id"
      ref="input"
      v-model="secret"
      type="password"
      autocomplete="new-password"
      :disabled="submitting"
      :aria-describedby="`${id}-help`"
    >
    <div class="actions">
      <button
        type="submit"
        :disabled="submitting || !secret"
      >
        Save credential
      </button><button
        class="secondary"
        type="reset"
      >
        Cancel
      </button>
    </div>
    <LiveRegion :message="announcement" />
  </form>
</template>
