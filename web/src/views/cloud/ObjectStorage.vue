<template>
  <section class="page-shell">
    <div class="page-heading">
      <div><h1>对象存储</h1><p>查看已配置云连接中的存储容器与对象。</p></div>
    </div>
    <p v-if="error" class="form-error">{{ error }}</p>
    <div v-if="loading" class="empty-state"><span class="empty-text">正在读取云连接...</span></div>
    <div v-else-if="!connections.length" class="empty-state"><span class="empty-text">尚无云连接。请先前往 <a href="#/settings/system?tab=cloud">系统设置 / 云提供商</a> 配置。</span></div>
    <template v-else>
      <div class="toolbar">
        <label class="select-field">云连接<select v-model.number="connectionID" class="form-select" @change="loadContainers"><option v-for="item in connections" :key="item.id" :value="item.id">{{ item.name }} · {{ item.provider }}</option></select></label>
        <button class="btn btn-primary" data-testid="container-create" @click="openContainer()">新增存储容器</button>
      </div>
      <div v-if="containerFormOpen" class="inline-form">
        <input v-model.trim="containerForm.name" class="form-input" data-testid="container-name" :disabled="Boolean(editingContainer)" placeholder="存储容器名称">
        <select v-model="containerForm.acl" class="form-select"><option value="private">private</option><option value="public-read">public-read</option></select>
        <select v-model="containerForm.versioning" class="form-select"><option value="">不修改版本控制</option><option value="Enabled">Enabled</option><option value="Suspended">Suspended</option></select>
        <button class="btn btn-primary" data-testid="container-save" @click="saveContainer">保存</button>
        <button class="btn" @click="containerFormOpen = false">取消</button>
      </div>
      <div class="table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>区域</th><th>存储类型</th><th>操作</th></tr></thead><tbody>
        <tr v-for="item in containers" :key="item.name"><td><button class="link-button" :data-testid="'open-container-' + item.name" @click="openObjects(item.name)">{{ item.name }}</button></td><td>{{ item.region || '-' }}</td><td>{{ item.storage_class || '-' }}</td><td><button class="btn btn-sm" @click="openContainer(item)">编辑</button> <button class="btn btn-sm danger" @click="removeContainer(item)">删除</button></td></tr>
        <tr v-if="!containers.length"><td colspan="4" class="empty-cell">暂无存储容器</td></tr>
      </tbody></table></div>
      <div v-if="container" class="objects-panel">
        <div class="toolbar"><strong>{{ container }}</strong><label class="select-field">对象前缀<input v-model.trim="prefix" class="form-input" @change="loadObjects"></label><label class="btn btn-primary upload-button">上传文件<input type="file" @change="uploadObject"></label></div>
        <div class="table-wrap"><table class="data-table"><thead><tr><th>对象</th><th>大小</th><th>更新时间</th><th>操作</th></tr></thead><tbody>
          <tr v-for="item in objects" :key="item.key"><td class="mono">{{ item.key }}</td><td>{{ formatSize(item.size) }}</td><td>{{ formatDate(item.last_modified) }}</td><td><a class="btn btn-sm" :href="downloadURL(item.key)">下载</a> <button class="btn btn-sm danger" :data-testid="'object-delete-' + item.key" @click="removeObject(item)">删除</button></td></tr>
          <tr v-if="!objects.length"><td colspan="4" class="empty-cell">当前前缀下没有对象</td></tr>
        </tbody></table></div>
      </div>
    </template>
  </section>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { createCloudContainer, deleteCloudContainer, deleteCloudObject, downloadCloudObjectURL, getCloudConnections, getCloudContainers, getCloudObjects, updateCloudContainer, uploadCloudObject } from '../../api/cloud-resources.js'

