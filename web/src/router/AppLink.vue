<script setup lang="ts">
/* global MouseEvent, HTMLAnchorElement */
import { navigate } from './router'

defineProps<{ to: string }>()

function follow(event: MouseEvent): void {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  navigate((event.currentTarget as HTMLAnchorElement).pathname)
}
</script>

<template>
  <a
    :href="to"
    @click="follow"
  ><slot /></a>
</template>
