import { onBeforeUnmount, onMounted, reactive } from 'vue'

interface InstallPromptEvent extends Event {
  prompt(): Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

export type InstallState = 'unsupported' | 'available' | 'dismissed' | 'installed'

export function useInstallState() {
  const state = reactive<{ install: InstallState; update: 'current' | 'waiting'; applyUpdate?: () => Promise<void> }>({
    install: isStandalone() ? 'installed' : 'unsupported',
    update: 'current'
  })
  let promptEvent: InstallPromptEvent | undefined

  const beforeInstall = (event: Event) => {
    event.preventDefault()
    promptEvent = event as InstallPromptEvent
    state.install = 'available'
  }
  const installed = () => { state.install = 'installed'; promptEvent = undefined }
  const updateWaiting = (event: Event) => {
    const detail = (event as CustomEvent<{ apply: () => Promise<void> }>).detail
    state.update = 'waiting'
    state.applyUpdate = detail.apply
  }
  onMounted(() => {
    window.addEventListener('beforeinstallprompt', beforeInstall)
    window.addEventListener('appinstalled', installed)
    window.addEventListener('app:update-waiting', updateWaiting)
  })
  onBeforeUnmount(() => {
    window.removeEventListener('beforeinstallprompt', beforeInstall)
    window.removeEventListener('appinstalled', installed)
    window.removeEventListener('app:update-waiting', updateWaiting)
  })

  return {
    state,
    async install() {
      if (!promptEvent) return
      await promptEvent.prompt()
      const choice = await promptEvent.userChoice
      state.install = choice.outcome === 'accepted' ? 'installed' : 'dismissed'
      promptEvent = undefined
    },
    async update() {
      await state.applyUpdate?.()
    }
  }
}

function isStandalone(): boolean {
  return typeof window !== 'undefined' && (window.matchMedia('(display-mode: standalone)').matches || Boolean((navigator as Navigator & { standalone?: boolean }).standalone))
}
