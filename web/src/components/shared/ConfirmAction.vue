<script setup lang="ts">
import { nextTick, ref, useId } from 'vue'
defineProps<{ label: string; confirmLabel?: string }>()
const emit = defineEmits<{ confirm: [] }>()
const open = ref(false); const trigger = ref<globalThis.HTMLButtonElement>(); const confirmButton = ref<globalThis.HTMLButtonElement>()
const dialog = ref<globalThis.HTMLElement>()
const titleId = `confirm-title-${useId()}`
async function show() { open.value = true; await nextTick(); confirmButton.value?.focus() }
async function close() { open.value = false; await nextTick(); trigger.value?.focus() }
function confirm() { emit('confirm'); void close() }
function trapFocus(event: globalThis.KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); void close(); return }
  if (event.key !== 'Tab') return
  const controls = Array.from(dialog.value?.querySelectorAll<globalThis.HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])') ?? [])
  if (!controls.length) return
  const first = controls[0]; const last = controls.at(-1)
  if (event.shiftKey && globalThis.document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && globalThis.document.activeElement === last) { event.preventDefault(); first?.focus() }
}
</script>
<template>
  <div class="confirm">
    <button
      ref="trigger"
      type="button"
      @click="show"
    >
      {{ label }}
    </button><div
      v-if="open"
      ref="dialog"
      role="alertdialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      @keydown="trapFocus"
    >
      <h3 :id="titleId">
        Confirm action
      </h3><p>This action cannot be undone.</p><button
        ref="confirmButton"
        type="button"
        @click="confirm"
      >
        {{ confirmLabel ?? 'Confirm' }}
      </button><button
        class="secondary"
        type="button"
        @click="close"
      >
        Cancel
      </button>
    </div>
  </div>
</template>
