<template>
  <div>
    <router-link class="back-link" to="/applications"><ArrowLeft :size="16" />返回应用列表</router-link>
    <div class="page-header"><div><h1 class="page-title">{{ application?.name || '应用详情' }}</h1><p class="page-subtitle">{{ application?.project?.name || '-' }} · {{ application?.environment?.name || '-' }} · {{ application?.environment?.namespace || '-' }}</p></div></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>
    <div v-if="application && releases.length > 0" class="card"><div class="table-wrap"><table class="data-table"><thead><tr><th>版本</th><th>镜像</th><th>状态</th><th>时间</th></tr></thead><tbody><tr v-for="release in releases" :key="release.id" class="clickable" @click="openRelease(release)"><td class="cell-primary">Release #{{ release.sequence }}</td><td>{{ release.image }}</td><td><span class="badge" :class="releaseBadge(release.status)">{{ release.status }}</span></td><td>{{ formatTime(release.created_at) }}</td></tr></tbody></table></div></div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { api } from '../api/index.js'

const props = defineProps({ applicationID: { type: String, required: true } })
const router = useRouter()
const application = ref(null)
const releases = ref([])
const error = ref('')

function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
async function loadApplication() { error.value = ''; try { const result = await api.get(`/applications/${props.applicationID}`); application.value = result.application; releases.value = result.releases || [] } catch (e) { error.value = e.message || '加载应用详情失败' } }
async function openRelease(release) { await router.push(`/applications/${props.applicationID}/releases/${release.id}`) }

watch(() => props.applicationID, loadApplication, { immediate: true })
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin-bottom:var(--space-16); color:var(--text-secondary); font-size:13px; font-weight:600; text-decoration:none; }.back-link:hover { color:var(--action-primary); }.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }.page-header { margin-bottom:var(--space-24); }.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
</style>
