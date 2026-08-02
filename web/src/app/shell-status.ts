import { reactive } from 'vue'

export type ShellStatus =
  | { kind: 'ready' }
  | { kind: 'loading'; message: string }
  | { kind: 'error'; message: string; retry?: () => void }
  | { kind: 'api-down'; retry?: () => void }

export const shellStatus = reactive<{ current: ShellStatus }>({ current: { kind: 'ready' } })

export function setShellStatus(status: ShellStatus): void {
  shellStatus.current = status
}
