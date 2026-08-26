<script setup lang="ts">
import { computed } from 'vue'
import type { RankingConfiguration } from '../../api/generated/models'
import DisclosurePanel from '../../components/shared/DisclosurePanel.vue'
import AccessibleField from '../../components/shared/AccessibleField.vue'
import { applyRankingPreset, detectRankingPreset, signalNames, type RankingForm, type RankingPreset, type SignalName } from '../profile/profile-form'

const props = defineProps<{ modelValue: RankingForm; limits: Pick<RankingConfiguration, 'perDemographicCap' | 'totalDemographicCap'>; errors: Record<string, string> }>()
const emit = defineEmits<{ 'update:modelValue': [value: RankingForm] }>()
const selected = computed(() => detectRankingPreset(props.modelValue))
const presets: { id: Exclude<RankingPreset, 'custom'>; title: string; description: string }[] = [
  { id: 'balanced', title: 'Balanced', description: 'A thoughtful mix of your interests and recent reporting. Recommended.' },
  { id: 'personalized', title: 'More personalized', description: 'Give your interests and preferred sources more influence.' },
  { id: 'recent', title: 'More recent', description: 'Prefer newer reporting while still using your preferences.' }
]
const labels: Record<SignalName, string> = { recency: 'Freshness', interest: 'Interests', sourcePreference: 'Preferred sources', behavior: 'Reading activity', location: 'Location', age: 'Age', gender: 'Gender', textSimilarity: 'Local text match' }
function choose(preset: Exclude<RankingPreset, 'custom'>) { emit('update:modelValue', applyRankingPreset(props.modelValue, preset)) }
function update(name: SignalName, key: 'enabled' | 'weight', value: boolean | string) { emit('update:modelValue', { ...props.modelValue, [name]: { ...props.modelValue[name], [key]: value } }) }
</script>

<template>
  <section
    class="ranking-preferences"
    aria-labelledby="ranking-settings-title"
  >
    <header class="surface__header">
      <h2 id="ranking-settings-title">
        Ranking style
      </h2>
      <p class="surface__description">
        Choose the kind of balance you want. Ranking stays deterministic and runs only on this computer.
      </p>
    </header>
    <fieldset class="preset-grid">
      <legend class="sr-only">
        Ranking style
      </legend>
      <label
        v-for="preset in presets"
        :key="preset.id"
        :class="['preset-card',{ 'preset-card--selected': selected === preset.id }]"
      >
        <input
          type="radio"
          name="ranking-preset"
          :value="preset.id"
          :checked="selected === preset.id"
          @change="choose(preset.id)"
        >
        <span><strong>{{ preset.title }}</strong><small>{{ preset.description }}</small></span>
      </label>
      <p
        v-if="selected === 'custom'"
        class="custom-note"
      >
        Your saved custom configuration is preserved. Choose a style only if you want to replace its relative weights.
      </p>
    </fieldset>
    <DisclosurePanel summary="Advanced ranking controls">
      <p class="field__description">
        These are relative influence values. They do not need to add up to 1. Optional demographic signals remain strictly capped.
      </p>
      <p class="cap-note">
        Location, age, and gender each contribute at most {{ limits.perDemographicCap }}, and together at most {{ limits.totalDemographicCap }}.
      </p>
      <div class="advanced-grid">
        <div
          v-for="name in signalNames"
          :key="name"
          class="advanced-row"
        >
          <label><input
            type="checkbox"
            :checked="modelValue[name].enabled"
            @change="update(name,'enabled',($event.target as HTMLInputElement).checked)"
          > Use {{ labels[name] }}</label>
          <AccessibleField
            :id="`ranking-${name}`"
            :label="`${labels[name]} relative influence`"
            description="From 0 (none) to 1 (maximum)."
            :error="errors[`${name}.weight`] || errors[`ranking.${name}.weight`]"
          >
            <template #default="{ describedby }">
              <input
                :id="`ranking-${name}`"
                :value="modelValue[name].weight"
                type="number"
                min="0"
                max="1"
                step="0.05"
                :aria-describedby="describedby"
                :aria-invalid="Boolean(errors[`${name}.weight`] || errors[`ranking.${name}.weight`])"
                @input="update(name,'weight',($event.target as HTMLInputElement).value)"
              >
            </template>
          </AccessibleField>
        </div>
      </div>
    </DisclosurePanel>
  </section>
</template>

<style scoped>
.ranking-preferences{display:grid;gap:var(--space-4)}.preset-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--space-3);padding:0;border:0}.preset-card{display:flex;align-items:flex-start;gap:var(--space-3);min-height:7.5rem;padding:var(--space-4);border:1px solid var(--color-border);border-radius:var(--radius-lg);cursor:pointer;background:var(--color-surface)}.preset-card:hover,.preset-card--selected{border-color:var(--color-brand);background:var(--color-brand-soft)}.preset-card span{display:grid;gap:var(--space-1)}.preset-card small{color:var(--color-muted);line-height:1.45}.custom-note,.cap-note{grid-column:1/-1;padding:var(--space-3);border-radius:var(--radius-md);background:var(--color-warning-soft)}.advanced-grid{display:grid;gap:var(--space-3)}.advanced-row{display:grid;grid-template-columns:minmax(12rem,1fr) minmax(14rem,1fr);gap:var(--space-4);align-items:end;padding:var(--space-3);border-radius:var(--radius-md);background:var(--color-surface-soft)}@media(max-width:48rem){.preset-grid{grid-template-columns:1fr}.preset-card{min-height:0}.advanced-row{grid-template-columns:1fr}}
</style>
