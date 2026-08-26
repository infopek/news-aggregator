import { createApp, defineComponent, h } from 'vue'
import '../src/styles/app.css'
import {
  ActionGroup,
  ArticleSummaryCard,
  AsyncState,
  CredentialInput,
  DemographicSignalField,
  DisclosurePanel,
  EmptyState,
  RankingExplanation,
  RefreshStatus,
  StatusBanner,
  SurfaceCard,
  TagChip,
} from '../src/components/shared'
import { articleSummary, contribution, partialRefresh } from '../src/testing/fixtures'

const malicious = '<img src=x onerror=alert(1)> مرحبا 世界 — an intentionally very long Unicode headline that demonstrates safe wrapping without trusting publisher markup'
const App = defineComponent({
  setup: () => () => h('main', { class: 'proof' }, [
    h('h1', 'Shared primitive variants'),
    h('p', { class: 'surface__description' }, 'A quiet, reusable foundation for the local news reader.'),
    h('div', { class: 'proof-grid' }, [
      h(SurfaceCard, { title: 'Personalization', description: 'Choose topics that matter to you.' }, () => [
        h('div', { class: 'action-group' }, [h(TagChip, { label: 'Science' }), h(TagChip, { label: 'Local news', removable: true })]),
        h(DisclosurePanel, { summary: 'Advanced controls' }, () => h('p', 'Fine-tune relative influence only when you need it.')),
      ]),
      h(StatusBanner, { title: 'Refresh complete', tone: 'success', live: true }, () => h('p', '12 new stories are ready.')),
      h(ArticleSummaryCard, { article: articleSummary({ title: malicious, excerpt: '<script>untrusted()</script>', language: 'ar' }), sourceName: 'ناشر محلي' }),
      h(RankingExplanation, { contributions: [contribution(), contribution({ signal: 'recency', reasonCode: 'future_v9', weightedScore: -0.125 })] }),
      h(RefreshStatus, { refresh: partialRefresh() }),
      h(DemographicSignalField, { id: 'proof-age', label: 'Age', modelValue: '', enabled: false }),
      h(CredentialInput, { id: 'proof-secret' }),
      h(AsyncState, { state: 'empty' }),
      h(EmptyState, { title: 'Nothing saved yet', description: 'Save stories from your feed to read them later.' }, () => h(ActionGroup, { label: 'Empty library actions' }, () => h('a', { class: 'button-link', href: '/' }, 'Browse stories'))),
    ]),
  ]),
})
createApp(App).mount('#app')
