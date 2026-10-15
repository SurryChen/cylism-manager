<template>
  <section class="page-shell">
    <SectionTabsHeader title="对象存储" :tabs="pageTabs" active-tab="containers" test-id-prefix="object-storage-tab" />
    <div class="cloud-page-content object-storage-page">
      <FeedbackBanner v-if="error" tone="error" :message="error" dismissible @dismiss="error = ''" />

      <EmptyState v-if="loading" variant="loading" message="正在读取云连接..." />
      <EmptyState v-else-if="!connections.length" message="尚无云连接。请先前往系统设置 / 云提供商配置。">
        <template #action><a class="btn btn-sm" href="#/settings/system?tab=cloud">前往云提供商设置</a></template>
      </EmptyState>

      <SurfaceCard v-else padding="none" class="object-storage-card">
        <div class="storage-toolbar">
          <div class="storage-filters">
            <label class="storage-filter">
              <span class="sr-only">云连接</span>
              <SelectMenu v-model.number="connectionID" class="form-select" aria-label="云连接" :options="connectionOptions" :disabled="containersLoading" @change="loadContainers" />
            </label>
          </div>
          <div class="storage-toolbar-actions">
            <span class="storage-count">{{ containersLoading ? '读取中...' : `共 ${containers.length} 个容器` }}</span>
            <button class="btn btn-primary" data-testid="container-create" type="button" @click="openContainer()"><Plus :size="16" />新增存储容器</button>
          </div>
        </div>

        <EmptyState v-if="containersLoading" variant="loading" message="正在读取存储容器..." />
        <EmptyState v-else-if="!containers.length" message="当前云连接下暂无存储容器" />
        <div v-else class="table-wrap storage-table-wrap">
          <table class="data-table storage-table">
            <thead><tr><th>名称</th><th>区域</th><th>存储类型</th><th class="action-cell">操作</th></tr></thead>
            <tbody>
              <tr v-for="item in containers" :key="item.name">
                <td class="cell-primary"><button class="link-button" :data-testid="'open-container-' + item.name" type="button" @click="openObjects(item)">{{ item.name }}</button></td>
                <td class="cell-secondary">{{ item.region || '-' }}</td>
                <td><span class="badge badge-offline">{{ item.storage_class || '-' }}</span></td>
                <td class="action-cell"><div class="btn-group"><button class="btn btn-sm" type="button" @click="openContainer(item)"><Pencil :size="14" />编辑</button><button class="btn btn-sm" :data-testid="'container-cors-' + item.name" type="button" @click="openCORS(item)"><Globe2 :size="14" />跨域</button><button class="btn btn-sm btn-danger" type="button" @click="requestDeleteContainer(item)"><Trash2 :size="14" />删除</button></div></td>
              </tr>
            </tbody>
          </table>
        </div>

      </SurfaceCard>
    </div>

    <BaseModal :open="containerFormOpen" :title="editingContainer ? '编辑存储容器' : '新增存储容器'" size="medium" @close="closeContainerForm">
      <form id="container-form" class="container-form" @submit.prevent="saveContainer">
        <label class="form-group"><span class="form-label">容器名称</span><input v-model.trim="containerForm.name" class="form-input" data-testid="container-name" :disabled="Boolean(editingContainer)" placeholder="存储容器名称" required></label>
        <label v-if="!editingContainer" class="form-group"><span class="form-label">存储地域</span><input v-model.trim="containerForm.region" class="form-input" data-testid="container-region" placeholder="例如 cn-hangzhou" required></label>
        <div class="form-row"><label class="form-group"><span class="form-label">访问权限</span><SelectMenu v-model="containerForm.acl" class="form-select" aria-label="访问权限" :options="aclOptions" /></label><label class="form-group"><span class="form-label">版本控制</span><SelectMenu v-model="containerForm.versioning" class="form-select" aria-label="版本控制" :options="versioningOptions" /></label></div>
      </form>
      <template #actions><button class="btn" type="button" @click="closeContainerForm">取消</button><button class="btn btn-primary" data-testid="container-save" type="button" @click="saveContainer">保存</button></template>
    </BaseModal>

    <BaseModal :open="Boolean(corsContainer)" :title="`跨域访问 · ${corsContainer?.name || ''}`" size="large" @close="closeCORS">
      <EmptyState v-if="corsLoading" variant="loading" message="正在读取跨域配置..." />
      <div v-else class="cors-form">
        <label class="cors-switch"><input v-model="corsEnabled" type="checkbox" data-testid="cors-enabled" @change="ensureCORSRule"><span>允许跨域访问</span></label>
        <template v-if="corsEnabled">
          <div v-for="(rule, index) in corsRules" :key="rule.key" class="cors-rule">
            <div class="cors-rule-heading"><strong>规则 {{ index + 1 }}</strong><button class="icon-button" type="button" title="删除规则" aria-label="删除规则" @click="corsRules.splice(index, 1)"><Trash2 :size="15" /></button></div>
            <label class="form-group"><span class="form-label">允许来源</span><textarea v-model="rule.origins" class="form-input cors-textarea" placeholder="https://example.com&#10;每行一个来源；* 表示允许所有来源" rows="2" /></label>
            <div class="form-group"><span class="form-label">允许的方法</span><div class="cors-methods"><label v-for="method in corsMethods" :key="method"><input v-model="rule.methods" type="checkbox" :value="method">{{ method }}</label></div></div>
            <div class="form-row"><label class="form-group"><span class="form-label">允许的请求头</span><input v-model.trim="rule.headers" class="form-input" placeholder="* 或以逗号分隔"></label><label class="form-group"><span class="form-label">暴露的响应头</span><input v-model.trim="rule.exposeHeaders" class="form-input" placeholder="以逗号分隔，可留空"></label></div>
            <label class="form-group"><span class="form-label">预检缓存（秒）</span><input v-model.number="rule.maxAgeSeconds" class="form-input cors-age" type="number" min="0" max="86400"></label>
          </div>
          <button class="btn btn-sm" type="button" :disabled="corsRules.length >= 10" @click="addCORSRule"><Plus :size="14" />添加规则</button>
        </template>
      </div>
      <template #actions><button class="btn" type="button" :disabled="corsSaving" @click="closeCORS">取消</button><button class="btn btn-primary" data-testid="cors-save" type="button" :disabled="corsLoading || corsSaving" @click="saveCORS">{{ corsSaving ? '保存中...' : '保存' }}</button></template>
    </BaseModal>

    <BaseModal :open="Boolean(container)" :title="`对象列表 · ${container}`" size="large" dialog-class="object-modal" @close="closeObjects">
      <div class="objects-modal-content" aria-labelledby="objects-title">
        <div class="objects-toolbar">
          <div class="objects-heading"><div><span class="eyebrow">对象存储</span><h2 id="objects-title">{{ container }}</h2></div></div>
          <div class="objects-actions"><label class="prefix-field"><span class="sr-only">对象前缀</span><input v-model.trim="prefix" class="form-input" placeholder="按前缀筛选" @keydown.enter.prevent="loadObjects"></label><label class="btn btn-primary upload-button"><Upload :size="15" />上传文件<input type="file" @change="uploadObject"></label></div>
        </div>
        <FeedbackBanner v-if="objectsError" tone="warning" :message="objectsError" dismissible @dismiss="objectsError = ''" />
        <EmptyState v-if="objectsLoading" variant="loading" message="正在读取对象..." />
        <EmptyState v-else-if="!objects.length" message="当前前缀下没有对象" />
        <div v-else class="table-wrap object-table-wrap">
          <table class="data-table object-table">
            <thead><tr><th>对象</th><th>大小</th><th>更新时间</th><th class="action-cell">操作</th></tr></thead>
            <tbody><tr v-for="item in objects" :key="item.key"><td class="mono cell-primary">{{ item.key }}</td><td>{{ formatSize(item.size) }}</td><td class="cell-secondary">{{ formatDate(item.last_modified) }}</td><td class="action-cell"><div class="btn-group"><a class="btn btn-sm" :href="downloadURL(item.key)"><Download :size="14" />下载</a><button class="btn btn-sm btn-danger" :data-testid="'object-delete-' + item.key" type="button" @click="requestDeleteObject(item)"><Trash2 :size="14" />删除</button></div></td></tr></tbody>
          </table>
        </div>
      </div>
    </BaseModal>

    <ConfirmDialog :open="Boolean(confirmTarget)" :title="confirmTarget?.type === 'container' ? '删除存储容器' : '删除对象'" :message="confirmMessage" confirm-text="删除" :busy="deleting" busy-text="删除中..." @cancel="confirmTarget = null" @close="confirmTarget = null" @confirm="confirmDelete" />
    <ErrorNoticeModal :open="Boolean(actionError)" :message="actionError" teleport @close="actionError = ''" />
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Download, Globe2, Pencil, Plus, Trash2, Upload } from 'lucide-vue-next'
import { createCloudContainer, deleteCloudContainer, deleteCloudObject, downloadCloudObjectURL, getCloudConnections, getCloudContainerCORS, getCloudContainers, getCloudObjects, updateCloudContainer, updateCloudContainerCORS, uploadCloudObject } from '../../api/cloud-resources.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const pageTabs = [{ id: 'containers', label: '存储容器' }]
const providerNames = { aliyun: '阿里云', tencent: '腾讯云', cloudcone: 'CloudCone' }
const aclOptions = [{ value: 'private', label: 'private（私有）' }, { value: 'public-read', label: 'public-read（公开读）' }]
const versioningOptions = [{ value: '', label: '不修改版本控制' }, { value: 'Enabled', label: 'Enabled（启用）' }, { value: 'Suspended', label: 'Suspended（暂停）' }]
const corsMethods = ['GET', 'HEAD', 'POST', 'PUT', 'DELETE']
const connections = ref([]); const connectionID = ref(0); const containers = ref([]); const container = ref(''); const prefix = ref(''); const objects = ref([])
const loading = ref(true); const containersLoading = ref(false); const objectsLoading = ref(false); const error = ref(''); const objectsError = ref(''); const actionError = ref(''); const uploading = ref(false)
let containersRequest = 0
const containerFormOpen = ref(false); const editingContainer = ref(null); const confirmTarget = ref(null); const deleting = ref(false)
const corsContainer = ref(null); const corsLoading = ref(false); const corsSaving = ref(false); const corsEnabled = ref(false); const corsRules = ref([]); const corsResponseVary = ref(null)
let corsRequest = 0; let nextCORSRuleKey = 0
const newContainer = () => ({ name: '', region: '', acl: 'private', versioning: '' }); const containerForm = reactive(newContainer())
const connectionOptions = computed(() => connections.value.map(item => ({ value: item.id, label: item.name === providerNames[item.provider] ? providerNames[item.provider] : `${item.name} · ${providerNames[item.provider] || item.provider}` })))
const containerRegion = computed(() => containers.value.find(item => item.name === container.value)?.region || '')
const confirmMessage = computed(() => confirmTarget.value?.type === 'container' ? `仅可删除空存储容器“${confirmTarget.value.item.name}”，确认继续？` : `确认删除对象“${confirmTarget.value?.item?.key || ''}”？`)
const formatSize = size => { if (!size) return '0 B'; const units = ['B', 'KB', 'MB', 'GB']; const index = Math.min(Math.floor(Math.log(size) / Math.log(1024)), 3); return (size / 1024 ** index).toFixed(index ? 1 : 0) + ' ' + units[index] }
const formatDate = value => value ? new Date(value).toLocaleString() : '-'

