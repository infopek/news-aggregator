<script setup lang="ts">
/* global KeyboardEvent */
import { computed, ref } from 'vue'
import TagChip from '../../components/shared/TagChip.vue'

const props = defineProps<{ modelValue: string[]; error?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const draft = ref('')
const message = ref('')
const examples = ['Technology', 'Geopolitics', 'Local news', 'Science', 'AI', 'Football', 'Climate']
const normalized = computed(() => new Set(props.modelValue.map((item) => item.toLocaleLowerCase())))

function add(value = draft.value) {
  const topic = value.trim().replace(/\s+/g, ' ')
  message.value = ''
  if (!topic) return
  if (normalized.value.has(topic.toLocaleLowerCase())) { message.value = `${topic} is already in your interests.`; return }
  emit('update:modelValue', [...props.modelValue, topic])
  draft.value = ''
}
function remove(topic: string) { emit('update:modelValue', props.modelValue.filter((item) => item !== topic)) }
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); add() }
}
</script>

<template>
  <div class="interest-picker">
    <div
      v-if="modelValue.length"
      class="chip-list"
      aria-label="Selected interests"
    >
      <TagChip
        v-for="topic in modelValue"
        :key="topic"
        :label="topic"
        removable
        @remove="remove(topic)"
      />
    </div>
    <label for="interest-input">Add an interest</label>
    <div class="interest-entry">
      <input
        id="interest-input"
        v-model="draft"
        placeholder="Try “AI”, “Hungarian politics”, “space”, “football”…"
        maxlength="80"
        :aria-invalid="Boolean(error)"
        aria-describedby="interest-help interest-message"
        @keydown="onKeydown"
      >
      <button
        type="button"
        class="secondary"
        @click="add()"
      >
        Add topic
      </button>
    </div>
    <p
      id="interest-help"
      class="field__description"
    >
      Press Enter or comma to add a topic. You can remove it at any time.
    </p>
    <p
      v-if="message || error"
      id="interest-message"
      :class="error ? 'field__error' : 'field__description'"
      aria-live="polite"
    >
      {{ error || message }}
    </p>
    <div
      class="topic-suggestions"
      aria-label="Suggested interests"
    >
      <button
        v-for="topic in examples.filter((item) => !normalized.has(item.toLocaleLowerCase()))"
        :key="topic"
        type="button"
        class="tertiary suggestion"
        @click="add(topic)"
      >
        + {{ topic }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.interest-picker{display:grid;gap:var(--space-3)}.chip-list,.topic-suggestions,.interest-entry{display:flex;flex-wrap:wrap;gap:var(--space-2);align-items:center}.interest-entry input{flex:1 1 18rem;min-height:var(--control-height);padding:.65rem .75rem;border:1px solid var(--color-border-strong);border-radius:var(--radius-md)}.suggestion{min-height:2.25rem;padding:.35rem .65rem;border-radius:var(--radius-pill);font-size:.88rem}@media(max-width:32rem){.interest-entry>*{width:100%}}
</style>
