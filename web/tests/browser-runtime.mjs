/* global process */
import { existsSync } from 'node:fs'
import { win32 } from 'node:path'

export function resolveBrowserExecutable({ platform = process.platform, env = process.env, exists = existsSync } = {}) {
  const override = env.NEWS_AGGREGATOR_BROWSER || env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
  if (override) {
    if (exists(override)) return override
    throw new Error(`Configured browser executable does not exist: ${override}`)
  }

  const candidates = browserCandidates(platform, env)
  const executable = candidates.find((candidate) => exists(candidate))
  if (executable) return executable
  throw new Error(
    `No supported browser executable found for ${platform}. ` +
    `Install Chrome/Chromium/Edge or set NEWS_AGGREGATOR_BROWSER to its executable path. Checked: ${candidates.join(', ')}`
  )
}

export function browserCandidates(platform, env = {}) {
  if (platform === 'win32') {
    return unique([
      env.LOCALAPPDATA && win32.join(env.LOCALAPPDATA, 'Google', 'Chrome', 'Application', 'chrome.exe'),
      env.LOCALAPPDATA && win32.join(env.LOCALAPPDATA, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
      env.PROGRAMFILES && win32.join(env.PROGRAMFILES, 'Google', 'Chrome', 'Application', 'chrome.exe'),
      env.PROGRAMFILES && win32.join(env.PROGRAMFILES, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
      env['PROGRAMFILES(X86)'] && win32.join(env['PROGRAMFILES(X86)'], 'Google', 'Chrome', 'Application', 'chrome.exe'),
      env['PROGRAMFILES(X86)'] && win32.join(env['PROGRAMFILES(X86)'], 'Microsoft', 'Edge', 'Application', 'msedge.exe')
    ])
  }
  if (platform === 'darwin') {
    return [
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
      '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
      '/Applications/Chromium.app/Contents/MacOS/Chromium'
    ]
  }
  return ['/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/microsoft-edge']
}

export function processTreeTermination(platform, pid) {
  if (platform === 'win32') return { command: 'taskkill', args: ['/pid', String(pid), '/t', '/f'] }
  return { signal: 'SIGTERM', pid: -pid }
}

function unique(values) {
  return [...new Set(values.filter(Boolean))]
}
