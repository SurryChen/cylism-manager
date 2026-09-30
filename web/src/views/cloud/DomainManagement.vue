<template>
  <section class="page-shell">
    <SectionTabsHeader title="域名管理" :tabs="pageTabs" active-tab="records" test-id-prefix="domain-management-tab" />
    <div class="cloud-page-content">
      <SurfaceCard padding="none" class="record-list" data-testid="domain-record-list">
        <div class="record-toolbar" data-testid="domain-record-toolbar">
          <div class="record-filters">
            <label class="record-filter"><span class="sr-only">云连接</span><SelectMenu v-model.number="connectionID" class="form-select" aria-label="云连接" :options="connectionOptions" :disabled="loadingConnections || !connections.length" @change="loadZones" /></label>
            <label class="record-filter"><span class="sr-only">域名</span><SelectMenu v-model="zone" class="form-select" aria-label="域名" :options="zoneOptions" placeholder="选择域名" :disabled="loadingZones || !zones.length" @change="loadRecords" /></label>
          </div>
          <div class="record-toolbar-actions">
            <span v-if="zone && !loadingRecords && !recordsFailed" class="record-count">{{ records.length }} 条记录</span>
            <button type="button" class="btn btn-primary" :disabled="!zone || loadingRecords || recordsFailed" data-testid="domain-record-create" @click="openRecord()"><Plus :size="16" /> 新增记录</button>
          </div>
        </div>

        <EmptyState v-if="loadingConnections" variant="loading" message="正在读取云连接..." />
        <EmptyState v-else-if="connectionsFailed" message="云连接暂不可展示">
          <template #action><button type="button" class="btn btn-sm" @click="refresh">重试</button></template>
        </EmptyState>
        <div v-else-if="!connections.length" class="record-empty"><span>尚无云连接。请先前往 <a href="#/settings/system?tab=cloud">系统设置 / 云提供商</a> 配置。</span></div>
        <EmptyState v-else-if="loadingZones" variant="loading" message="正在读取域名..." />
        <EmptyState v-else-if="zonesFailed" message="域名暂不可展示">
          <template #action><button type="button" class="btn btn-sm" @click="loadZones">重试</button></template>
        </EmptyState>
        <EmptyState v-else-if="!zones.length" message="此云连接下暂无域名" />
        <EmptyState v-else-if="loadingRecords" variant="loading" message="正在读取 DNS 记录..." />
        <EmptyState v-else-if="recordsFailed" message="DNS 记录暂不可展示" data-testid="domain-record-error-state">
          <template #action><button type="button" class="btn btn-sm" data-testid="domain-record-retry" @click="loadRecords">重试</button></template>
        </EmptyState>
        <EmptyState v-else-if="!records.length" message="暂无 DNS 记录" />
        <div v-else class="table-wrap"><table class="data-table record-table"><thead><tr><th>主机记录</th><th>类型</th><th>线路</th><th>记录值</th><th>TTL</th><th class="action-cell">操作</th></tr></thead><tbody>
          <tr v-for="item in records" :key="item.id"><td class="cell-primary">{{ item.rr }}</td><td>{{ item.type }}</td><td>{{ item.line }}</td><td class="mono">{{ item.value }}</td><td>{{ item.ttl }}</td><td class="action-cell"><button class="btn btn-sm" @click="openRecord(item)">编辑</button> <button class="btn btn-sm danger" :data-testid="'domain-record-delete-' + item.id" @click="removeRecord(item)">删除</button></td></tr>
        </tbody></table></div>
      </SurfaceCard>
    </div>

    <BaseModal :open="formOpen" :title="editing ? '编辑 DNS 记录' : '新增 DNS 记录'" size="medium" @close="formOpen = false">
      <form id="domain-record-form" class="record-form" @submit.prevent="saveRecord">
        <div class="form-row">
          <label class="form-group"><span class="form-label">主机记录</span><input v-model.trim="form.rr" class="form-input" placeholder="例如 @ 或 www" required></label>
          <label class="form-group"><span class="form-label">类型</span><SelectMenu v-model="form.type" class="form-select" aria-label="记录类型" :options="recordTypeOptions" /></label>
        </div>
        <label class="form-group"><span class="form-label">记录值</span><input v-model.trim="form.value" class="form-input" data-testid="domain-record-value" placeholder="填写记录值" required></label>
        <label class="form-group"><span class="form-label">TTL（秒）</span><input v-model.number="form.ttl" class="form-input" type="number" min="1" max="86400" required></label>
      </form>
      <template #actions>
        <button type="button" class="btn" :disabled="saving" @click="formOpen = false">取消</button>
        <button type="submit" form="domain-record-form" class="btn btn-primary" :disabled="saving" data-testid="domain-record-save">{{ saving ? '保存中...' : '保存' }}</button>
      </template>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(error)" :title="errorTitle" :message="error" @close="error = ''" />
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus } from 'lucide-vue-next'
import { createCloudDNSRecord, deleteCloudDNSRecord, getCloudConnections, getCloudDNSRecords, getCloudZones, updateCloudDNSRecord } from '../../api/cloud-resources.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const pageTabs = [{ id: 'records', label: 'DNS 记录' }]
const recordTypeOptions = ['A', 'AAAA', 'CNAME', 'TXT', 'MX'].map(value => ({ value, label: value }))
const providerNames = { aliyun: '阿里云', tencent: '腾讯云', cloudcone: 'CloudCone' }
const connections = ref([])
const connectionID = ref(0)
const connectionOptions = computed(() => connections.value.map(item => {
  const provider = providerNames[item.provider] || item.provider
  return { value: item.id, label: item.name === provider ? provider : `${item.name} · ${provider}` }
}))
const selectedProviderName = computed(() => {
  const provider = connections.value.find(item => item.id === connectionID.value)?.provider
  return providerNames[provider] || '云服务'
})
const zones = ref([])
const zone = ref('')
const zoneOptions = computed(() => zones.value.map(item => ({ value: item.name, label: item.name })))
const records = ref([])
const loadingConnections = ref(true)
const loadingZones = ref(false)
const loadingRecords = ref(false)
const connectionsFailed = ref(false)
const zonesFailed = ref(false)
const recordsFailed = ref(false)
const error = ref('')
const errorTitle = ref('操作失败')
const formOpen = ref(false)
const saving = ref(false)
const editing = ref(null)
const newRecord = () => ({ rr: '@', type: 'A', line: 'default', value: '', ttl: 600, priority: 0 })
const form = reactive(newRecord())
const assign = value => Object.assign(form, value)

