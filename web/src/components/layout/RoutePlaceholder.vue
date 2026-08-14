<script setup lang="ts">
import type { AppRoute } from '../../router/routes'
import AppLink from '../../router/AppLink.vue'
import { ProfileSettings } from '../../features/profile'
import { FirstRunSetup } from '../../features/setup'
import { SourceManagement } from '../../features/sources'
import { RankedFeed } from '../../features/feed'
import { ArticleReader } from '../../features/reader'
import { LibraryView } from '../../features/library'

defineProps<{ route: AppRoute; articleId?: string }>()
</script>

<template>
  <FirstRunSetup v-if="route.name === 'setup'" />
  <ProfileSettings v-else-if="route.name === 'settings'" />
  <SourceManagement v-else-if="route.name === 'sources'" />
  <RankedFeed v-else-if="route.name === 'feed'" />
  <LibraryView v-else-if="route.name === 'library'" />
  <ArticleReader
    v-else-if="route.name === 'article'"
    :article-id="articleId ?? ''"
  />
  <section
    v-else
    class="route-boundary"
    :aria-labelledby="`${route.name}-title`"
  >
    <p class="eyebrow">
      Local workspace
    </p>
    <h1
      :id="`${route.name}-title`"
      tabindex="-1"
    >
      {{ route.title }}
    </h1>
    <p>{{ route.description }}</p>
    <p
      v-if="route.name === 'article'"
      class="context"
    >
      Article reference: {{ articleId }}
    </p>
    <template v-if="route.name === 'not-found'">
      <p>Check the address or return to your ranked feed.</p>
      <AppLink
        class="button-link"
        to="/"
      >
        Go to ranked feed
      </AppLink>
    </template>
    <p
      v-else
      class="boundary-note"
    >
      This workflow will appear here when its feature module is ready.
    </p>
  </section>
</template>
