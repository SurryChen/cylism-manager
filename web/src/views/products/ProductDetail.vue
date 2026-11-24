<template>
  <div class="product-page product-detail-page">
    <SectionTabsHeader :title="product?.name || (projectMode ? '项目详情' : '产品详情')" :tabs="tabs" :active-tab="tab" @select="selectTab">
      <template #title>
        <div class="product-detail-title">
          <PageBackLink :to="projectMode ? '/projects' : '/products'">
            {{ projectMode ? '返回项目中心' : '返回产品中心' }}
          </PageBackLink>
          <h1 class="page-title">{{ product?.name || (projectMode ? '项目详情' : '产品详情') }}</h1>
        </div>
      </template>
      <template #actions><div class="product-actions"><button v-if="product" class="btn btn-primary btn-sm" :disabled="busy || resource.loading.value" @click="openEditor()">编辑基本信息</button><button v-if="product && product.stage !== 'archived'" class="btn btn-sm" :disabled="busy || resource.loading.value" @click="archiveOpen = true">归档</button><button v-if="product" class="icon-button" :title="projectMode ? '删除项目' : '删除产品'" :aria-label="projectMode ? '删除项目' : '删除产品'" :disabled="busy || resource.loading.value" @click="deleteTarget = product"><Trash2 :size="15" /></button></div></template>
    </SectionTabsHeader>
    <EmptyState v-if="resource.loading.value" variant="loading" :message="projectMode ? '正在读取项目...' : '正在读取产品...'" />
    <EmptyState v-else-if="resource.error.value || !product" :message="projectMode ? '项目不存在或暂时无法读取' : '产品不存在或暂时无法读取'"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
    <div v-else-if="tab === 'overview'" class="product-detail-content">
      <SurfaceCard>
        <div class="product-overview-heading"><span class="product-mark"><img v-if="product.icon_url" :src="product.icon_url" alt="产品图标" referrerpolicy="no-referrer" /><template v-else>{{ product.name.slice(0, 1) }}</template></span><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span><span class="product-muted">更新于 {{ formatTime(product.updated_at) }}</span></div>
        <p v-if="product.summary" class="product-overview-summary">{{ product.summary }}</p>
        <div v-if="!product.links?.length" class="product-actions product-section-gap"><button class="btn btn-sm" :disabled="busy" @click="openEditor('links')">添加资料链接</button></div>
      </SurfaceCard>
      <SurfaceCard v-if="product.links?.length">
        <div class="product-section-heading"><h2 class="product-section-title">资料链接</h2><button class="btn btn-sm" :disabled="busy" @click="openEditor('links')">管理链接</button></div>
        <div v-for="(link, index) in product.links" :key="index" class="product-detail-row"><span class="badge badge-offline">{{ labelFor(linkKinds, link.kind) }}</span> <a :href="link.url" target="_blank" rel="noopener noreferrer">{{ link.label || link.url }}</a></div>
      </SurfaceCard>
    </div>
    <ProjectEnvironments v-else-if="tab === 'environments' && product.project_id" :project-i-d="String(product.project_id)" embedded @changed="refreshProduct" />
    <ProductLifecycle v-else-if="['tasks', 'versions', 'distributions'].includes(tab)" :key="product.id + '-' + tab" :product="product" :kind="tab" :initial-version-id="lifecycleVersionID" @changed="refreshProduct" @select-version="id => showVersionRecords('tasks', id)" @select-distributions="id => showVersionRecords('distributions', id)" @show-channels="channelsOpen = true" @update-release-units="updateReleaseUnits" />
    <BaseModal teleport :open="channelsOpen" title="发行渠道" size="large" @close="channelsOpen = false">
      <p class="product-muted">维护平台标识和访问链接；渠道当前版本与审核进度由发行记录汇总。</p>
      <EmptyState v-if="!product?.channels?.length" message="尚未添加发行渠道" />
      <div v-else class="table-wrap"><DataTable><thead><tr><th>平台 / 渠道</th><th>应用标识</th><th>版本 / 状态</th><th>链接</th></tr></thead><tbody><tr v-for="channel in product.channels" :key="channel.id">
        <td>{{ channel.name || labelFor(platforms, channel.platform) }}<span class="cell-secondary product-line">{{ labelFor(platforms, channel.platform) }}<template v-if="product.release_units?.length"> · {{ channel.release_unit_id == null ? '不限发布端' : releaseUnitName(product.release_units, channel.release_unit_id) }}</template></span></td><td>{{ channel.app_identifier || '—' }}</td>
        <td><span class="badge" :class="channelClass(channel.publish_status)">{{ labelFor(channelStatuses, channel.publish_status) }}</span><span class="product-line">正式线上 {{ channel.online_version || '—' }}</span><span v-if="channel.pending_version" class="cell-secondary product-line">正式待发布 {{ channel.pending_version }}</span></td>
        <td><a v-if="channel.admin_url" :href="channel.admin_url" target="_blank" rel="noopener noreferrer">管理后台</a><a v-if="channel.public_url" class="product-line" :href="channel.public_url" target="_blank" rel="noopener noreferrer">公开访问 / 商店</a><span v-if="!channel.admin_url && !channel.public_url">—</span></td>
      </tr></tbody></DataTable></div>
      <template #actions><button class="btn" @click="channelsOpen = false">关闭</button><button class="btn btn-primary" :disabled="busy" @click="openEditor('channels')">管理渠道</button></template>
    </BaseModal>
    <ProductEditor :project-mode="projectMode" :open="editorOpen" :initial-tab="editorTab" :product="product" :busy="busy" @close="editorOpen = false" @save="save" />
    <ConfirmDialog teleport :open="archiveOpen" :title="projectMode ? '归档项目' : '归档产品'" :message="projectMode ? '归档后项目将移入已归档列表，并退出活跃项目和行动汇总。已有部署资源不会被修改。可通过编辑基本信息恢复阶段。' : '归档后产品将移入已归档列表，并退出活跃产品和行动汇总。已有部署资源不会被修改。可通过编辑产品恢复阶段。'" confirm-text="归档" :busy="busy" @cancel="archiveOpen = false" @confirm="archive" />
    <ProductDeleteDialog :project-mode="projectMode" :open="Boolean(deleteTarget)" :product="deleteTarget" :busy="busy" @cancel="deleteTarget = null" @confirm="remove" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import DataTable from '../../components/DataTable.vue'
