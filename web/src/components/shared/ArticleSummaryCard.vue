<script setup lang="ts">
import type { ArticleSummary } from '../../api/generated/models'
import { computed } from 'vue'
import PermissionBadge from './PermissionBadge.vue'
const props = defineProps<{ article: ArticleSummary; sourceName?: string }>()
const publisherUrl = computed(() => {
  try {
    const parsed = new globalThis.URL(props.article.canonicalUrl)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : null
  } catch { return null }
})
</script>
<template>
  <article
    class="article-card"
    :dir="article.language === 'ar' || article.language === 'he' ? 'rtl' : 'auto'"
  >
    <header>
      <PermissionBadge :permission="article.contentPermission" /><p class="article-card__source">
        {{ sourceName }}
      </p>
    </header>
    <h3>
      <a
        v-if="publisherUrl"
        :href="publisherUrl"
        target="_blank"
        rel="noopener noreferrer"
      >{{ article.title }}</a>
      <span v-else>{{ article.title }}</span>
    </h3>
    <p v-if="article.excerpt">
      {{ article.excerpt }}
    </p>
    <a
      v-if="publisherUrl"
      class="publisher-link"
      :href="publisherUrl"
      target="_blank"
      rel="noopener noreferrer"
    >Read at publisher<span class="sr-only">: {{ article.title }}</span></a>
    <p
      v-else
      class="publisher-link--unavailable"
    >
      Publisher link unavailable because its address is invalid.
    </p>
  </article>
</template>
