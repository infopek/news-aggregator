<script setup lang="ts">
import type { ArticleSummary, LibraryStateWrite } from '../../api/generated/models'
import LiveRegion from '../../components/shared/LiveRegion.vue'

defineProps<{ article: ArticleSummary; busy?: boolean; message?: string }>()
defineEmits<{ mutate: [LibraryStateWrite] }>()
</script>
<template>
  <div
    class="actions"
    aria-label="Article actions"
  >
    <button
      type="button"
      :disabled="busy"
      @click="$emit('mutate', { read: !article.library.readAt })"
    >
      {{ article.library.readAt ? 'Mark unread' : 'Mark read' }}
    </button>
    <button
      type="button"
      :disabled="busy"
      @click="$emit('mutate', { saved: !article.library.savedAt })"
    >
      {{ article.library.savedAt ? 'Unsave' : 'Save' }}
    </button>
    <button
      type="button"
      :disabled="busy"
      @click="$emit('mutate', { hidden: true })"
    >
      Hide
    </button>
    <LiveRegion
      :message="message ?? ''"
      :urgent="message?.includes('unavailable') ?? false"
    />
  </div>
</template>
