import type { Source, SourceWrite } from '../../api/generated/models'

export interface SourceForm {
  id?: string
  name: string
  url: string
  kind: Source['kind']
  enabled: boolean
  contentPermission: Source['contentPermission']
  feedFormat: 'auto' | 'rss' | 'atom'
  apiProvider: string
  apiPageSize: string
  articleSelector: string
  titleSelector: string
  excerptSelector: string
  contentSelector: string
  policyStatus: Source['scraperPolicy']['status']
  termsUrl: string
  robotsUrl: string
  reviewedAt: string
  reviewNotes: string
}

export function emptySource(): SourceForm {
  return { name: '', url: '', kind: 'feed', enabled: true, contentPermission: 'metadata_only', feedFormat: 'auto', apiProvider: '', apiPageSize: '50', articleSelector: 'article', titleSelector: 'h1', excerptSelector: '', contentSelector: '', policyStatus: 'pending', termsUrl: '', robotsUrl: '', reviewedAt: '', reviewNotes: '' }
}

export function sourceForm(source: Source): SourceForm {
  const config = source.adapterConfig as Record<string, unknown>
  return {
    id: source.id, name: source.name, url: source.url, kind: source.kind, enabled: source.enabled, contentPermission: source.contentPermission,
    feedFormat: (config.format as SourceForm['feedFormat']) ?? 'auto', apiProvider: String(config.provider ?? ''), apiPageSize: String(config.pageSize ?? 50), articleSelector: String(config.articleSelector ?? 'article'), titleSelector: String(config.titleSelector ?? 'h1'), excerptSelector: String(config.excerptSelector ?? ''), contentSelector: String(config.contentSelector ?? ''),
    policyStatus: source.scraperPolicy.status, termsUrl: source.scraperPolicy.termsUrl ?? '', robotsUrl: source.scraperPolicy.robotsUrl ?? '', reviewedAt: source.scraperPolicy.reviewedAt ? localDateTime(source.scraperPolicy.reviewedAt) : '', reviewNotes: source.scraperPolicy.reviewNotes ?? ''
  }
}

export function localDateTime(instant: string): string {
  const date = new Date(instant)
  const part = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())}T${part(date.getHours())}:${part(date.getMinutes())}`
}

export function sourceWrite(form: SourceForm): SourceWrite {
  const common = { name: form.name.trim(), url: form.url.trim(), enabled: form.enabled, contentPermission: form.contentPermission }
  if (form.kind === 'feed') return { ...common, kind: 'feed', adapterConfig: { format: form.feedFormat }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null } }
  if (form.kind === 'api') return { ...common, kind: 'api', adapterConfig: { provider: form.apiProvider.trim(), pageSize: Number(form.apiPageSize) }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null } }
  const scraperPolicy = { status: form.policyStatus, termsUrl: form.termsUrl.trim() || null, robotsUrl: form.robotsUrl.trim() || null, reviewedAt: form.reviewedAt ? new Date(form.reviewedAt).toISOString() : null, reviewNotes: form.reviewNotes.trim() || null }
  return { ...common, kind: 'scraper', adapterConfig: { articleSelector: form.articleSelector.trim(), titleSelector: form.titleSelector.trim(), excerptSelector: form.excerptSelector.trim() || undefined, contentSelector: form.contentSelector.trim() || undefined }, scraperPolicy } as SourceWrite
}

export function validateSource(form: SourceForm): string[] {
  const errors: string[] = []
  if (!form.name.trim()) errors.push('Enter a source name.')
  try { const url = new URL(form.url); if (url.protocol !== 'https:' && url.protocol !== 'http:') errors.push('Use an HTTP or HTTPS source URL.') } catch { errors.push('Enter a valid source URL.') }
  if (form.kind === 'api') {
    if (!form.apiProvider.trim()) errors.push('Enter the official API provider identifier.')
    if (!Number.isInteger(Number(form.apiPageSize)) || Number(form.apiPageSize) < 0) errors.push('API page size must be a whole number of 0 or more.')
  }
  if (form.kind === 'scraper') {
    if (!form.articleSelector.trim() || !form.titleSelector.trim()) errors.push('Scrapers require article and title selectors.')
    if (form.enabled && form.policyStatus !== 'approved') errors.push('An enabled scraper requires an approved policy review.')
    if (form.policyStatus === 'approved') {
      if (!form.reviewedAt) errors.push('An approved scraper requires a review timestamp.')
      if (!validURL(form.termsUrl)) errors.push('An approved scraper requires a valid HTTP or HTTPS Terms URL.')
      if (!validURL(form.robotsUrl)) errors.push('An approved scraper requires a valid HTTP or HTTPS robots URL.')
      if (!form.reviewNotes.trim()) errors.push('An approved scraper requires review notes.')
    }
  }
  return errors
}

function validURL(raw: string): boolean {
  try { const url = new URL(raw); return (url.protocol === 'http:' || url.protocol === 'https:') && Boolean(url.host) } catch { return false }
}
