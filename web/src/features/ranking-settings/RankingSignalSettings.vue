<script setup lang="ts">
import type { RankingConfiguration } from '../../api/generated/models'
import AccessibleField from '../../components/shared/AccessibleField.vue'
import { signalNames, type RankingForm, type SignalName } from '../profile/profile-form'

defineProps<{ modelValue: RankingForm; limits: Pick<RankingConfiguration, 'perDemographicCap' | 'totalDemographicCap'>; errors: Record<string, string> }>()
const emit = defineEmits<{ 'update:modelValue': [value: RankingForm] }>()
const labels: Record<SignalName, string> = {
  recency: 'Recency', interest: 'Explicit interests', sourcePreference: 'Preferred sources', behavior: 'Local reading activity',
  location: 'Manual location', age: 'Age', gender: 'Gender', textSimilarity: 'Local text similarity'
}
function update(model: RankingForm, name: SignalName, key: 'enabled' | 'weight', value: boolean | string) {
  emit('update:modelValue', { ...model, [name]: { ...model[name], [key]: value } })
}
</script>

<template>
  <fieldset class="signal-settings">
    <legend>Ranking signals</legend>
    <p>Explicit interests and source choices are the primary controls. The server computes ranking; these bounded values only configure its signals.</p>
    <p class="cap-disclosure">
      Age, gender, and location are optional. Each demographic contribution is capped at {{ limits.perDemographicCap }}, and together they are capped at {{ limits.totalDemographicCap }}.
    </p>
    <div
      v-for="name in signalNames"
      :key="name"
      class="signal-row"
    >
      <label><input
        :checked="modelValue[name].enabled"
        type="checkbox"
        @change="update(modelValue, name, 'enabled', ($event.target as HTMLInputElement).checked)"
      > Use {{ labels[name] }}</label>
      <AccessibleField
        :id="`ranking-${name}`"
        :label="`${labels[name]} weight`"
        description="Enter a value from 0 to 1."
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
            @input="update(modelValue, name, 'weight', ($event.target as HTMLInputElement).value)"
          >
        </template>
      </AccessibleField>
    </div>
  </fieldset>
</template>

<style scoped>
.signal-settings,.signal-row{display:grid;gap:.75rem}.signal-settings{border:0;padding:0}.signal-row{grid-template-columns:minmax(13rem,1fr) minmax(12rem,1fr);align-items:end;padding:.75rem;background:var(--surface-muted,#f4f5f6);border-radius:.5rem}.cap-disclosure{border-inline-start:.25rem solid #5966b0;padding:.65rem}.signal-row input[type=number]{width:100%}@media(max-width:42rem){.signal-row{grid-template-columns:1fr}}
</style>
