<template>
  <div class="product-page product-detail-page">
    <SectionTabsHeader :title="product?.name || '产品详情'" :tabs="tabs" :active-tab="tab" @select="selectTab">
      <template #title>
        <div class="product-detail-title">
          <router-link class="icon-button" to="/products" title="返回产品中心" aria-label="返回产品中心"><ArrowLeft :size="16" aria-hidden="true" /></router-link>
          <h1 class="page-title">{{ product?.name || '产品详情' }}</h1>
        </div>
      </template>
      <template #actions><div class="product-actions"><button v-if="product" class="btn btn-primary btn-sm" :disabled="busy || resource.loading.value" @click="openEditor()">编辑基本信息</button><button v-if="product && product.stage !== 'archived'" class="btn btn-sm" :disabled="busy || resource.loading.value" @click="archiveOpen = true">归档</button><button v-if="product" class="icon-button" title="删除产品" aria-label="删除产品" :disabled="busy || resource.loading.value" @click="deleteTarget = product"><Trash2 :size="15" /></button></div></template>
    </SectionTabsHeader>
    <EmptyState v-if="resource.loading.value" variant="loading" message="正在读取产品..." />
    <EmptyState v-else-if="resource.error.value || !product" message="产品不存在或暂时无法读取"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
    <div v-else-if="tab === 'overview'" class="product-detail-content">
      <SurfaceCard>
        <div class="product-overview-heading"><span class="product-mark"><img v-if="product.icon_url" :src="product.icon_url" alt="产品图标" referrerpolicy="no-referrer" /><template v-else>{{ product.identifier || product.name.slice(0, 1) }}</template></span><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span><span class="product-muted">更新于 {{ formatTime(product.updated_at) }}</span></div>
        <p v-if="product.summary" class="product-overview-summary">{{ product.summary }}</p>
        <div v-if="!product.links?.length || !hasPublicDetails" class="product-actions product-section-gap"><button v-if="!product.links?.length" class="btn btn-sm" :disabled="busy" @click="openEditor('links')">添加资料链接</button><button v-if="!hasPublicDetails" class="btn btn-sm" :disabled="busy" @click="openEditor('showcase')">添加公开资料</button></div>
      </SurfaceCard>
      <SurfaceCard v-if="product.links?.length">
        <div class="product-section-heading"><h2 class="product-section-title">资料链接</h2><button class="btn btn-sm" :disabled="busy" @click="openEditor('links')">管理链接</button></div>
        <div v-for="(link, index) in product.links" :key="index" class="product-detail-row"><span class="badge badge-offline">{{ labelFor(linkKinds, link.kind) }}</span> <a :href="link.url" target="_blank" rel="noopener noreferrer">{{ link.label || link.url }}</a></div>
      </SurfaceCard>
      <section class="product-resource-section" aria-labelledby="product-resources-heading">
        <div class="product-resource-heading">
          <h2 id="product-resources-heading" class="product-section-title">服务与资源</h2>
          <span class="product-muted">{{ product.projects?.length || 0 }} 个项目 · {{ relatedApplications.length }} 个组件<template v-if="product.platform_linked"> · 当前管理平台</template></span>
          <button class="btn btn-sm" :disabled="busy" @click="openEditor('resources')">管理关联</button>
        </div>
        <div v-if="hasResourceDetails" class="product-detail-content">
          <SurfaceCard v-if="product.platform_linked" class="product-table-card" padding="none">
            <div class="product-toolbar"><h3 class="product-section-title" style="margin: 0">Cylism Manager（当前管理平台）</h3></div>
            <div class="table-wrap"><table class="data-table"><thead><tr><th>关联资源</th><th>管理入口</th></tr></thead><tbody><tr><td>当前管理平台<span class="cell-secondary product-line">产品版本可关联平台更新记录</span></td><td><div class="product-actions"><router-link to="/settings/system?tab=release">发布与更新</router-link><router-link to="/settings/system?tab=entry">平台入口</router-link></div></td></tr></tbody></table></div>
          </SurfaceCard>
          <SurfaceCard v-if="product.projects?.length" class="product-table-card" padding="none">
            <div class="product-toolbar"><h3 class="product-section-title" style="margin: 0">关联运维项目</h3></div>
            <div class="table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>已有环境 / 命名空间</th></tr></thead><tbody><tr v-for="project in product.projects" :key="project.id"><td><router-link :to="`/applications/projects/${project.id}`">{{ project.name }}</router-link></td><td><span v-for="env in project.environments" :key="env.id" class="product-line">{{ env.name }} · {{ env.namespace }} <span class="cell-secondary">{{ env.cluster_id ? `集群 #${env.cluster_id}` : '未绑定集群' }}</span></span><span v-if="!project.environments?.length">暂无环境</span></td></tr></tbody></table></div>
          </SurfaceCard>
          <SurfaceCard v-if="relatedApplications.length" class="product-table-card" padding="none">
            <div class="product-toolbar"><h3 class="product-section-title" style="margin: 0">相关部署组件与访问入口</h3><span class="product-muted">沿用项目和组件的已有关系</span></div>
            <div class="table-wrap"><table class="data-table"><thead><tr><th>组件</th><th>环境 / 命名空间</th><th>已有访问入口</th><th>关联方式</th></tr></thead><tbody><tr v-for="app in relatedApplications" :key="app.id"><td><router-link :to="`/applications/${app.id}`">{{ app.name }}</router-link></td><td>{{ app.environment?.name || '—' }}<span class="cell-secondary product-line">{{ app.environment?.namespace || '—' }}</span></td><td><span v-for="endpoint in app.endpoints" :key="endpoint.id" class="product-line">{{ endpoint.domain || '集群内部' }}{{ endpoint.path || '/' }} · {{ endpoint.service_port }}</span><span v-if="!app.endpoints?.length">未配置</span></td><td>{{ product.applications?.some(item => item.id === app.id) ? '指定组件' : '由项目关联' }}</td></tr></tbody></table></div>
          </SurfaceCard>
          <SurfaceCard v-if="product.database_resources?.length" class="product-table-card" padding="none">
            <div class="product-toolbar"><h3 class="product-section-title" style="margin: 0">已有数据库绑定</h3><router-link class="btn btn-sm product-primary" to="/cloud-services/databases">数据库管理</router-link></div>
            <div class="table-wrap"><table class="data-table"><thead><tr><th>数据库资源</th><th>部署组件</th></tr></thead><tbody><tr v-for="binding in product.database_resources" :key="`${binding.instance_id}-${binding.application_id}`"><td>{{ binding.name }}</td><td><router-link :to="`/applications/${binding.application_id}`">{{ relatedApplications.find(app => app.id === binding.application_id)?.name || binding.application_id }}</router-link></td></tr></tbody></table></div>
          </SurfaceCard>
        </div>
      </section>
      <SurfaceCard v-if="hasPublicDetails">
        <div class="product-section-heading"><h2 class="product-section-title">公开资料<span class="product-muted">可选 · {{ product.public_showcase ? '计划展示' : '暂不展示' }}</span></h2><button class="btn btn-sm" :disabled="busy" @click="openEditor('showcase')">编辑公开资料</button></div>
        <p class="product-muted">仅保存展示资料，尚未发布到个人主页。</p>
        <dl class="product-detail-list"><div v-if="product.public_bio"><dt>公开简介</dt><dd>{{ product.public_bio }}</dd></div><div v-if="product.public_url"><dt>展示链接</dt><dd><a :href="product.public_url" target="_blank" rel="noopener noreferrer">{{ product.public_url }}</a></dd></div></dl>
      </SurfaceCard>
    </div>
    <ProductLifecycle v-else-if="['tasks', 'versions'].includes(tab)" :key="product.id + '-' + tab" :product="product" :kind="tab" :initial-version-id="taskVersionID" @changed="refreshProduct" @select-version="showVersionTasks" />
    <ProductLifecycle v-else-if="tab === 'distributions'" :key="product.id + '-distributions'" :product="product" kind="distributions" @changed="refreshProduct" @show-channels="channelsOpen = true" />
    <BaseModal teleport :open="channelsOpen" title="发行渠道" size="large" @close="channelsOpen = false">
      <p class="product-muted">维护平台标识和访问链接；渠道当前版本与审核进度由发行记录汇总。</p>
      <EmptyState v-if="!product?.channels?.length" message="尚未添加发行渠道" />
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>平台 / 渠道</th><th>应用标识</th><th>版本 / 状态</th><th>链接</th></tr></thead><tbody><tr v-for="channel in product.channels" :key="channel.id">
        <td>{{ channel.name || labelFor(platforms, channel.platform) }}<span class="cell-secondary product-line">{{ labelFor(platforms, channel.platform) }}</span></td><td>{{ channel.app_identifier || '—' }}</td>
        <td><span class="badge" :class="channelClass(channel.publish_status)">{{ labelFor(channelStatuses, channel.publish_status) }}</span><span class="product-line">线上 {{ channel.online_version || '—' }}</span><span v-if="channel.pending_version" class="cell-secondary product-line">待发布 {{ channel.pending_version }}</span></td>
        <td><a v-if="channel.admin_url" :href="channel.admin_url" target="_blank" rel="noopener noreferrer">管理后台</a><a v-if="channel.public_url" class="product-line" :href="channel.public_url" target="_blank" rel="noopener noreferrer">公开访问 / 商店</a><span v-if="!channel.admin_url && !channel.public_url">—</span></td>
      </tr></tbody></table></div>
      <template #actions><button class="btn" @click="channelsOpen = false">关闭</button><button class="btn btn-primary" :disabled="busy" @click="openEditor('channels')">管理渠道</button></template>
    </BaseModal>
    <ProductEditor :open="editorOpen" :initial-tab="editorTab" :product="product" :busy="busy" :options="options" @close="editorOpen = false" @save="save" />
    <ConfirmDialog teleport :open="archiveOpen" title="归档产品" message="归档后产品将移入已归档列表，并退出活跃产品和行动汇总。已有部署资源不会被修改。可通过编辑产品恢复阶段。" confirm-text="归档" :busy="busy" @cancel="archiveOpen = false" @confirm="archive" />
    <ProductDeleteDialog :open="Boolean(deleteTarget)" :product="deleteTarget" :busy="busy" @cancel="deleteTarget = null" @confirm="remove" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowLeft, Trash2 } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { getProduct, getProductResourceOptions, updateProduct, deleteProduct } from '../../api/products.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ProductEditor from './ProductEditor.vue'
