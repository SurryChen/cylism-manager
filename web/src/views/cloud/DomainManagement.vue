<template>
  <section class='page-shell'>
    <div class='page-heading'><div><h1>域名管理</h1><p>查看已配置云连接中的域名与 DNS 解析记录。</p></div></div>
    <p v-if='error' class='form-error'>{{ error }}</p>
    <div v-if='loading' class='empty-state'><span class='empty-text'>正在读取云连接...</span></div>
    <div v-else-if='!connections.length' class='empty-state'><span class='empty-text'>尚无云连接。请先前往 <a href='#/settings/system?tab=cloud'>系统设置 / 云提供商</a> 配置。</span></div>
    <template v-else>
      <div class='toolbar'>
        <label class='select-field'>云连接<select v-model.number='connectionID' class='form-select' @change='loadZones'><option v-for='item in connections' :key='item.id' :value='item.id'>{{ item.name }} · {{ item.provider }}</option></select></label>
        <label class='select-field'>域名<select v-model='zone' class='form-select' @change='loadRecords'><option value='' disabled>选择域名</option><option v-for='item in zones' :key='item.name' :value='item.name'>{{ item.name }}</option></select></label>
        <button class='btn btn-primary' :disabled='!zone' data-testid='domain-record-create' @click='openRecord()'>新增记录</button>
      </div>
      <div v-if='formOpen' class='inline-form'>
        <input v-model.trim='form.rr' class='form-input' placeholder='主机记录，如 @ 或 www'>
        <select v-model='form.type' class='form-select'><option>A</option><option>AAAA</option><option>CNAME</option><option>TXT</option><option>MX</option></select>
        <input v-model.trim='form.value' class='form-input' data-testid='domain-record-value' placeholder='记录值'>
        <input v-model.number='form.ttl' class='form-input' type='number' min='1' max='86400'>
        <button class='btn btn-primary' data-testid='domain-record-save' @click='saveRecord'>保存</button>
        <button class='btn' @click='formOpen = false'>取消</button>
      </div>
      <div class='table-wrap'><table class='data-table'><thead><tr><th>主机记录</th><th>类型</th><th>线路</th><th>记录值</th><th>TTL</th><th class='action-cell'>操作</th></tr></thead><tbody>
        <tr v-for='item in records' :key='item.id'><td>{{ item.rr }}</td><td>{{ item.type }}</td><td>{{ item.line }}</td><td class='mono'>{{ item.value }}</td><td>{{ item.ttl }}</td><td class='action-cell'><button class='btn btn-sm' @click='openRecord(item)'>编辑</button> <button class='btn btn-sm danger' :data-testid="'domain-record-delete-' + item.id" @click='removeRecord(item)'>删除</button></td></tr>
        <tr v-if='zone && !records.length'><td colspan='6' class='empty-cell'>暂无 DNS 记录</td></tr>
      </tbody></table></div>
    </template>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { createCloudDNSRecord, deleteCloudDNSRecord, getCloudConnections, getCloudDNSRecords, getCloudZones, updateCloudDNSRecord } from '../../api/cloud-resources.js'

const connections = ref([])
const connectionID = ref(0)
const zones = ref([])
const zone = ref('')
const records = ref([])
const loading = ref(true)
const error = ref('')
const formOpen = ref(false)
const editing = ref(null)
const newRecord = () => ({ rr: '@', type: 'A', line: 'default', value: '', ttl: 600, priority: 0 })
const form = reactive(newRecord())
const assign = value => Object.assign(form, value)
async function refresh() {
  loading.value = true
  error.value = ''
  try {
    connections.value = await getCloudConnections()
    connectionID.value = connections.value[0]?.id || 0
    if (connectionID.value) await loadZones()
  } catch (err) { error.value = err.message || '读取云连接失败' } finally { loading.value = false }
}
async function loadZones() {
  zones.value = []; zone.value = ''; records.value = []
  try { zones.value = await getCloudZones(connectionID.value); zone.value = zones.value[0]?.name || ''; if (zone.value) await loadRecords() } catch (err) { error.value = err.message || '读取域名失败' }
}
async function loadRecords() {
  if (!zone.value) return
  try { records.value = (await getCloudDNSRecords(connectionID.value, zone.value)).records || [] } catch (err) { error.value = err.message || '读取 DNS 记录失败' }
}
function openRecord(record) {
  editing.value = record || null
  assign(record ? { ...newRecord(), ...record } : newRecord())
  formOpen.value = true
}
async function saveRecord() {
  try {
    if (editing.value) await updateCloudDNSRecord(connectionID.value, editing.value.id, { ...form })
    else await createCloudDNSRecord(connectionID.value, { zone: zone.value, ...form })
    formOpen.value = false
    await loadRecords()
  } catch (err) { error.value = err.message || '保存 DNS 记录失败' }
}
async function removeRecord(record) {
  if (!window.confirm('删除 ' + record.rr + ' ' + record.type + ' 记录？')) return
  try {
    await deleteCloudDNSRecord(connectionID.value, record.id, { confirm: true })
    await loadRecords()
  } catch (err) { error.value = err.message || '删除 DNS 记录失败' }
}
onMounted(refresh)
</script>

<style scoped>
.page-heading{margin-bottom:16px}.page-heading h1{margin:0}.page-heading p{margin:4px 0 0;color:var(--text-secondary)}.toolbar{display:flex;gap:12px;align-items:end;margin-bottom:16px}.inline-form{display:grid;grid-template-columns:1fr 100px minmax(140px,1fr) 90px auto auto;gap:8px;margin:0 0 12px}.mono{font-family:var(--font-mono);overflow-wrap:anywhere}.action-cell{width:1%;white-space:nowrap}.danger{color:var(--danger)}.empty-cell{text-align:center;color:var(--text-muted);padding:24px}@media(max-width:760px){.toolbar{align-items:stretch;flex-direction:column}.inline-form{grid-template-columns:1fr}}
</style>
