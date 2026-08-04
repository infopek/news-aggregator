<script setup lang="ts">
import type { ServerApi } from '../../api/server-api'
import ProfileSettings from '../profile/ProfileSettings.vue'
defineProps<{ serverApi?: ServerApi }>()
</script>
<template>
  <ProfileSettings
    mode="setup"
    :server-api="serverApi"
  />
</template>
