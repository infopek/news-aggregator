<script setup lang="ts">
import type { AppRoute } from '../../router/routes'
import AppLink from '../../router/AppLink.vue'
import { ProfileSettings } from '../../features/profile'
import { FirstRunSetup } from '../../features/setup'

defineProps<{ route: AppRoute; articleId?: string }>()
</script>

<template>
  <FirstRunSetup v-if="route.name === 'setup'" />
  <ProfileSettings v-else-if="route.name === 'settings'" />
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