import ProductLifecycle from './ProductLifecycle.vue'
import ProductDeleteDialog from './ProductDeleteDialog.vue'
import { stages, platforms, channelStatuses, linkKinds, labelFor, stageClass, channelClass, editableProduct } from './product-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
import '../../styles/products.css'

const route = useRoute()
const router = useRouter()
const resource = useAsyncResource(({ signal }, id) => getProduct(id, { signal }))
const product = computed(() => resource.data.value)
const relatedApplications = computed(() => product.value?.related_applications || product.value?.applications || [])
const hasResourceDetails = computed(() => Boolean(product.value?.platform_linked || product.value?.projects?.length || relatedApplications.value.length || product.value?.database_resources?.length))
const hasPublicDetails = computed(() => Boolean(product.value?.public_bio || product.value?.public_url))
const tabs = [{ id: 'overview', label: '总览' }, { id: 'tasks', label: '事项' }, { id: 'versions', label: '版本' }, { id: 'distributions', label: '发行' }]
const tab = ref('overview'), editorTab = ref('basics'), editorOpen = ref(false), channelsOpen = ref(false), archiveOpen = ref(false), busy = ref(false), error = ref('')
const taskVersionID = ref('')
const deleteTarget = ref(null)
const options = ref({ projects: [], applications: [] })
async function load() { await resource.refresh(route.params.id); if (resource.error.value) error.value = resource.error.value.message }
async function refreshProduct() {
  const id = route.params.id
  try { const updated = await getProduct(id); if (route.params.id === id) resource.data.value = updated }
  catch (cause) { if (route.params.id === id) error.value = cause.message }
}
function applyRouteTab(value) {
  if (value === 'channels') { tab.value = 'distributions'; channelsOpen.value = true; return }
  if (value === 'resources' || value === 'showcase') {
    tab.value = 'overview'
    return
  }
  tab.value = tabs.some(item => item.id === value) ? value : 'overview'
  if (tab.value !== 'distributions') channelsOpen.value = false
}
function selectTab(value) { applyRouteTab(value); void router.replace({ query: { ...route.query, tab: value } }) }
function showVersionTasks(id) { taskVersionID.value = id; selectTab('tasks') }
async function openEditor(initialTab = 'basics') {
  if (initialTab !== 'resources') { editorTab.value = initialTab; editorOpen.value = true; return }
  busy.value = true
  try { options.value = await getProductResourceOptions(); editorTab.value = initialTab; editorOpen.value = true }
  catch (e) { error.value = e.message } finally { busy.value = false }
}
async function save(payload) { busy.value = true; try { await updateProduct(product.value.id, payload); editorOpen.value = false; archiveOpen.value = false; await load() } catch (e) { error.value = e.message } finally { busy.value = false } }
async function archive() { await save({ ...editableProduct(product.value), stage: 'archived' }) }
async function remove() {
  if (busy.value || !deleteTarget.value) return
  const id = deleteTarget.value.id
  busy.value = true
  try { await deleteProduct(id); deleteTarget.value = null; if (String(route.params.id) === String(id)) await router.push('/products') }
  catch (cause) { error.value = cause.message } finally { busy.value = false }
}
watch(() => route.params.id, () => { resource.data.value = null; editorOpen.value = false; channelsOpen.value = false; archiveOpen.value = false; deleteTarget.value = null; taskVersionID.value = ''; applyRouteTab(route.query?.tab); void load() }, { immediate: true })
watch(() => route.query?.tab, applyRouteTab)
</script>