import { computed, ref, watch } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { getProduct, updateProduct, deleteProduct, getProjectWorkspace, deleteProjectWorkspace } from '../../api/products.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import PageBackLink from '../../components/PageBackLink.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import BaseModal from '../../components/BaseModal.vue'
import ProjectEnvironments from '../applications/ProjectEnvironments.vue'
import ProductEditor from './ProductEditor.vue'
import { releaseUnitName } from './lifecycle-utils.js'
import ProductLifecycle from './ProductLifecycle.vue'
import ProductDeleteDialog from './ProductDeleteDialog.vue'
import { stages, platforms, channelStatuses, linkKinds, labelFor, stageClass, channelClass, editableProduct } from './product-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
import '../../styles/products.css'

const props = defineProps({ projectMode: Boolean })
const route = useRoute()
const routeID = computed(() => props.projectMode ? route.params.projectID : route.params.id)
const router = useRouter()
const resource = useAsyncResource(({ signal }, id) => props.projectMode ? getProjectWorkspace(id, { signal }) : getProduct(id, { signal }))
const product = computed(() => resource.data.value)
const tabs = computed(() => [{ id: 'overview', label: '总览' }, { id: 'tasks', label: props.projectMode ? '待办' : '事项' }, { id: 'versions', label: '版本' }, ...(props.projectMode ? [{ id: 'environments', label: '环境与部署' }] : []), { id: 'distributions', label: '发行' }])
const tab = ref('overview'), editorTab = ref('basics'), editorOpen = ref(false), channelsOpen = ref(false), archiveOpen = ref(false), busy = ref(false), error = ref('')
const lifecycleVersionID = ref('')
const deleteTarget = ref(null)
async function load() { await resource.refresh(routeID.value); if (resource.error.value) error.value = resource.error.value.message
  if (!props.projectMode && product.value?.project_id) await router.replace({ path: '/projects/' + product.value.project_id, query: route.query })
}
async function refreshProduct() {
  const id = routeID.value
  try { const updated = await (props.projectMode ? getProjectWorkspace(id) : getProduct(id)); if (routeID.value === id) resource.data.value = updated }
  catch (cause) { if (routeID.value === id) error.value = cause.message }
}
function applyRouteTab(value) {
  if (value === 'channels') { tab.value = 'distributions'; channelsOpen.value = true; return }
  tab.value = tabs.value.some(item => item.id === value) ? value : 'overview'
  if (tab.value !== 'distributions') channelsOpen.value = false
}
function selectTab(value) { showVersionRecords(value, '') }
function showVersionRecords(value, id) {
  lifecycleVersionID.value = id
  applyRouteTab(value)
  void router.replace({ query: { ...route.query, tab: value } })
}
function updateReleaseUnits(units) {
  resource.data.value = { ...product.value, release_units: units }
}
function openEditor(initialTab = 'basics') { channelsOpen.value = false; editorTab.value = initialTab; editorOpen.value = true }
async function save(payload) { busy.value = true; try { await updateProduct(product.value.id, payload); editorOpen.value = false; archiveOpen.value = false; await load() } catch (e) { error.value = e.message } finally { busy.value = false } }
async function archive() { await save({ ...editableProduct(product.value), stage: 'archived' }) }
async function remove() {
  if (busy.value || !deleteTarget.value) return
  const id = deleteTarget.value.id
  busy.value = true
  try { if (props.projectMode) await deleteProjectWorkspace(product.value.project_id); else await deleteProduct(id); deleteTarget.value = null; await router.push(props.projectMode ? '/projects' : '/products') }
  catch (cause) { error.value = cause.message } finally { busy.value = false }
}
watch(() => [props.projectMode, routeID.value], () => { resource.data.value = null; editorOpen.value = false; channelsOpen.value = false; archiveOpen.value = false; deleteTarget.value = null; lifecycleVersionID.value = ''; applyRouteTab(route.query?.tab); void load() }, { immediate: true })
watch(() => route.query?.tab, applyRouteTab)
</script>

