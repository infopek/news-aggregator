import type { Source, SourceWrite } from '../../api/generated/models'

export interface SourceForm {
  id?: string
  name: string
  url: string
  kind: Source['kind']
  enabled: boolean
  contentPermission: Source['contentPermission']
  apiProvider: string
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
  return { name: '', url: '', kind: 'feed', enabled: true, contentPermission: 'metadata_only', apiProvider: '', articleSelector: 'article', titleSelector: 'h1', excerptSelector: '', contentSelector: '', policyStatus: 'pending', termsUrl: '', robotsUrl: '', reviewedAt: '', reviewNotes: '' }
}

export function sourceForm(source: Source): SourceForm {
  const config = source.adapterConfig as Record<string, unknown>
  return {
    id: source.id, name: source.name, url: source.url, kind: source.kind, enabled: source.enabled, contentPermission: source.contentPermission,
    apiProvider: String(config.provider ?? ''), articleSelector: String(config.articleSelector ?? 'article'), titleSelector: String(config.titleSelector ?? 'h1'), excerptSelector: String(config.excerptSelector ?? ''), contentSelector: String(config.contentSelector ?? ''),
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
  if (form.kind === 'feed') return { ...common, kind: 'feed', adapterConfig: { format: 'rss' }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null } }
  if (form.kind === 'api') return { ...common, kind: 'api', adapterConfig: { provider: form.apiProvider.trim(), pageSize: 50 }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null } }
  const scraperPolicy = { status: form.policyStatus, termsUrl: form.termsUrl.trim() || null, robotsUrl: form.robotsUrl.trim() || null, reviewedAt: form.reviewedAt ? new Date(form.reviewedAt).toISOString() : null, reviewNotes: form.reviewNotes.trim() || null }
  return { ...common, kind: 'scraper', adapterConfig: { articleSelector: form.articleSelector.trim(), titleSelector: form.titleSelector.trim(), excerptSelector: form.excerptSelector.trim() || undefined, contentSelector: form.contentSelector.trim() || undefined }, scraperPolicy } as SourceWrite
}

export function validateSource(form: SourceForm): string[] {
  const errors: string[] = []
  if (!form.name.trim()) errors.push('Enter a source name.')
  try { const url = new URL(form.url); if (url.protocol !== 'https:' && url.protocol !== 'http:') errors.push('Use an HTTP or HTTPS source URL.') } catch { errors.push('Enter a valid source URL.') }
  if (form.kind === 'api' && !form.apiProvider.trim()) errors.push('Enter the official API provider identifier.')
  if (form.kind === 'scraper') {
    if (!form.articleSelector.trim() || !form.titleSelector.trim()) errors.push('Scrapers require article and title selectors.')
    if (form.enabled && (form.policyStatus !== 'approved' || !form.reviewedAt)) errors.push('An enabled scraper requires an approved, dated policy review.')
  }
  return errors
}
