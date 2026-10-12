<template>
  <div class="catalog-detail">
    <button class="back-link" type="button" @click="router.push('/applications/catalog')"><ArrowLeft :size="15" />返回应用商城</button>
    <SectionTabsHeader :title="item?.name || '商城应用'" :tabs="[{ id: 'config', label: '应用配置' }]" active-tab="config" :sticky="false">
      <template #actions><span v-if="item" class="badge" :class="item.enabled ? 'badge-online' : 'badge-offline'">{{ item.enabled ? '已启用' : '未启用' }}</span></template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="readError" tone="warning" :message="readError" />
    <EmptyState v-if="loading" variant="loading" message="加载中..." />
    <SurfaceCard v-else-if="item" padding="none">
      <form class="detail-form" @submit.prevent="save">
        <section class="detail-section"><h2>基本信息</h2><div class="detail-fields"><div class="form-group"><label class="form-label">应用名称</label><input v-model.trim="form.name" class="form-input" required maxlength="128" /></div><div class="form-group"><label class="form-label">应用标识</label><input v-model.trim="form.slug" class="form-input" required maxlength="63" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" /></div><div class="form-group detail-description"><label class="form-label">说明</label><input v-model.trim="form.description" class="form-input" maxlength="512" placeholder="可选" /></div></div></section>
        <section class="detail-section"><div class="section-title"><h2>部署组件</h2><button type="button" class="btn btn-sm" :disabled="form.components.length >= 20" @click="addComponent"><Plus :size="14" />添加组件</button></div>
          <EmptyState v-if="form.components.length === 0" message="尚未配置部署组件" />
          <template v-else><div class="component-tabs"><button v-for="(component, index) in form.components" :key="component.localID" type="button" class="component-tab" :class="{ 'is-active': activeIndex === index }" @click="activeIndex = index"><span>{{ component.name || `组件 ${index + 1}` }}</span><small>{{ component.applicationName || '未设置标识' }}</small></button></div>
            <div v-if="activeComponent" class="component-fields"><div class="section-title component-heading"><span>组件 {{ activeIndex + 1 }} / {{ form.components.length }}</span><button type="button" class="icon-button" title="移除当前组件" aria-label="移除当前组件" @click="removeComponent"><Trash2 :size="15" /></button></div>
              <div class="detail-fields"><div class="form-group"><label class="form-label">组件名称</label><input v-model.trim="activeComponent.name" class="form-input" required maxlength="128" placeholder="前端" /></div><div class="form-group"><label class="form-label">部署标识</label><input v-model.trim="activeComponent.applicationName" class="form-input" required maxlength="63" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="orders-web" /></div><div class="form-group"><label class="form-label">镜像路径</label><input v-model.trim="activeComponent.image" class="form-input" required placeholder="registry.example.com/team/orders-web" /></div><div class="form-group"><label class="form-label">默认版本号</label><input v-model.trim="activeComponent.defaultVersion" class="form-input" pattern="[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}" placeholder="1.4.2" /></div><div class="form-group"><label class="form-label">容器与 Service 端口</label><input v-model.number="activeComponent.port" class="form-input" type="number" min="1" max="65535" required /></div><div class="form-group"><label class="form-label">默认副本数</label><input v-model.number="activeComponent.replicas" class="form-input" type="number" min="1" required /></div></div>
              <h3 class="field-heading">资源请求与限制</h3><div class="resource-fields"><div class="form-group"><label class="form-label">CPU 请求</label><input v-model.trim="activeComponent.requestsCPU" class="form-input" required placeholder="100m" /></div><div class="form-group"><label class="form-label">内存请求</label><input v-model.trim="activeComponent.requestsMemory" class="form-input" required placeholder="128Mi" /></div><div class="form-group"><label class="form-label">CPU 限制</label><input v-model.trim="activeComponent.limitsCPU" class="form-input" required placeholder="500m" /></div><div class="form-group"><label class="form-label">内存限制</label><input v-model.trim="activeComponent.limitsMemory" class="form-input" required placeholder="512Mi" /></div></div>
              <div class="section-title config-heading"><h3>组件配置</h3><button type="button" class="btn btn-sm" @click="activeComponent.configRows.push({ key: '', value: '' })"><Plus :size="14" />添加配置</button></div><div v-for="(row, index) in activeComponent.configRows" :key="index" class="config-row"><input v-model.trim="row.key" class="form-input" :aria-label="`配置项 ${index + 1} 名称`" placeholder="API_URL" /><input v-model="row.value" class="form-input" :aria-label="`配置项 ${index + 1} 值`" placeholder="http://orders-api:8080" /><button type="button" class="icon-button" title="删除配置项" aria-label="删除配置项" @click="activeComponent.configRows.splice(index, 1)"><Trash2 :size="15" /></button></div>
              <label class="check-row"><input v-model="activeComponent.readinessEnabled" type="checkbox" />启用 HTTP 就绪检查</label><div v-if="activeComponent.readinessEnabled" class="form-group"><label class="form-label">检查路径</label><input v-model.trim="activeComponent.readinessPath" class="form-input" required placeholder="/healthz" /></div>
            </div></template>
        </section>
        <div class="detail-actions"><label class="check-row"><input v-model="form.enabled" type="checkbox" />启用商城应用</label><button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存配置' }}</button></div>
      </form>
    </SurfaceCard>
    <ErrorNoticeModal :open="!!mutationError" :message="mutationError" teleport @close="mutationError = ''" />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Plus, Trash2 } from 'lucide-vue-next'