function showError(err, title, fallback) {
  const message = err?.message || ''
  errorTitle.value = title
  error.value = /(?:i\/o timeout|timeout|timed out)/i.test(message)
    ? title === '读取云连接失败'
      ? '读取云连接超时，请检查服务器网络后重试。'
      : `连接${selectedProviderName.value} DNS 接口超时，请检查服务器网络和 DNS 解析后重试。`
    : /https?:\/\/|AccessKeyId=|Signature=/i.test(message) ? fallback : message || fallback
}

async function refresh() {
  loadingConnections.value = true
  connectionsFailed.value = false
  error.value = ''
  try {
    connections.value = await getCloudConnections()
    connectionID.value = connections.value[0]?.id || 0
    if (connectionID.value) await loadZones()
  } catch (err) {
    connectionsFailed.value = true
    showError(err, '读取云连接失败', '读取云连接失败，请稍后重试。')
  } finally {
    loadingConnections.value = false
  }
}

async function loadZones() {
  zones.value = []
  zone.value = ''
  records.value = []
  zonesFailed.value = false
  recordsFailed.value = false
  error.value = ''
  if (!connectionID.value) return
  loadingZones.value = true
  try {
    zones.value = await getCloudZones(connectionID.value)
    zone.value = zones.value[0]?.name || ''
    if (zone.value) await loadRecords()
  } catch (err) {
    zonesFailed.value = true
    showError(err, '读取域名失败', '读取域名失败，请稍后重试。')
  } finally {
    loadingZones.value = false
  }
}

async function loadRecords() {
  records.value = []
  recordsFailed.value = false
  error.value = ''
  if (!zone.value) return
  loadingRecords.value = true
  try {
    records.value = (await getCloudDNSRecords(connectionID.value, zone.value)).records || []
  } catch (err) {
    recordsFailed.value = true
    showError(err, '读取 DNS 记录失败', '读取 DNS 记录失败，请稍后重试。')
  } finally {
    loadingRecords.value = false
  }
}

function openRecord(record) {
  editing.value = record || null
  assign(record ? { ...newRecord(), ...record } : newRecord())
  formOpen.value = true
}

async function saveRecord() {
  saving.value = true
  try {
    if (editing.value) await updateCloudDNSRecord(connectionID.value, editing.value.id, { ...form })
    else await createCloudDNSRecord(connectionID.value, { zone: zone.value, ...form })
    formOpen.value = false
    await loadRecords()
  } catch (err) {
    showError(err, '保存 DNS 记录失败', '保存 DNS 记录失败，请检查填写内容后重试。')
  } finally {
    saving.value = false
  }
}

async function removeRecord(record) {
  if (!window.confirm('删除 ' + record.rr + ' ' + record.type + ' 记录？')) return
  try {
    await deleteCloudDNSRecord(connectionID.value, record.id, { confirm: true })
    await loadRecords()
  } catch (err) {
    showError(err, '删除 DNS 记录失败', '删除 DNS 记录失败，请稍后重试。')
  }
}

onMounted(refresh)
</script>

<style scoped>
.cloud-page-content { margin-top: var(--tabbed-page-content-gap); }
.record-list { min-height: 300px; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
.record-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: var(--space-16); padding: 12px var(--space-16); }
.record-filters { display: flex; flex: 1 1 auto; min-width: 0; flex-wrap: wrap; align-items: center; gap: var(--space-8); }
.record-filter { display: block; flex: 0 1 220px; min-width: 160px; }
.record-filter :deep(.select-menu-trigger) { min-height: 36px; }
.record-toolbar-actions { display: flex; flex: 0 0 auto; align-items: center; justify-content: flex-end; gap: var(--space-12); }
.record-count { color: var(--text-secondary); font-size: 12px; white-space: nowrap; }
.record-list > .table-wrap { padding: 0 var(--space-16); }
.record-table { min-width: 680px; }
.record-table th:last-child, .record-table td:last-child { width: 1%; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.action-cell { white-space: nowrap; }
.danger { color: var(--danger); }
.record-empty { display: flex; min-height: 190px; align-items: center; justify-content: center; padding: var(--space-16); color: var(--text-muted); font-size: 12px; text-align: center; }
.record-form { display: grid; gap: var(--space-12); }
.record-form .form-group { margin: 0; }
@media (max-width: 760px) {
  .record-toolbar { align-items: stretch; flex-direction: column; }
  .record-filters { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .record-filter { width: 100%; min-width: 0; }
  .record-toolbar-actions { justify-content: flex-end; }
}
@media (max-width: 480px) { .record-filters { grid-template-columns: 1fr; } }
</style>
