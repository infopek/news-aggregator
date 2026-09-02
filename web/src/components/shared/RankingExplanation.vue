<script setup lang="ts">
import type { ScoreContribution } from '../../api/generated/models'

withDefaults(defineProps<{ contributions: ScoreContribution[]; headingLevel?: 'h2' | 'h3' | 'h4' }>(), { headingLevel: 'h3' })

const reasons: Record<string, string> = {
  recency_fresh: 'Published recently',
  explicit_interest_match: 'Matches one of your interests',
  explicit_source_preference: 'Comes from a source you prefer',
  article_read: 'Adjusted using your reading history',
  article_saved: 'Adjusted because you saved this story',
  explicit_location_match: 'Relevant to your optional location',
  explicit_age_adjustment: 'Matches an optional publisher-declared age range',
  explicit_gender_adjustment: 'Matches an optional publisher-declared audience',
  local_text_match: 'Similar to topics you follow',
  neutral_default: 'No active preference changed this story’s position',
}
</script>

<template>
  <details class="ranking">
    <summary>Why this story?</summary>
    <div class="ranking__content">
      <p
        v-if="!contributions.length"
        class="ranking__empty"
      >
        No ranking explanation was provided.
      </p>
      <ul
        v-else
        class="ranking__reasons"
      >
        <li
          v-for="(item, index) in contributions"
          :key="`${item.signal}-${index}`"
        >
          {{ reasons[item.reasonCode] ?? 'Another local ranking signal contributed' }}
        </li>
      </ul>
      <p class="ranking__context">
        These local signals help order your feed, not guarantees of what you will enjoy.
      </p>
      <details
        v-if="contributions.length"
        class="ranking__technical"
      >
        <summary>Technical details</summary>
        <dl>
          <template
            v-for="(item, index) in contributions"
            :key="`${item.signal}-detail-${index}`"
          >
            <dt>{{ item.signal }}</dt>
            <dd>{{ item.weightedScore }}</dd>
          </template>
        </dl>
      </details>
    </div>
  </details>
</template>

<style scoped>
.ranking { border: 0; background: transparent; }
.ranking > summary { width: fit-content; min-height: 2.75rem; display: inline-flex; align-items: center; color: var(--color-brand); cursor: pointer; font-weight: 720; }
.ranking > summary:hover { color: var(--color-brand-strong); }
.ranking__content { display: grid; gap: var(--space-3); margin-top: var(--space-2); padding: var(--space-4); border-radius: var(--radius-md); background: var(--color-surface-soft); }
.ranking__reasons { display: grid; gap: var(--space-2); margin: 0; padding-inline-start: 1.25rem; }
.ranking__context, .ranking__empty { color: var(--color-muted); font-size: .9rem; }
.ranking__technical summary { color: var(--color-muted); cursor: pointer; font-size: .88rem; font-weight: 680; }
.ranking__technical dl { display: grid; grid-template-columns: minmax(7rem, 1fr) auto; gap: var(--space-1) var(--space-4); margin: var(--space-2) 0 0; color: var(--color-muted); font-size: .82rem; }
.ranking__technical dd { margin: 0; font-variant-numeric: tabular-nums; }
</style>
