<template>
  <div>
    <div class="detail-header">
      <button class="back-link" @click="goBack"><ArrowLeft :size="16" /> {{ route.params.domainID ? '返回受管域名' : '返回证书' }}</button>
      <div class="detail-title-row"><h1 class="page-title">{{ name }}</h1><button class="btn btn-sm" :disabled="loading" @click="loadOperations"><RefreshCw :size="14" />{{ loading ? '刷新中...' : '刷新' }}</button></div>
    </div>
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <SurfaceCard v-if="loaded && operations.length"><div class="table-wrap"><table class="data-table"><thead><tr><th>资源</th><th>名称</th><th>域名</th><th>挑战</th><th>状态</th><th>原因</th><th>创建时间</th></tr></thead><tbody><tr v-for="item in operations" :key="item.kind + item.name"><td class="cell-primary">{{ item.kind }}</td><td>{{ item.name }}</td><td>{{ item.domain || '-' }}</td><td>{{ item.type || '-' }}</td><td><span class="badge" :class="statusClass(item.status)">{{ item.status }}</span></td><td>{{ item.reason || '-' }}</td><td>{{ item.created_at || '-' }}</td></tr></tbody></table></div></SurfaceCard>
    <EmptyState v-else-if="!loaded" variant="loading" message="加载中..." />
    <EmptyState v-else-if="!error" message="证书尚未生成签发过程资源" />
  </div>
</template>
<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, RefreshCw } from 'lucide-vue-next'
import { getCertificateOperations } from '../../api/certificates.js'
import { getManagedDomainOperations } from '../../api/domains.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
const route = useRoute(), router = useRouter(), operations = ref([]), loaded = ref(false), loading = ref(false), error = ref('')
const name = computed(() => route.params.domainID ? route.query?.hostname || '域名签发过程' : route.params.name || '')
function statusClass(status) { return String(status).toLowerCase() === 'valid' || status === 'Ready' ? 'badge-online' : String(status).toLowerCase() === 'invalid' || status === 'Failed' ? 'badge-danger' : 'badge-deploying' }
const operationResource = useAsyncResource(({ signal }) => route.params.domainID
  ? getManagedDomainOperations(route.params.domainID, { signal })
  : getCertificateOperations(route.params.namespace, route.params.name, { signal }, route.params.clusterID), [])
const operationKey = computed(() => route.params.domainID ? `domain/${route.params.domainID}` : `${route.params.clusterID || ''}/${route.params.namespace || ''}/${route.params.name || ''}`)
function goBack() {
  if (route.params.domainID) {
    const query = Object.fromEntries(['project_id', 'environment_id', 'application_id', 'return_tab'].filter(key => route.query?.[key]).map(key => [key, route.query[key]]))
    router.push({ path: '/applications/domains', query })
    return
  }
  router.push(`/cluster/${route.params.clusterID}/network?tab=certificates`)
}
async function loadOperations() {
  const requestKey = operationKey.value
  loading.value = true
  error.value = ''
  const result = await operationResource.refresh()
  if (requestKey !== operationKey.value) return
  if (result) operations.value = Array.isArray(result) ? result : []
  else if (operationResource.error.value) error.value = operationResource.error.value.message || '加载签发过程失败'
  loaded.value = true
  loading.value = false
}
watch(operationKey, () => { operations.value = []; loaded.value = false; loadOperations() }, { immediate: true })
</script>
<style scoped>.detail-header{display:flex;flex-direction:column;gap:var(--space-12);margin-bottom:var(--space-16)}.detail-title-row{display:flex;align-items:center;justify-content:space-between;gap:var(--space-12)}.detail-title-row .page-title{min-width:0;overflow-wrap:anywhere}.back-link{display:inline-flex;width:max-content;align-items:center;gap:6px;border:0;background:transparent;color:var(--text-secondary);cursor:pointer;font:inherit;padding:0}.back-link:hover{color:var(--accent)}</style>
