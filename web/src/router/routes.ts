export interface AppRoute {
  name: string
  path: string
  title: string
  description: string
  navigation?: boolean
  navigationLabel?: string
}

export const routes: readonly AppRoute[] = [
  { name: 'setup', path: '/setup', title: 'First-run setup', description: 'Set up your private, local news experience.' },
  { name: 'feed', path: '/', title: 'Ranked feed', description: 'Your ranked news feed.', navigation: true, navigationLabel: 'Ranked feed' },
  { name: 'library', path: '/library', title: 'Personal library', description: 'Saved and previously read articles.', navigation: true, navigationLabel: 'Library' },
  { name: 'sources', path: '/sources', title: 'Sources and refresh', description: 'Manage sources and refresh news.', navigation: true, navigationLabel: 'Sources' },
  { name: 'settings', path: '/settings', title: 'Profile and ranking', description: 'Manage profile signals and ranking preferences.', navigation: true, navigationLabel: 'Settings' }
]

export const articleRoute: AppRoute = {
  name: 'article', path: '/articles/:articleId', title: 'Article reader', description: 'Read an article and its ranking explanation.'
}

export const notFoundRoute: AppRoute = {
  name: 'not-found', path: '*', title: 'Page not found', description: 'That page does not exist in this app.'
}

export function matchRoute(pathname: string): { route: AppRoute; params: Record<string, string> } {
  const route = routes.find((candidate) => candidate.path === pathname)
  if (route) return { route, params: {} }
  const article = pathname.match(/^\/articles\/([^/]+)$/)
  if (article) return { route: articleRoute, params: { articleId: decodeURIComponent(article[1]) } }
  return { route: notFoundRoute, params: {} }
}
