<script setup lang="ts">
import type { ScoreContribution } from '../../api/generated/models'
import { useId } from 'vue'
withDefaults(defineProps<{ contributions: ScoreContribution[]; headingLevel?: 'h2' | 'h3' | 'h4' }>(), { headingLevel: 'h4' })
const titleId = `ranking-title-${useId()}`
const reasons: Record<string, string> = {
  recent_publication: 'Published recently', interest_match: 'Matches an interest', preferred_source: 'From a preferred source',
  behavior_match: 'Similar to reading activity', location_match: 'Relevant to the optional location signal',
  age_match: 'Influenced slightly by the optional age signal', gender_match: 'Influenced slightly by the optional gender signal',
  text_similarity: 'Text is similar to selected interests',
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
