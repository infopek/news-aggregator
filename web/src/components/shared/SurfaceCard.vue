<script setup lang="ts">
withDefaults(defineProps<{ title?: string; description?: string; soft?: boolean }>(), { title: '', description: '', soft: false })
</script>

<template>
  <section :class="['surface', { 'surface--soft': soft }]">
    <header
      v-if="title || description"
      class="surface__header"
    >
      <h2 v-if="title">
        {{ title }}
      </h2>
      <p
        v-if="description"
        class="surface__description"
      >
        {{ description }}
      </p>
    </header>
    <slot />
  </section>
</template>
