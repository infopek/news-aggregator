<script setup lang="ts">
import type { ArticleSummary } from '../../api/generated/models'
import { computed } from 'vue'
import AppLink from '../../router/AppLink.vue'
import PermissionBadge from './PermissionBadge.vue'

const props = withDefaults(defineProps<{ article: ArticleSummary; sourceName?: string; headingLevel?: 'h2' | 'h3'; readerTo?: string }>(), { sourceName: '', headingLevel: 'h3', readerTo: '' })
const publisherUrl = computed(() => {
  try {
    const parsed = new globalThis.URL(props.article.canonicalUrl)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : null
  } catch { return null }
})
const published = computed(() => {
  if (!props.article.publishedAt) return 'Publication time unavailable'
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(props.article.publishedAt))
})
</script>

<template>
  <article
    class="article-card"
    :dir="article.language === 'ar' || article.language === 'he' ? 'rtl' : 'auto'"
  >
    <div class="article-card__meta">
      <span class="article-card__source">{{ sourceName }}</span>
      <span aria-hidden="true">·</span>
      <time
        v-if="article.publishedAt"
        :datetime="article.publishedAt"
      >{{ published }}</time>
      <span v-else>{{ published }}</span>
      <PermissionBadge :permission="article.contentPermission" />
    </div>
    <component
      :is="headingLevel"
      class="article-card__title"
    >
      <AppLink
        v-if="readerTo"
        :to="readerTo"
      >
        {{ article.title }}
      </AppLink>
      <a
        v-else-if="publisherUrl"
        :href="publisherUrl"
        target="_blank"
        rel="noopener noreferrer"
      >{{ article.title }}</a>
      <span v-else>{{ article.title }}</span>
    </component>
    <p
      v-if="article.excerpt"
      class="article-card__excerpt"
    >
      {{ article.excerpt }}
    </p>
    <div
      v-if="$slots.explanation"
      class="article-card__explanation"
    >
      <slot name="explanation" />
    </div>
    <div class="article-card__footer">
      <div class="article-card__primary">
        <AppLink
          v-if="readerTo"
          class="button-link"
          :to="readerTo"
        >
          Open article
        </AppLink>
        <a
          v-else-if="publisherUrl"
          class="button-link"
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
      </div>
      <slot name="actions" />
    </div>
  </article>
</template>

<style scoped>
.article-card { gap: var(--space-4); }
.article-card__meta { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-2); color: var(--color-muted); font-size: .86rem; }
.article-card__source { color: var(--color-ink); font-weight: 750; }
.article-card__title { font-size: clamp(1.2rem, 3vw, 1.55rem); }
.article-card__title a { color: var(--color-ink); text-decoration: none; }
.article-card__title a:hover { color: var(--color-brand); text-decoration: underline; }
.article-card__excerpt { display: -webkit-box; overflow: hidden; color: var(--color-muted); line-height: 1.6; -webkit-box-orient: vertical; -webkit-line-clamp: 3; }
.article-card__footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--space-3); padding-top: var(--space-2); }
.article-card__primary { display: flex; }
@media (max-width: 34rem) { .article-card__footer { align-items: stretch; flex-direction: column; } .article-card__primary .button-link { width: 100%; } }
</style>
