import { createApp, defineComponent, h } from 'vue'
import '../src/styles/app.css'
import '../src/styles/primitives.css'
import { ArticleSummaryCard, AsyncState, CredentialInput, DemographicSignalField, RankingExplanation, RefreshStatus } from '../src/components/shared'
import { articleSummary, contribution, partialRefresh } from '../src/testing/fixtures'

const malicious = '<img src=x onerror=alert(1)> مرحبا 世界 — an intentionally very long Unicode headline that demonstrates safe wrapping without trusting publisher markup'
const App = defineComponent({
  setup: () => () => h('main', { class: 'proof' }, [
    h('h1', 'Shared primitive variants'),
    h('div', { class: 'proof-grid' }, [
      h(ArticleSummaryCard, { article: articleSummary({ title: malicious, excerpt: '<script>untrusted()</script>', language: 'ar' }), sourceName: 'ناشر محلي' }),
      h(RankingExplanation, { contributions: [contribution(), contribution({ signal: 'recency', reasonCode: 'future_v9', weightedScore: -0.125 })] }),
      h(RefreshStatus, { refresh: partialRefresh() }),
      h(DemographicSignalField, { id: 'proof-age', label: 'Age', modelValue: '', enabled: false }),
      h(CredentialInput, { id: 'proof-secret' }),
      h(AsyncState, { state: 'empty' }),
    ]),
  ]),
})
createApp(App).mount('#app')
