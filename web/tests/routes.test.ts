import { describe, expect, it } from 'vitest'

import { firstRunDestination } from '../src/router/router'
import { articleRoute, matchRoute, notFoundRoute, routes } from '../src/router/routes'
import type { Profile } from '../src/api/generated/models'

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

  it('routes only an authoritatively empty profile into first-run', () => {
    const empty = { interests: [], preferredSourceIds: [], location: { present: false } } as Profile
    const complete = { ...empty, interests: [{ name: 'news', weight: 1 }] } as Profile
    expect(firstRunDestination('/', empty)).toBe('/setup')
    expect(firstRunDestination('/', complete)).toBeUndefined()
    expect(firstRunDestination('/setup', complete)).toBe('/settings')
    expect(firstRunDestination('/setup', empty)).toBeUndefined()
    expect(firstRunDestination('/settings', empty)).toBeUndefined()
  })
})
