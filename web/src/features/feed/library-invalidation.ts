type Listener = (articleId: string) => void

const listeners = new Set<Listener>()

export function publishLibraryInvalidation(articleId: string) {
  for (const listener of listeners) listener(articleId)
}

export function subscribeLibraryInvalidation(listener: Listener) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
