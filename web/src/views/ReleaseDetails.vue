<template>
  <div>
    <router-link class="back-link" :to="`/applications/${applicationID}`"><ArrowLeft :size="16" />返回 {{ application?.name || '应用详情' }}</router-link>
    <div class="page-header"><div><h1 class="page-title">{{ release ? `Release #${release.sequence}` : '发布详情' }}</h1><p class="page-subtitle">{{ release?.image || '-' }}</p></div><div v-if="release" class="btn-group"><button v-if="release.status === 'failed'" class="btn" @click="retryRelease">重试</button><button class="btn" @click="rollbackRelease">回滚</button></div></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>
    <div v-if="release && release.operations?.length" class="card"><div class="table-wrap"><table class="data-table"><thead><tr><th>步骤</th><th>状态</th><th>详情</th><th>开始时间</th></tr></thead><tbody><tr v-for="operation in release.operations" :key="operation.id"><td class="cell-primary">{{ operation.step }}</td><td><span class="badge" :class="operationBadge(operation.status)">{{ operation.status }}</span></td><td>{{ operation.detail || '-' }}</td><td>{{ formatTime(operation.started_at) }}</td></tr></tbody></table></div></div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { api } from '../api/index.js'

const props = defineProps({ applicationID: { type: String, required: true }, releaseID: { type: String, required: true } })
const router = useRouter()
const application = ref(null)
const release = ref(null)
const error = ref('')
let poller = null

function operationBadge(status) { return status === 'success' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
function isTerminal(status) { return ['succeeded', 'failed', 'rolled_back'].includes(status) }
function stopPolling() { if (poller) { window.clearInterval(poller); poller = null } }
function startPolling() { stopPolling(); if (!release.value || isTerminal(release.value.status)) return; poller = window.setInterval(loadRelease, 2000) }
async function loadRelease() { error.value = ''; try { const [applicationResult, releaseResult] = await Promise.all([api.get(`/applications/${props.applicationID}`), api.get(`/applications/${props.applicationID}/releases/${props.releaseID}`)]); application.value = applicationResult.application; release.value = releaseResult; startPolling() } catch (e) { error.value = e.message || '加载发布详情失败'; stopPolling() } }
async function retryRelease() { try { const next = await api.post(`/applications/${props.applicationID}/releases/${props.releaseID}/retry`); await router.push(`/applications/${props.applicationID}/releases/${next.id}`) } catch (e) { error.value = e.message || '重试失败' } }
async function rollbackRelease() { try { const next = await api.post(`/applications/${props.applicationID}/releases/${props.releaseID}/rollback`); await router.push(`/applications/${props.applicationID}/releases/${next.id}`) } catch (e) { error.value = e.message || '回滚失败' } }

watch(() => [props.applicationID, props.releaseID], loadRelease, { immediate: true })
onBeforeUnmount(stopPolling)
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin-bottom:var(--space-16); color:var(--text-secondary); font-size:13px; font-weight:600; text-decoration:none; }.back-link:hover { color:var(--action-primary); }.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); margin-bottom:var(--space-24); }.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; } }
</style>
