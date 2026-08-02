import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

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
