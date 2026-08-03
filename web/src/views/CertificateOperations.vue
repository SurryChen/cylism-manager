<template>
  <div><div class="detail-header"><button class="back-link" @click="router.push('/network?tab=certificates')"><ArrowLeft :size="16" /> 返回证书</button><div><h1 class="page-title">{{ name }}</h1><p class="page-subtitle">查看 CertificateRequest、Order 与 Challenge 的实际签发状态</p></div></div><div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div><section v-if="loaded && operations.length" class="card"><div class="table-wrap"><table class="data-table"><thead><tr><th>资源</th><th>名称</th><th>域名</th><th>挑战</th><th>状态</th><th>原因</th><th>创建时间</th></tr></thead><tbody><tr v-for="item in operations" :key="item.kind + item.name"><td class="cell-primary">{{ item.kind }}</td><td>{{ item.name }}</td><td>{{ item.domain || '-' }}</td><td>{{ item.type || '-' }}</td><td><span class="badge" :class="statusClass(item.status)">{{ item.status }}</span></td><td>{{ item.reason || '-' }}</td><td>{{ item.created_at || '-' }}</td></tr></tbody></table></div></section><div v-else-if="loaded && !error" class="empty-state"><FileSearch :size="30" :stroke-width="1.5" /><span class="empty-text">证书尚未生成签发过程资源</span></div></div>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, FileSearch } from 'lucide-vue-next'
import { api } from '../api/index.js'
const route = useRoute(), router = useRouter(), operations = ref([]), loaded = ref(false), error = ref(''), name = route.params.name
function statusClass(status) { return String(status).toLowerCase() === 'valid' || status === 'Ready' ? 'badge-online' : String(status).toLowerCase() === 'invalid' || status === 'Failed' ? 'badge-danger' : 'badge-deploying' }
onMounted(async () => { try { operations.value = await api.get(`/certs/${route.params.namespace}/${route.params.name}/operations`) || [] } catch (e) { error.value = e.message || '加载签发过程失败' } finally { loaded.value = true } })
</script>
<style scoped>.detail-header{display:flex;flex-direction:column;gap:var(--space-12)}.back-link{display:inline-flex;width:max-content;align-items:center;gap:6px;border:0;background:transparent;color:var(--text-secondary);cursor:pointer;font:inherit;padding:0}.back-link:hover{color:var(--accent)}.page-subtitle{margin:var(--space-4) 0;color:var(--text-secondary);font-size:13px}</style>