import { getApplicationCatalogItem, updateApplicationCatalogItem } from '../../api/applications.js'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const route = useRoute()
const router = useRouter()
const item = ref(null)
const form = ref(newForm())
const activeIndex = ref(0)
const loading = ref(true)
const saving = ref(false)
const readError = ref('')
const mutationError = ref('')
let nextLocalID = 0
const activeComponent = computed(() => form.value.components[activeIndex.value])

function newForm() { return { name: '', slug: '', description: '', enabled: false, entryIndex: 0, components: [] } }
function newComponent() { return { localID: ++nextLocalID, name: '', applicationName: '', image: '', defaultVersion: '', port: 8080, replicas: 1, requestsCPU: '100m', requestsMemory: '128Mi', limitsCPU: '500m', limitsMemory: '512Mi', configRows: [], readinessEnabled: false, readinessPath: '/healthz' } }
function fromComponent(component) {
  const spec = component.spec
  return { localID: ++nextLocalID, name: component.name, applicationName: component.application_name, image: spec.image, defaultVersion: component.default_version || '', port: spec.container_port, replicas: spec.replicas, requestsCPU: spec.resources?.requests_cpu || '', requestsMemory: spec.resources?.requests_memory || '', limitsCPU: spec.resources?.limits_cpu || '', limitsMemory: spec.resources?.limits_memory || '', configRows: Object.entries(spec.config || {}).map(([key, value]) => ({ key, value })), readinessEnabled: spec.health?.readiness_enabled || false, readinessPath: spec.health?.readiness_path || '/healthz' }
}
function fromItem(value) { return { name: value.name, slug: value.slug, description: value.description || '', enabled: value.enabled, components: value.components.map(fromComponent), entryIndex: Math.max(0, value.components.findIndex(component => component.application_name === value.entry_component)) } }
function addComponent() { form.value.components.push(newComponent()); activeIndex.value = form.value.components.length - 1 }
function removeComponent() { const index = activeIndex.value; form.value.components.splice(index, 1); if (form.value.entryIndex === index) form.value.entryIndex = 0; else if (form.value.entryIndex > index) form.value.entryIndex--; activeIndex.value = Math.max(0, Math.min(index, form.value.components.length - 1)) }
function componentPayload(component) {
  return { name: component.name, application_name: component.applicationName, default_version: component.defaultVersion, spec: { image: component.image, container_port: component.port, replicas: component.replicas, resources: { requests_cpu: component.requestsCPU, requests_memory: component.requestsMemory, limits_cpu: component.limitsCPU, limits_memory: component.limitsMemory }, config: Object.fromEntries(component.configRows.map(row => [row.key, row.value])), health: { readiness_enabled: component.readinessEnabled, readiness_type: 'http', readiness_path: component.readinessPath, liveness_enabled: false, liveness_type: 'http', liveness_path: '/healthz' }, service: { type: 'ClusterIP', ports: [{ name: 'http', protocol: 'TCP', port: component.port, target_port: component.port }] }, endpoint: { exposure: 'cluster' } } }
}
function validateForm() {
  if (form.value.enabled && form.value.components.length === 0) { mutationError.value = '启用前请至少配置一个部署组件'; return false }
  const names = form.value.components.map(component => component.applicationName)
  const invalidIndex = form.value.components.findIndex(component => {
    const keys = component.configRows.map(row => row.key)
    return !component.name || !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(component.applicationName) || names.indexOf(component.applicationName) !== names.lastIndexOf(component.applicationName) || !component.image || !Number.isInteger(component.port) || component.port < 1 || component.port > 65535 || !Number.isInteger(component.replicas) || component.replicas < 1 || !component.requestsCPU || !component.requestsMemory || !component.limitsCPU || !component.limitsMemory || component.configRows.some(row => !/^[A-Za-z_][A-Za-z0-9_]*$/.test(row.key) || keys.indexOf(row.key) !== keys.lastIndexOf(row.key)) || (component.readinessEnabled && !component.readinessPath)
  })
  if (invalidIndex < 0) return true
  activeIndex.value = invalidIndex
  mutationError.value = `请补全组件 ${invalidIndex + 1} 的部署配置`
  return false
}
async function load() {
  loading.value = true; readError.value = ''
  try { item.value = await getApplicationCatalogItem(route.params.catalogID); form.value = fromItem(item.value); activeIndex.value = 0 }
  catch (error) { readError.value = error.message || '加载商城应用失败'; item.value = null }
  finally { loading.value = false }
}
async function save() {
  mutationError.value = ''
  if (!validateForm()) return
  saving.value = true
  try {
    const value = form.value
    const payload = { name: value.name, slug: value.slug, description: value.description, enabled: value.enabled, entry_component: value.components[value.entryIndex]?.applicationName || '', components: value.components.map(componentPayload), revision: item.value.revision }
    await updateApplicationCatalogItem(item.value.id, payload)
    await router.push('/applications/catalog')
  } catch (error) { mutationError.value = error.message || '保存商城应用失败' }
  finally { saving.value = false }
}
watch(() => route.params.catalogID, load, { immediate: true })
</script>

