<script setup lang="ts">
import { ref } from 'vue'
import type { ArticleSummary, LibraryState } from '../../api/generated/models'
import type { ServerApi } from '../../api/server-api'
import LiveRegion from '../../components/shared/LiveRegion.vue'
import { toUserSafeError } from '../../state/errors'

const props = defineProps<{ article: ArticleSummary; server: ServerApi }>()
const emit = defineEmits<{ updated: [LibraryState]; hidden: [] }>()
const busy = ref(false), message = ref('')
async function update(field: 'read'|'saved'|'hidden', value: boolean) {
  busy.value = true; message.value = ''
  try {
    const state = await props.server.updateLibrary(props.article.id, { [field]: value })
    emit('updated', state); if (field === 'hidden' && value) emit('hidden')
    message.value = `${field === 'saved' ? 'Saved state' : field === 'read' ? 'Read state' : 'Hidden state'} updated.`
  } catch (cause) { message.value = toUserSafeError(cause).message } finally { busy.value = false }
}
</script>
<template>
  <div
    class="actions"
    aria-label="Article actions"
  >
    <button
      type="button"
      :disabled="busy"
      @click="update('read', !article.library.readAt)"
    >
      {{ article.library.readAt ? 'Mark unread' : 'Mark read' }}
    </button>
    <button
      type="button"
      :disabled="busy"
      @click="update('saved', !article.library.savedAt)"
    >
      {{ article.library.savedAt ? 'Unsave' : 'Save' }}
    </button>
    <button
      type="button"
      :disabled="busy"
      @click="update('hidden', true)"
    >
      Hide
    </button>
    <LiveRegion
      :message="message"
      :urgent="message.includes('unavailable')"
    />
  </div>
</template>
