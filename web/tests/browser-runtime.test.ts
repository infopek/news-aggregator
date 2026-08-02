import { describe, expect, it } from 'vitest'

import { browserCandidates, processTreeTermination, resolveBrowserExecutable } from './browser-runtime.mjs'

describe('browser proof runtime portability', () => {
  it('prefers and validates the explicit browser override', () => {
    const seen: string[] = []
    expect(resolveBrowserExecutable({
      platform: 'win32',
      env: { NEWS_AGGREGATOR_BROWSER: 'D:\\portable\\chrome.exe' },
      exists: (path: string) => { seen.push(path); return true }
    })).toBe('D:\\portable\\chrome.exe')
    expect(seen).toEqual(['D:\\portable\\chrome.exe'])
  })

  it('discovers standard Windows Chrome and Edge locations deterministically', () => {
    const env = {
      LOCALAPPDATA: 'C:\\Users\\Local\\AppData\\Local',
      PROGRAMFILES: 'C:\\Program Files',
      'PROGRAMFILES(X86)': 'C:\\Program Files (x86)'
    }
    const candidates = browserCandidates('win32', env)
    expect(candidates).toEqual([
      'C:\\Users\\Local\\AppData\\Local\\Google\\Chrome\\Application\\chrome.exe',
      'C:\\Users\\Local\\AppData\\Local\\Microsoft\\Edge\\Application\\msedge.exe',
      'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
      'C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe',
      'C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe',
      'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
    ])
    expect(resolveBrowserExecutable({
      platform: 'win32', env, exists: (path: string) => path === candidates[3]
    })).toBe(candidates[3])
  })

  it('uses taskkill for the complete Windows process tree', () => {
    expect(processTreeTermination('win32', 4173)).toEqual({
      command: 'taskkill', args: ['/pid', '4173', '/t', '/f']
    })
    expect(processTreeTermination('linux', 4173)).toEqual({ signal: 'SIGTERM', pid: -4173 })
  })

  it('returns an actionable error when no browser is available', () => {
    expect(() => resolveBrowserExecutable({ platform: 'linux', env: {}, exists: () => false }))
      .toThrow(/set NEWS_AGGREGATOR_BROWSER/)
  })
})