async function refresh() { loading.value = true; error.value = ''; try { connections.value = await getCloudConnections(); connectionID.value = connections.value[0]?.id || 0 } catch (err) { error.value = err.message || '读取云连接失败' } finally { loading.value = false }; if (connectionID.value) void loadContainers() }
async function loadContainers() { const request = ++containersRequest; container.value = ''; objects.value = []; containers.value = []; containersLoading.value = true; error.value = ''; try { const items = await getCloudContainers(connectionID.value); if (request === containersRequest) containers.value = items } catch (err) { if (request === containersRequest) error.value = err.message || '读取存储容器失败' } finally { if (request === containersRequest) containersLoading.value = false } }
function openContainer(item) { editingContainer.value = item || null; Object.assign(containerForm, item ? { ...newContainer(), ...item } : newContainer()); containerFormOpen.value = true }
function closeContainerForm() { if (!deleting.value) containerFormOpen.value = false }
const splitCORSValues = value => value.split(/[\n,]/).map(part => part.trim()).filter(Boolean)
const newCORSRule = (rule = {}) => ({ key: ++nextCORSRuleKey, origins: (rule.allowed_origins || []).join('\n'), methods: [...(rule.allowed_methods || ['GET', 'HEAD'])], headers: (rule.allowed_headers || ['*']).join(', '), exposeHeaders: (rule.expose_headers || []).join(', '), maxAgeSeconds: rule.max_age_seconds ?? 0 })
function addCORSRule() { corsRules.value.push(newCORSRule()) }
function ensureCORSRule() { if (corsEnabled.value && !corsRules.value.length) addCORSRule() }
async function openCORS(item) {
  const request = ++corsRequest
  corsContainer.value = item; corsLoading.value = true; corsRules.value = []; corsEnabled.value = false
  try {
    const config = await getCloudContainerCORS(connectionID.value, item.name, item.region)
    if (request !== corsRequest) return
    corsRules.value = (config.rules || []).map(newCORSRule)
    corsEnabled.value = corsRules.value.length > 0
    corsResponseVary.value = config.response_vary ?? null
  } catch (err) {
    if (request !== corsRequest) return
    corsContainer.value = null
    actionError.value = err.message || '读取跨域配置失败'
  } finally { if (request === corsRequest) corsLoading.value = false }
}
function closeCORS() { if (corsSaving.value) return; ++corsRequest; corsContainer.value = null }
async function saveCORS() {
  if (!corsContainer.value || corsSaving.value) return
  if (corsEnabled.value && !corsRules.value.length) { actionError.value = '请至少添加一条跨域规则'; return }
  const rules = corsEnabled.value ? corsRules.value.map(rule => ({ allowed_origins: splitCORSValues(rule.origins), allowed_methods: rule.methods, allowed_headers: splitCORSValues(rule.headers), expose_headers: splitCORSValues(rule.exposeHeaders), max_age_seconds: Number(rule.maxAgeSeconds) })) : []
  if (rules.some(rule => !rule.allowed_origins.length || !rule.allowed_methods.length || !Number.isInteger(rule.max_age_seconds) || rule.max_age_seconds < 0 || rule.max_age_seconds > 86400)) { actionError.value = '请填写每条规则的来源、请求方法和有效的缓存时间'; return }
  corsSaving.value = true
  try {
    const item = corsContainer.value
    await updateCloudContainerCORS(connectionID.value, item.name, { rules, response_vary: corsResponseVary.value }, item.region)
    corsContainer.value = null
  } catch (err) { actionError.value = err.message || '保存跨域配置失败' } finally { corsSaving.value = false }
}
async function saveContainer() { if (!containerForm.name || (!editingContainer.value && !containerForm.region)) { actionError.value = '请完整填写容器名称和存储地域'; return }; try { if (editingContainer.value) await updateCloudContainer(connectionID.value, editingContainer.value.name, { ...containerForm }); else await createCloudContainer(connectionID.value, { ...containerForm }); containerFormOpen.value = false; await loadContainers() } catch (err) { actionError.value = err.message || '保存存储容器失败' } }
function requestDeleteContainer(item) { confirmTarget.value = { type: 'container', item } }
function requestDeleteObject(item) { confirmTarget.value = { type: 'object', item } }
async function confirmDelete() { if (!confirmTarget.value) return; deleting.value = true; const target = confirmTarget.value; try { if (target.type === 'container') { await deleteCloudContainer(connectionID.value, target.item.name, { confirm: true }, target.item.region); if (container.value === target.item.name) closeObjects(); await loadContainers() } else { await deleteCloudObject(connectionID.value, container.value, { key: target.item.key, confirm: true }, containerRegion.value); await loadObjects() }; confirmTarget.value = null } catch (err) { actionError.value = err.message || (target.type === 'container' ? '删除存储容器失败' : '删除对象失败') } finally { deleting.value = false } }
function closeObjects() { container.value = ''; prefix.value = ''; objects.value = []; objectsError.value = '' }
async function openObjects(item) { container.value = item.name; prefix.value = ''; await loadObjects() }
async function loadObjects() { if (!container.value) return; objectsLoading.value = true; objectsError.value = ''; try { objects.value = (await getCloudObjects(connectionID.value, container.value, { prefix: prefix.value, region: containerRegion.value })).objects || [] } catch (err) { objectsError.value = err.message || '读取对象失败' } finally { objectsLoading.value = false } }
async function uploadObject(event) { const file = event.target.files?.[0]; if (!file || !container.value || uploading.value) return; uploading.value = true; const form = new FormData(); form.append('file', file); form.append('key', prefix.value + file.name); try { await uploadCloudObject(connectionID.value, container.value, form, containerRegion.value); await loadObjects() } catch (err) { actionError.value = err.message || '上传对象失败' } finally { uploading.value = false; event.target.value = '' } }
const downloadURL = key => downloadCloudObjectURL(connectionID.value, container.value, key, containerRegion.value)
onMounted(refresh)
</script>

