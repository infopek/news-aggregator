<script setup lang="ts">
import type { ArticleSummary } from '../../api/generated/models'
import { computed } from 'vue'
import PermissionBadge from './PermissionBadge.vue'
const props = withDefaults(defineProps<{ article: ArticleSummary; sourceName?: string; headingLevel?: 'h2' | 'h3' }>(), { sourceName: '', headingLevel: 'h3' })
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
    <component :is="headingLevel">
      <a
        v-if="publisherUrl"
        :href="publisherUrl"
        target="_blank"
        rel="noopener noreferrer"
      >{{ article.title }}</a>
      <span v-else>{{ article.title }}</span>
    </component>
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