<style scoped>
.product-detail-page > :deep(.section-tabs-header) { height: auto; min-height: var(--tabbed-page-header-height); flex-wrap: wrap; column-gap: var(--space-24); row-gap: 0; }
.product-detail-title { display: flex; align-items: center; gap: var(--space-12); flex: 1 1 240px; min-width: 0; margin-bottom: 23px; }
.product-detail-title .icon-button { flex-shrink: 0; }
.product-detail-title .page-title { min-width: 0; margin: 0; font-size: var(--tabbed-page-header-title-size); overflow-wrap: anywhere; }
.product-detail-page > :deep(.section-tabs-header .section-tabs), .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { flex-shrink: 0; }
.product-detail-page > :deep(.section-tab) { white-space: nowrap; }
.product-detail-content { display: grid; gap: var(--space-16); min-width: 0; }
.product-overview-heading { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-12); }
.product-overview-heading .product-muted { margin-left: auto; }
.product-overview-summary { margin: var(--space-16) 0 0; font-size: 13px; line-height: 1.7; white-space: pre-wrap; overflow-wrap: anywhere; }
.product-section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); margin-bottom: var(--space-16); }
.product-section-heading .product-section-title { margin: 0; }
.product-resource-section { display: grid; gap: var(--space-16); min-width: 0; }
.product-resource-heading { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-12); }
.product-resource-heading .product-section-title { margin: 0; }
.product-resource-heading .btn { margin-left: auto; }
.product-section-heading .product-section-title .product-muted { display: inline-block; margin-left: var(--space-12); font-weight: 400; }
.product-channel-notes { max-width: 300px; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 1200px) {
  .product-detail-page > :deep(.section-tabs-header) { align-items: flex-start; gap: var(--space-12); }
  .product-detail-title { flex-basis: 100%; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { order: 1; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs) { order: 2; width: 100%; min-width: 0; margin: 0 0 var(--space-12); overflow-x: auto; }
  .product-detail-page > :deep(.section-tab) { flex-shrink: 0; }
}
</style>