<style scoped>
.cloud-page-content { margin-top: var(--tabbed-page-content-gap); }
.object-storage-page { display: grid; gap: var(--space-16); }
.object-storage-card { overflow: visible; }
.storage-toolbar, .objects-toolbar { display: flex; min-height: 66px; align-items: flex-end; justify-content: space-between; gap: var(--space-16); padding: 14px var(--space-20); }
.storage-filters, .storage-toolbar-actions, .objects-actions, .objects-heading, .btn-group { display: flex; align-items: center; gap: 8px; }
.storage-filter { display: block; width: min(320px, 100%); }
.storage-toolbar-actions { justify-content: flex-end; }
.storage-count { color: var(--text-secondary); font-size: 12px; white-space: nowrap; }
.storage-table-wrap, .object-table-wrap { padding: 0 var(--space-20); }
.storage-table { min-width: 680px; }
.object-table { min-width: 760px; }
.action-cell { width: 260px; white-space: nowrap; text-align: right; }
.action-cell .btn-group { flex-wrap: nowrap; justify-content: flex-end; }
.objects-heading h2 { margin: 2px 0 0; color: var(--text-primary); font-size: 15px; }
.eyebrow { color: var(--text-muted); font: 10px/1 var(--font-mono); letter-spacing: .08em; text-transform: uppercase; }
.prefix-field { width: min(240px, 34vw); }
.prefix-field .form-input { min-height: var(--button-height); }
.upload-button { position: relative; overflow: hidden; cursor: pointer; }
.upload-button input { position: absolute; inset: 0; width: 100%; cursor: pointer; opacity: 0; }
.link-button { padding: 0; border: 0; background: transparent; color: var(--action-primary); font: inherit; font-weight: 700; cursor: pointer; }
.link-button:hover { text-decoration: underline; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.container-form { display: grid; gap: var(--space-12); }
.container-form .form-group { margin: 0; }
.cors-form { display: grid; gap: var(--space-12); }
.cors-switch, .cors-methods, .cors-methods label, .cors-rule-heading { display: flex; align-items: center; gap: 8px; }
.cors-switch { font-size: 13px; font-weight: 600; }
.cors-rule { display: grid; gap: var(--space-12); padding: 12px 0; border-top: 1px solid var(--border-muted); }
.cors-rule-heading { justify-content: space-between; font-size: 12px; }
.cors-rule .form-group { margin: 0; }
.cors-methods { flex-wrap: wrap; gap: 8px 16px; font-size: 12px; }
.cors-textarea { min-height: 64px; resize: vertical; }
.cors-age { width: 130px; }
.objects-modal-content { min-width: 0; }
.objects-modal-content .objects-toolbar { padding: 0 0 var(--space-16); }
.objects-modal-content .object-table-wrap { padding: 0; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 760px) { .storage-toolbar, .objects-toolbar { align-items: stretch; flex-direction: column; padding: 14px; }.storage-filter { width: 100%; }.storage-toolbar-actions, .objects-actions { align-items: stretch; justify-content: space-between; }.prefix-field { width: auto; flex: 1; }.storage-table-wrap, .object-table-wrap { padding: 0 14px; } }
@media (max-width: 460px) { .storage-toolbar-actions, .objects-actions { align-items: stretch; flex-direction: column; }.storage-count { align-self: flex-start; }.prefix-field { width: 100%; }.upload-button { width: 100%; } }
</style>
