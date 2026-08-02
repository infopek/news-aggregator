import { registerSW } from 'virtual:pwa-register'

export interface ServiceWorkerUpdate {
  apply(): Promise<void>
  dismiss(): void
}

export function registerApplicationServiceWorker(
  onUpdateAvailable: (update: ServiceWorkerUpdate) => void = () => undefined
): void {
  let dismissed = false
  const updateServiceWorker = registerSW({
    immediate: true,
    onNeedRefresh() {
      onUpdateAvailable({
        apply: () => updateServiceWorker(true),
        dismiss: () => { dismissed = true }
      })
    },
    onRegisteredSW(_, registration) {
      if (!dismissed) void registration?.update()
    },
    onRegisterError(error) {
      console.warn('Service worker registration failed; continuing online.', error)
    }
  })
}
