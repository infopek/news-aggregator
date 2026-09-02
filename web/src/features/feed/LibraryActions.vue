<script setup lang="ts">
import type { ArticleSummary, LibraryStateWrite } from '../../api/generated/models'
import LiveRegion from '../../components/shared/LiveRegion.vue'

defineProps<{ article: ArticleSummary; busy?: boolean; message?: string }>()
defineEmits<{ mutate: [LibraryStateWrite] }>()
</script>
<template>
  <div
    class="article-actions"
    aria-label="Article actions"
  >
    <button
      type="button"
      class="tertiary"
      :disabled="busy"
      @click="$emit('mutate', { read: !article.library.readAt })"
    >
      {{ article.library.readAt ? 'Mark unread' : 'Mark read' }}
    </button>
    <button
      type="button"
      class="tertiary"
      :disabled="busy"
      @click="$emit('mutate', { saved: !article.library.savedAt })"
    >
      {{ article.library.savedAt ? 'Unsave' : 'Save' }}
    </button>
    <button
      type="button"
      class="tertiary article-actions__hide"
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
<style scoped>
.article-actions { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-1); }
.article-actions button { min-height: 2.5rem; padding-inline: .7rem; }
.article-actions__hide { color: var(--color-muted); }
@media (max-width: 34rem) { .article-actions { justify-content: flex-start; } .article-actions button { flex: 1 1 auto; } }
</style>
