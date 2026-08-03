import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { LocalApiClient } from '../api/client'
import { createServerApi } from '../api/server-api'
import type { Profile } from '../api/generated/models'
import { matchRoute } from './routes'

const currentPath = ref(typeof window === 'undefined' ? '/' : window.location.pathname)

function readLocation(): void {
  currentPath.value = window.location.pathname
}

export function navigate(path: string): void {
  if (path === window.location.pathname) return
  window.history.pushState(null, '', path)
  readLocation()
  window.dispatchEvent(new CustomEvent('app:navigation'))
}

export function useRouter() {
  const onHistoryChange = () => readLocation()
  onMounted(() => {
    readLocation()
    window.addEventListener('popstate', onHistoryChange)
    window.addEventListener('app:navigation', onHistoryChange)
    void routeNewUserToSetup()
  })
  onBeforeUnmount(() => {
    window.removeEventListener('popstate', onHistoryChange)
    window.removeEventListener('app:navigation', onHistoryChange)
  })
  return {
    currentPath,
    current: computed(() => matchRoute(currentPath.value))
  }
}

export function firstRunDestination(path: string, profile: Profile): string | undefined {
  const empty = profile.interests.length === 0 && profile.preferredSourceIds.length === 0 && !profile.location.present
  if (path === '/' && empty) return '/setup'
  if (path === '/setup' && !empty) return '/settings'
  return undefined
}

async function routeNewUserToSetup(): Promise<void> {
  if (window.location.pathname !== '/' && window.location.pathname !== '/setup') return
  try {
    const destination = firstRunDestination(window.location.pathname, await createServerApi(new LocalApiClient()).profile())
    if (destination) navigate(destination)
  } catch { /* The shell owns API-unavailable feedback; never guess first-run state. */ }
}
