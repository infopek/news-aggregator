<script setup lang="ts">
import type { ServerApi } from '../../api/server-api'
import { navigate } from '../../router/router'
import ProfileSettings from '../profile/ProfileSettings.vue'
defineProps<{ serverApi?: ServerApi }>()
function finish(firstRun: boolean) { if (firstRun) navigate('/') }
</script>
<template>
  <ProfileSettings
    mode="setup"
    :server-api="serverApi"
    @saved="finish"
  />
</template>