<style scoped>
.catalog-detail { display: grid; gap: var(--space-4); }
.back-link { display: inline-flex; width: max-content; align-items: center; gap: 6px; border: 0; padding: 0; background: transparent; color: var(--action-primary); font: inherit; font-size: 12px; cursor: pointer; }
.detail-form { padding: 0 var(--space-20); }
.detail-section { padding: var(--space-20) 0; border-bottom: 1px solid var(--border-muted); }
.detail-section h2 { margin: 0 0 14px; color: var(--text-primary); font-size: 15px; }
.detail-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; }
.detail-description { grid-column: 1 / -1; }
.section-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.section-title h2, .section-title h3 { margin: 0; }
.component-tabs { display: flex; gap: 6px; overflow-x: auto; padding: 14px 0 10px; }
.component-tab { display: flex; min-width: 130px; flex-direction: column; gap: 3px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 8px 10px; background: var(--surface-subtle); color: var(--text-primary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; }
.component-tab.is-active { border-color: var(--action-primary); background: var(--surface-hover); }
.component-tab small { color: var(--text-muted); font-size: 10px; }
.component-fields { padding-top: 12px; }
.component-heading { margin-bottom: 12px; color: var(--text-secondary); font-size: 12px; }
.field-heading, .config-heading h3 { margin: 8px 0 12px; color: var(--text-primary); font-size: 12px; }
.resource-fields { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.config-heading { margin: 10px 0; }
.config-heading h3 { margin: 0; }
.config-row { display: grid; grid-template-columns: minmax(110px, 1fr) minmax(0, 2fr) 34px; align-items: center; gap: 8px; margin-bottom: 8px; }
.check-row { display: flex; align-items: center; gap: 8px; color: var(--text-primary); font-size: 12px; }
.detail-actions { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: var(--space-20) 0; }
@media (max-width: 760px) { .detail-form { padding: 0 14px; }.resource-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .detail-fields { grid-template-columns: 1fr; }.config-row { grid-template-columns: minmax(0, 1fr) 34px; }.config-row .form-input:nth-child(2) { grid-column: 1; grid-row: 2; }.config-row .icon-button { grid-column: 2; grid-row: 1 / 3; } }
</style>
