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
    <div class="credential__heading">
      <strong>{{ label ?? 'Private API credential' }}</strong><span>Write-only</span>
    </div><label :for="id">Credential value</label>
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
      placeholder="Paste the credential once"
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
<style scoped>
.credential{display:grid;gap:var(--space-3);padding:var(--space-4);border:1px solid var(--color-border);border-radius:var(--radius-md);background:var(--color-surface-soft)}.credential__heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-3)}.credential__heading span{padding:.25rem .55rem;border-radius:var(--radius-pill);background:var(--color-brand-soft);color:var(--color-brand-strong);font-size:.8rem;font-weight:700}.credential p{color:var(--color-muted);font-size:.9rem}.credential input{width:100%;min-height:var(--control-height);padding:.65rem .75rem;border:1px solid var(--color-border-strong);border-radius:var(--radius-md);background:var(--color-surface)}.actions{display:flex;flex-wrap:wrap;gap:var(--space-2)}@media(max-width:36rem){.actions button{flex:1 1 auto}}
</style>
