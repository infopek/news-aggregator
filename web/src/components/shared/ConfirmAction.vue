<script setup lang="ts">
import { nextTick, ref } from 'vue'
defineProps<{ label: string; confirmLabel?: string }>()
const emit = defineEmits<{ confirm: [] }>()
const open = ref(false); const trigger = ref<globalThis.HTMLButtonElement>(); const confirmButton = ref<globalThis.HTMLButtonElement>()
async function show() { open.value = true; await nextTick(); confirmButton.value?.focus() }
async function close() { open.value = false; await nextTick(); trigger.value?.focus() }
function confirm() { emit('confirm'); void close() }
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
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      @keydown.esc="close"
    >
      <h3 id="confirm-title">
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