<style scoped>
.product-detail-page > :deep(.section-tabs-header) { height: auto; min-height: var(--tabbed-page-header-height); flex-wrap: wrap; column-gap: var(--space-24); row-gap: 0; }
.product-detail-title { display: flex; flex: 1 1 240px; flex-direction: column; align-items: flex-start; min-width: 0; margin-bottom: 23px; }
.product-detail-title .page-title { min-width: 0; margin: 0; font-size: var(--tabbed-page-header-title-size); overflow-wrap: anywhere; }
.product-detail-page > :deep(.section-tabs-header .section-tabs), .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { flex-shrink: 0; }
.product-detail-page > :deep(.section-tab) { white-space: nowrap; }
.product-detail-content { display: grid; gap: var(--space-16); min-width: 0; }
.product-overview-heading { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-12); }
.product-overview-heading .product-muted { margin-left: auto; }
.product-overview-summary { margin: var(--space-16) 0 0; font-size: 13px; line-height: 1.7; white-space: pre-wrap; overflow-wrap: anywhere; }
.product-section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); margin-bottom: var(--space-16); }
.product-section-heading .product-section-title { margin: 0; }
.product-channel-notes { max-width: 300px; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 1200px) {
  .product-detail-page > :deep(.section-tabs-header) { align-items: flex-start; gap: var(--space-12); }
  .product-detail-title { flex-basis: 100%; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { order: 1; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs) { order: 2; width: 100%; min-width: 0; margin: 0 0 var(--space-12); overflow-x: auto; }
  .product-detail-page > :deep(.section-tab) { flex-shrink: 0; }
}
</style>
