<script setup lang="ts">
/* global AbortController, URL */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleDetail, LibraryState } from '../../api/generated/models'
import PermissionBadge from '../../components/shared/PermissionBadge.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import AppLink from '../../router/AppLink.vue'
import LibraryActions from '../feed/LibraryActions.vue'
import { toUserSafeError } from '../../state/errors'
const props = withDefaults(defineProps<{ articleId: string; serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client), detail = ref<ArticleDetail>(), state = ref<'loading'|'ready'|'error'>('loading'), error = ref('')
let controller: AbortController | undefined, generation=0
const publisher = computed(() => { try { const url = new URL(detail.value?.article.canonicalUrl ?? ''); return ['http:','https:'].includes(url.protocol) ? url.href : null } catch { return null } })
onMounted(load); onBeforeUnmount(()=>controller?.abort()); watch(()=>props.articleId,load)
async function load(){const current=++generation;controller?.abort();controller=new AbortController();state.value='loading';error.value='';try{const value=await server.article(props.articleId,controller.signal);if(current!==generation)return;detail.value=value;state.value='ready'}catch(cause){if(current!==generation||controller.signal.aborted)return;error.value=toUserSafeError(cause).message;state.value='error'}}
function reconcile(library:LibraryState){if(detail.value)detail.value.article.library=library}
</script>
<template>
  <section aria-labelledby="reader-title">
    <AppLink to="/">
      Back to ranked feed
    </AppLink><p
      v-if="state==='loading'"
      role="status"
    >
      Loading article…
    </p><div
      v-else-if="state==='error'"
      role="alert"
    >
      <p>{{ error }}</p><button
        type="button"
        @click="load"
      >
        Try again
      </button>
    </div><article v-else-if="detail">
      <PermissionBadge :permission="detail.article.contentPermission" /><h1
        id="reader-title"
        tabindex="-1"
      >
        {{ detail.article.title }}
      </h1><p>{{ detail.article.author }}</p><RankingExplanation
        :contributions="detail.article.ranking.contributions"
        heading-level="h2"
      /><LibraryActions
        :article="detail.article"
        :server="server"
        @updated="reconcile"
      /><div
        v-if="detail.article.contentPermission==='full_content_allowed'"
        class="article-content"
      >
        {{ detail.fullContent || 'No article body was provided by this source.' }}
      </div><div v-else>
        <p>This source permits metadata only. The article body is not stored or displayed here.</p><a
          v-if="publisher"
          :href="publisher"
          target="_blank"
          rel="noopener noreferrer"
        >Read the full article at the publisher</a><p v-else>
          Publisher link unavailable.
        </p>
      </div>
    </article>
  </section>
</template>
<style scoped>.article-content{white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.7}</style>
