import { describe, expect, it } from 'vitest'

import { articleRoute, matchRoute, notFoundRoute, routes } from '../src/router/routes'

describe('route contract', () => {
  it('keeps every accepted screen on a stable path', () => {
    expect(routes.map(({ name, path }) => ({ name, path }))).toEqual([
      { name: 'setup', path: '/setup' },
      { name: 'feed', path: '/' },
      { name: 'library', path: '/library' },
      { name: 'sources', path: '/sources' },
      { name: 'settings', path: '/settings' }
    ])
    expect(articleRoute.path).toBe('/articles/:articleId')
  })

  it('makes every non-contextual screen reachable from primary navigation', () => {
    const acceptedScreenNames = ['setup', 'feed', 'library', 'sources', 'settings']
    expect(routes.filter((route) => route.navigation).map((route) => route.name)).toEqual(acceptedScreenNames)
    expect(routes.filter((route) => route.navigation).every((route) => Boolean(route.navigationLabel))).toBe(true)
    expect(articleRoute.path).toContain(':articleId')
  })

  it('decodes contextual article paths and rejects partial matches', () => {
    expect(matchRoute('/articles/local%20news')).toEqual({ route: articleRoute, params: { articleId: 'local news' } })
    expect(matchRoute('/articles/one/extra').route).toBe(notFoundRoute)
  })
})
