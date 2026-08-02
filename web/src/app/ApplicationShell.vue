<script setup lang="ts">
/* global document, HTMLElement */
import { computed, nextTick, watch } from 'vue'

import GlobalStatus from '../components/layout/GlobalStatus.vue'
import RoutePlaceholder from '../components/layout/RoutePlaceholder.vue'
import AppLink from '../router/AppLink.vue'
import { routes } from '../router/routes'
import { useRouter } from '../router/router'
import { appName } from '../app-meta'
import { useInstallState } from './install-state'
import { shellStatus } from './shell-status'

const router = useRouter()
const lifecycle = useInstallState()
const navigation = routes.filter((route) => route.navigation)
const pageTitle = computed(() => `${router.current.value.route.title} · ${appName}`)

watch(pageTitle, async (title) => {
  document.title = title
  await nextTick()
  document.querySelector<HTMLElement>('#main-content h1')?.focus()
}, { immediate: true })
</script>

<template>
  <a
    class="skip-link"
    href="#main-content"
  >Skip to main content</a>
  <div class="app-frame">
    <header class="app-header">
      <AppLink
        class="brand"
        to="/"
        :aria-label="`${appName} home`"
      >
        {{ appName }}
      </AppLink>
      <div
        class="app-mode"
        aria-label="Application status"
      >
        <span aria-hidden="true" />Local only
      </div>
    </header>
    <nav
      class="primary-nav"
      aria-label="Primary navigation"
    >
      <AppLink
        v-for="item in navigation"
        :key="item.name"
        :to="item.path"
        :aria-current="router.current.value.route.name === item.name ? 'page' : undefined"
      >
        {{ item.navigationLabel }}
      </AppLink>
    </nav>
    <main
      id="main-content"
      tabindex="-1"
    >
      <GlobalStatus :status="shellStatus.current" />
      <RoutePlaceholder
        v-if="shellStatus.current.kind === 'ready'"
        :route="router.current.value.route"
        :article-id="router.current.value.params.articleId"
      />
    </main>
    <aside
      v-if="lifecycle.state.update === 'waiting'"
      class="update-banner"
      aria-labelledby="update-title"
    >
      <div><strong id="update-title">Update ready</strong><p>Reload to use the latest local app version.</p></div>
      <button
        type="button"
        @click="lifecycle.update"
      >
        Reload and update
      </button>
    </aside>
    <footer class="app-footer">
      <button
        v-if="lifecycle.state.install === 'available'"
        type="button"
        @click="lifecycle.install"
      >
        Install app
      </button>
      <p v-else-if="lifecycle.state.install === 'unsupported'">
        Installation is not available in this browser. You can keep using this tab.
      </p>
      <p v-else>
        Installed on this computer
      </p>
    </footer>
  </div>
</template>
