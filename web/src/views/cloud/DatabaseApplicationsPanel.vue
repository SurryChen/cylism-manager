<template>
  <section class="database-applications">
    <div class="applications-toolbar">
      <div><h3>关联应用 <span>{{ rows.length }}</span></h3><p class="cell-secondary">按上线模板配置识别，实际运行状态请在应用中查看。</p></div>
      <button class="icon-button" type="button" title="刷新应用连接" aria-label="刷新应用连接" :disabled="loading" @click="load"><RefreshCw :size="15" /></button>
    </div>
    <EmptyState v-if="loading" variant="loading" message="识别应用连接..." />
    <EmptyState v-else-if="failed" message="应用连接读取失败" description="可刷新重试" />
    <template v-else>
      <EmptyState v-if="!rows.length" message="未识别到应用连接" description="识别环境变量、MySQL 连接串及挂载的 YAML、JSON、dotenv 配置；未匹配不代表没有应用使用此实例。" />
      <div v-else class="applications-table">
        <table class="data-table">
          <thead><tr><th>应用 / 模板</th><th>数据库 / 账号</th><th>关联方式</th><th>配置来源</th></tr></thead>
          <tbody><tr v-for="(row, index) in rows" :key="`${row.template_id}-${index}`">
            <td><a :href="`#/applications/${row.application_id}?tab=config`">{{ row.application_name }}</a><div class="cell-secondary">{{ row.template_name }}</div></td>
            <td>{{ row.database_name || '未指定数据库' }}<div class="cell-secondary">{{ row.username || '未识别账号' }}</div></td>
            <td><span class="badge" :class="row.binding_id ? 'badge-online' : 'badge-offline'">{{ row.binding_id ? '已绑定' : '配置识别' }}</span></td>
            <td><div v-for="source in row.sources" :key="source" class="cell-secondary connection-source">{{ source }}</div></td>
          </tr></tbody>
        </table>
      </div>
      <button v-if="warnings.length" class="btn btn-sm" type="button" @click="error = warnings.join('\n')">{{ warnings.length }} 项配置未完整识别 · 查看原因</button>
    </template>
    <ErrorNoticeModal :open="Boolean(error)" title="应用连接识别" :message="error" teleport @close="error = ''" />
  </section>
</template>

<script setup>
import { ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getDatabaseApplications } from '../../api/databases.js'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
const props = defineProps({ instanceId: { type: Number, required: true } })
const rows = ref([]), warnings = ref([]), loading = ref(false), failed = ref(false), error = ref('')
let generation = 0
async function load() {
  const current = ++generation
  rows.value = []; warnings.value = []; loading.value = true; failed.value = false; error.value = ''
  try {
    const result = await getDatabaseApplications(props.instanceId)
    if (current !== generation) return
    rows.value = result.items; warnings.value = result.warnings || []
  } catch (cause) {
    if (current !== generation) return
    failed.value = true; error.value = cause.message || '读取应用连接失败'
  } finally { if (current === generation) loading.value = false }
}
watch(() => props.instanceId, load, { immediate: true })
</script>

<style scoped>
.database-applications { min-width: 0; }
.applications-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); margin-bottom: var(--space-20); }
.applications-toolbar h3 { margin: 0; font-size: 15px; font-weight: 600; }.applications-toolbar h3 span { margin-left: 6px; color: var(--text-muted); font-weight: 400; }
.applications-toolbar p { margin: 6px 0 0; font-size: 12px; }.applications-table { overflow-x: auto; margin-bottom: var(--space-16); }
.applications-table .data-table { min-width: 560px; }.connection-source { max-width: 260px; overflow-wrap: anywhere; }
</style>
