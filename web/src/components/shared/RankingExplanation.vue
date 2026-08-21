<script setup lang="ts">
import type { ScoreContribution } from '../../api/generated/models'
import { useId } from 'vue'
withDefaults(defineProps<{ contributions: ScoreContribution[]; headingLevel?: 'h2' | 'h3' | 'h4' }>(), { headingLevel: 'h4' })
const titleId = `ranking-title-${useId()}`
const reasons: Record<string, string> = {
  recency_fresh: 'Published recently', explicit_interest_match: 'Matches an explicit interest', explicit_source_preference: 'From an explicitly preferred source',
  article_read: 'Adjusted using your read history', article_saved: 'Adjusted because you saved this article',
  explicit_location_match: 'Matches your optional location and declared article metadata',
  explicit_age_adjustment: 'Matches your optional age and a declared audience range', explicit_gender_adjustment: 'Matches your optional gender and a declared audience value',
  local_text_match: 'Local text is similar to selected interests', neutral_default: 'No active signal changed this article’s score',
}
</script>
<template>
  <div
    class="ranking"
    :aria-labelledby="titleId"
  >
    <component
      :is="headingLevel"
      :id="titleId"
    >
      Why this was ranked here
    </component>
    <p v-if="!contributions.length">
      No ranking explanation was provided.
    </p>
    <ul v-else>
      <li
        v-for="(item, index) in contributions"
        :key="`${item.signal}-${index}`"
      >
        <span>{{ reasons[item.reasonCode] ?? 'Another ranking signal contributed' }}</span>
        <output :aria-label="`Score contribution ${item.weightedScore}`">{{ item.weightedScore }}</output>
      </li>
    </ul>
    <p class="context">
      These are contributions reported by the ranking service, not guarantees about why you will find an article relevant.
    </p>
  </div>
</template>