const connections = ref([])
const connectionID = ref(0)
const containers = ref([])
const container = ref('')
const prefix = ref('')
const objects = ref([])
const loading = ref(true)
const error = ref('')
const containerFormOpen = ref(false)
const editingContainer = ref(null)
const newContainer = () => ({ name: '', acl: 'private', versioning: '' })
const containerForm = reactive(newContainer())
const assignContainer = value => Object.assign(containerForm, value)
const formatSize = size => { if (!size) return '0 B'; const units = ['B', 'KB', 'MB', 'GB']; const index = Math.min(Math.floor(Math.log(size) / Math.log(1024)), 3); return (size / 1024 ** index).toFixed(index ? 1 : 0) + ' ' + units[index] }
const formatDate = value => value ? new Date(value).toLocaleString() : '-'
async function refresh() { loading.value = true; error.value = ''; try { connections.value = await getCloudConnections(); connectionID.value = connections.value[0] ? connections.value[0].id : 0; if (connectionID.value) await loadContainers() } catch (err) { error.value = err.message || '读取云连接失败' } finally { loading.value = false } }
async function loadContainers() { container.value = ''; objects.value = []; try { containers.value = await getCloudContainers(connectionID.value) } catch (err) { error.value = err.message || '读取存储容器失败' } }
function openContainer(item) {
  editingContainer.value = item || null
  assignContainer(item ? { ...newContainer(), ...item } : newContainer())
  containerFormOpen.value = true
}
async function saveContainer() {
  try {
    if (editingContainer.value) await updateCloudContainer(connectionID.value, editingContainer.value.name, { ...containerForm })
    else await createCloudContainer(connectionID.value, { ...containerForm })
    containerFormOpen.value = false
    await loadContainers()
  } catch (err) { error.value = err.message || '保存存储容器失败' }
}
async function removeContainer(item) {
  if (!window.confirm('仅可删除空存储容器：' + item.name + '。确认继续？')) return
  try {
    await deleteCloudContainer(connectionID.value, item.name, { confirm: true })
    if (container.value === item.name) { container.value = ''; objects.value = [] }
    await loadContainers()
  } catch (err) { error.value = err.message || '删除存储容器失败' }
}
async function openObjects(name) { container.value = name; prefix.value = ''; await loadObjects() }
async function loadObjects() { if (!container.value) return; try { objects.value = (await getCloudObjects(connectionID.value, container.value, { prefix: prefix.value })).objects || [] } catch (err) { error.value = err.message || '读取对象失败' } }
async function uploadObject(event) {
  const file = event.target.files?.[0]
  if (!file || !container.value) return
  const form = new FormData()
  form.append('file', file)
  form.append('key', prefix.value + file.name)
  try {
    await uploadCloudObject(connectionID.value, container.value, form)
    await loadObjects()
  } catch (err) { error.value = err.message || '上传对象失败' } finally { event.target.value = '' }
}
const downloadURL = key => downloadCloudObjectURL(connectionID.value, container.value, key)
async function removeObject(item) {
  if (!window.confirm('删除对象 ' + item.key + '？')) return
  try {
    await deleteCloudObject(connectionID.value, container.value, { key: item.key, confirm: true })
    await loadObjects()
  } catch (err) { error.value = err.message || '删除对象失败' }
}
onMounted(refresh)
</script>

<style scoped>
.page-heading{margin-bottom:16px}.page-heading h1{margin:0}.page-heading p{margin:4px 0 0;color:var(--text-secondary)}.toolbar{display:flex;gap:12px;align-items:end;margin-bottom:16px}.inline-form{display:grid;grid-template-columns:minmax(160px,1fr) 130px 170px auto auto;gap:8px;margin:0 0 12px}.objects-panel{margin-top:16px;padding:16px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-raised)}.link-button{padding:0;border:0;background:transparent;color:var(--action-primary);cursor:pointer}.upload-button input{display:none}.mono{font-family:var(--font-mono);overflow-wrap:anywhere}.danger{color:var(--danger)}.empty-cell{text-align:center;color:var(--text-muted);padding:24px}@media(max-width:760px){.toolbar{align-items:stretch;flex-direction:column}.inline-form{grid-template-columns:1fr}}
</style>
