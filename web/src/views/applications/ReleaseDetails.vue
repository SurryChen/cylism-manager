<template>
  <div>
    <router-link class="back-link" :to="`/applications/${applicationID}`"><ArrowLeft :size="16" />返回 {{ application?.name || '应用详情' }}</router-link>
    <div class="page-header"><div><h1 class="page-title">{{ release ? `Release #${release.sequence}` : '发布详情' }}</h1><p class="page-subtitle">{{ release?.image || '-' }}</p></div><div v-if="release" class="btn-group"><button v-if="release.status === 'failed'" class="btn" @click="retryRelease">重试</button><button class="btn" @click="rollbackRelease">回滚</button></div></div>
    <div v-if="readError" data-testid="release-read-error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ readError }}</div>
    <div v-if="mutationError" data-testid="release-mutation-error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ mutationError }}</div>
    <SurfaceCard v-if="release && release.operations?.length" as="div"><div class="table-wrap"><table class="data-table"><thead><tr><th>步骤</th><th>状态</th><th>详情</th><th>开始时间</th></tr></thead><tbody><tr v-for="operation in release.operations" :key="operation.id"><td class="cell-primary">{{ operation.step }}</td><td><span class="badge" :class="operationBadge(operation.status)">{{ operation.status }}</span></td><td>{{ operation.detail || '-' }}</td><td>{{ formatTime(operation.started_at) }}</td></tr></tbody></table></div></SurfaceCard>
    <section v-if="release?.runtime" class="runtime-section"><div class="section-heading"><div><h2>当前运行状态</h2><p v-if="release.runtime.tracking === 'exact'">实时读取此发布关联 Pod 的 Kubernetes 状态</p><p v-else>无法建立实时 Pod 关联</p></div><span class="badge" :class="runtimeBadge(release.runtime)">{{ runtimeLabel(release.runtime) }}</span></div><div v-if="release.runtime.diagnostic" class="k8s-banner k8s-banner-warn runtime-diagnostic">{{ release.runtime.diagnostic }}</div><div v-if="release.runtime.pods?.length" class="table-wrap"><table class="data-table"><thead><tr><th>Pod</th><th>节点</th><th>状态</th><th>重启</th><th>容器</th><th>上次退出</th></tr></thead><tbody><tr v-for="pod in release.runtime.pods" :key="pod.name"><td class="cell-primary">{{ pod.name }}</td><td>{{ pod.node_name || '-' }}</td><td><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-danger'">{{ pod.ready ? 'Ready' : pod.phase || 'Unknown' }}</span></td><td>{{ pod.restarts }}</td><td><div v-for="container in pod.containers" :key="container.name" class="container-state"><strong>{{ container.name }}</strong><span>{{ container.reason || container.state || '-' }}</span></div></td><td><div v-for="container in pod.containers" :key="`${container.name}-exit`" class="container-state"><strong>{{ container.last_reason || '-' }}</strong><span v-if="container.last_exit_code !== undefined">退出码 {{ container.last_exit_code }}</span></div></td></tr></tbody></table></div><div v-else class="empty-inline">{{ release.runtime.tracking === 'exact' ? '当前没有与此发布标签关联的 Pod。' : release.runtime.diagnostic || '暂无运行态信息。' }}</div></section>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { getApplication, getApplicationRelease, retryRelease as retryReleaseRequest, rollbackRelease as rollbackReleaseRequest } from '../../api/applications.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatDateTime as formatTime } from '../../utils/formatters.js'

const props = defineProps({ applicationID: { type: String, required: true }, releaseID: { type: String, required: true } })
const router = useRouter()
const application = ref(null)
const release = ref(null)
const readError = ref('')
const mutationError = ref('')
const releasePolling = usePolling(loadRelease, { interval: 2000 })
const releaseResource = useAsyncResource(async ({ signal }) => {
  const [applicationResult, releaseResult] = await Promise.all([
    getApplication(props.applicationID, { signal }),
    getApplicationRelease(props.applicationID, props.releaseID, { signal }),
  ])
  return { applicationResult, releaseResult }
}, null)

function operationBadge(status) { return status === 'success' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function runtimeHealthy(runtime) { return runtime.tracking === 'exact' && runtime.pods?.length > 0 && runtime.pods.every(pod => pod.ready) && !runtime.diagnostic }
function runtimeBadge(runtime) { if (runtime.tracking !== 'exact') return 'badge-offline'; return runtimeHealthy(runtime) ? 'badge-online' : 'badge-danger' }
function runtimeLabel(runtime) { if (runtime.tracking === 'legacy_untracked') return '历史发布未关联'; if (runtime.tracking === 'unavailable') return '运行态不可用'; if (!runtime.pods?.length) return '暂无 Pod'; return runtimeHealthy(runtime) ? '运行正常' : '需要处理' }
function stopPolling() { releasePolling.stop() }
function startPolling() { if (!release.value) return; releasePolling.stop(); releasePolling.start({ interval: ['succeeded', 'failed', 'rolled_back'].includes(release.value.status) ? 5000 : 2000 }) }
async function loadRelease() { readError.value = ''; const result = await releaseResource.refresh(); if (result) { application.value = result.applicationResult.application; release.value = result.releaseResult; startPolling() } else if (releaseResource.error.value) { readError.value = releaseResource.error.value.message || '加载发布详情失败'; stopPolling() } }
async function retryRelease() { mutationError.value = ''; try { const next = await retryReleaseRequest(props.applicationID, props.releaseID); await router.push(`/applications/${props.applicationID}/releases/${next.id}`) } catch (e) { mutationError.value = e.message || '重试失败' } }
async function rollbackRelease() { mutationError.value = ''; try { const next = await rollbackReleaseRequest(props.applicationID, props.releaseID); await router.push(`/applications/${props.applicationID}/releases/${next.id}`) } catch (e) { mutationError.value = e.message || '回滚失败' } }

watch(() => [props.applicationID, props.releaseID], loadRelease, { immediate: true })
onBeforeUnmount(stopPolling)
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin-bottom:var(--space-16); color:var(--text-secondary); font-size:13px; font-weight:600; text-decoration:none; }.back-link:hover { color:var(--action-primary); }.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }.page-header,.section-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }.page-header { margin-bottom:var(--space-24); }.page-subtitle,.section-heading p,.empty-inline { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }.runtime-section { padding:var(--space-20) 0; border-top:1px solid var(--border-muted); margin-top:var(--space-20); }.section-heading { margin-bottom:var(--space-12); }.section-heading h2 { margin:0; font-size:16px; }.section-heading p { font-size:12px; }.runtime-diagnostic { margin-bottom:var(--space-12); }.container-state { display:grid; gap:2px; min-width:132px; margin-bottom:6px; font-size:12px; }.container-state:last-child { margin-bottom:0; }.container-state strong { font-weight:600; }.container-state span { color:var(--text-secondary); word-break:break-word; }@media (max-width:640px) { .page-header,.section-heading { align-items:stretch; flex-direction:column; } }
</style>
