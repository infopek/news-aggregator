<script setup lang="ts">
import type { ArticleSummary } from '../../api/generated/models'
import PermissionBadge from './PermissionBadge.vue'
defineProps<{ article: ArticleSummary; sourceName?: string }>()
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
        :href="article.canonicalUrl"
        target="_blank"
        rel="noopener noreferrer"
      >{{ article.title }}</a>
    </h3>
    <p v-if="article.excerpt">
      {{ article.excerpt }}
    </p>
    <a
      class="publisher-link"
      :href="article.canonicalUrl"
      target="_blank"
      rel="noopener noreferrer"
    >Read at publisher<span class="sr-only">: {{ article.title }}</span></a>
  </article>
</template>
