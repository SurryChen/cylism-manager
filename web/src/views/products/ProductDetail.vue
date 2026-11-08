<template>
  <div class="product-page product-detail-page">
    <SectionTabsHeader :title="product?.name || '产品详情'" :tabs="tabs" :active-tab="tab" @select="tab = $event"><template #actions><div class="product-actions"><router-link class="btn btn-sm" to="/products">返回产品中心</router-link><button v-if="product" class="btn btn-primary btn-sm" :disabled="busy || resource.loading.value" @click="openEditor()">编辑产品</button><button v-if="product && product.stage !== 'archived'" class="btn btn-sm" :disabled="busy || resource.loading.value" @click="archiveOpen = true">归档</button></div></template></SectionTabsHeader>
    <EmptyState v-if="resource.loading.value" variant="loading" message="正在读取产品..." />
    <EmptyState v-else-if="resource.error.value || !product" message="产品不存在或暂时无法读取"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
    <div v-else-if="tab === 'overview'" class="product-detail-content">
      <div class="product-detail-grid">
        <SurfaceCard><h2 class="product-section-title">产品资料</h2><dl class="product-detail-list"><div><dt>阶段</dt><dd><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span></dd></div><div><dt>标识</dt><dd><span class="product-mark"><img v-if="product.icon_url" :src="product.icon_url" alt="产品图标" referrerpolicy="no-referrer" /><template v-else>{{ product.identifier || product.name.slice(0, 1) }}</template></span><span v-if="product.icon_url && product.identifier" class="product-line">{{ product.identifier }}</span></dd></div><div><dt>简介</dt><dd>{{ product.summary || '暂无简介' }}</dd></div><div><dt>最近更新</dt><dd>{{ formatTime(product.updated_at) }}</dd></div></dl></SurfaceCard>
        <SurfaceCard><h2 class="product-section-title">下一步行动</h2><dl class="product-detail-list"><div><dt>行动</dt><dd>{{ product.next_action || '尚未设置' }}</dd></div><div><dt>截止日期</dt><dd>{{ dateOnly(product.next_due_date) || '未设置' }}<span v-if="product.next_due_date" class="badge product-line" :class="dueClass(dueState(product.next_due_date))">{{ dueLabel(dueState(product.next_due_date)) }}</span></dd></div></dl></SurfaceCard>
      </div>
      <SurfaceCard><h2 class="product-section-title">资料链接</h2><EmptyState v-if="!product.links?.length" message="暂无代码仓库、设计稿或文档链接" /><div v-for="(link, index) in product.links" :key="index" class="product-detail-row"><span class="badge badge-offline">{{ labelFor(linkKinds, link.kind) }}</span> <a :href="link.url" target="_blank" rel="noopener noreferrer">{{ link.label || link.url }}</a></div></SurfaceCard>
    </div>
    <SurfaceCard v-else-if="tab === 'channels'" class="product-table-card" padding="none">
      <div class="product-toolbar"><h2 class="product-section-title" style="margin: 0">发行渠道</h2><span class="product-muted">手工维护</span><button class="btn btn-sm product-primary" :disabled="busy" @click="openEditor('channels')">管理渠道</button></div>
      <EmptyState v-if="!product.channels?.length" message="尚未添加发行渠道" />
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>平台 / 渠道</th><th>平台应用标识</th><th>版本 / 状态</th><th>链接</th><th>记录更新时间 / 备注</th></tr></thead><tbody><tr v-for="channel in product.channels" :key="channel.id">
        <td>{{ channel.name || labelFor(platforms, channel.platform) }}<span class="cell-secondary product-line">{{ labelFor(platforms, channel.platform) }}</span></td><td>{{ channel.app_identifier || '—' }}</td>
        <td><span class="badge" :class="channelClass(channel.publish_status)">{{ labelFor(channelStatuses, channel.publish_status) }}</span><span class="product-line">线上 {{ channel.online_version || '—' }}</span><span class="cell-secondary product-line">待发布 {{ channel.pending_version || '—' }}</span></td>
        <td><a v-if="channel.admin_url" :href="channel.admin_url" target="_blank" rel="noopener noreferrer">管理后台</a><a v-if="channel.public_url" class="product-line" :href="channel.public_url" target="_blank" rel="noopener noreferrer">公开访问 / 商店</a><span v-if="!channel.admin_url && !channel.public_url">—</span></td><td>{{ formatTime(channel.updated_at) }}<span class="cell-secondary product-line product-channel-notes">{{ channel.notes || '—' }}</span></td>
      </tr></tbody></table></div>
    </SurfaceCard>
    <div v-else-if="tab === 'resources'" class="product-detail-content">
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-section-title" style="margin: 0">关联运维项目</h2></div>
        <EmptyState v-if="!product.projects?.length" message="未关联项目，产品可以独立管理" />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>已有环境 / 命名空间</th></tr></thead><tbody><tr v-for="project in product.projects" :key="project.id"><td><router-link :to="`/applications/projects/${project.id}`">{{ project.name }}</router-link></td><td><span v-for="env in project.environments" :key="env.id" class="product-line">{{ env.name }} · {{ env.namespace }} <span class="cell-secondary">{{ env.cluster_id ? `集群 #${env.cluster_id}` : '未绑定集群' }}</span></span><span v-if="!project.environments?.length">暂无环境</span></td></tr></tbody></table></div>
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-section-title" style="margin: 0">相关部署组件与访问入口</h2><span class="product-muted">沿用项目和组件的已有关系</span></div>
        <EmptyState v-if="!relatedApplications.length" message="暂无关联部署组件" />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>组件</th><th>环境 / 命名空间</th><th>已有访问入口</th><th>关联方式</th></tr></thead><tbody><tr v-for="app in relatedApplications" :key="app.id"><td><router-link :to="`/applications/${app.id}`">{{ app.name }}</router-link></td><td>{{ app.environment?.name || '—' }}<span class="cell-secondary product-line">{{ app.environment?.namespace || '—' }}</span></td><td><span v-for="endpoint in app.endpoints" :key="endpoint.id" class="product-line">{{ endpoint.domain || '集群内部' }}{{ endpoint.path || '/' }} · {{ endpoint.service_port }}</span><span v-if="!app.endpoints?.length">未配置</span></td><td>{{ product.applications?.some(item => item.id === app.id) ? '指定组件' : '由项目关联' }}</td></tr></tbody></table></div>
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-section-title" style="margin: 0">已有数据库绑定</h2><router-link class="btn btn-sm product-primary" to="/cloud-services/databases">数据库管理</router-link></div>
        <EmptyState v-if="!product.database_resources?.length" message="相关组件暂无数据库绑定" />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>数据库资源</th><th>部署组件</th></tr></thead><tbody><tr v-for="binding in product.database_resources" :key="`${binding.instance_id}-${binding.application_id}`"><td>{{ binding.name }}</td><td><router-link :to="`/applications/${binding.application_id}`">{{ relatedApplications.find(app => app.id === binding.application_id)?.name || binding.application_id }}</router-link></td></tr></tbody></table></div>
      </SurfaceCard>
    </div>
    <SurfaceCard v-else><h2 class="product-section-title">公开作品资料</h2><dl class="product-detail-list"><div><dt>展示计划</dt><dd>{{ product.public_showcase ? '计划展示' : '暂不展示' }} · 尚未发布到个人主页</dd></div><div><dt>公开简介</dt><dd>{{ product.public_bio || '未填写' }}</dd></div><div><dt>展示链接</dt><dd><a v-if="product.public_url" :href="product.public_url" target="_blank" rel="noopener noreferrer">{{ product.public_url }}</a><template v-else>未填写</template></dd></div></dl></SurfaceCard>
    <ProductEditor :open="editorOpen" :initial-tab="editorTab" :product="product" :busy="busy" :options="options" @close="editorOpen = false" @save="save" />
    <ConfirmDialog teleport :open="archiveOpen" title="归档产品" message="归档后产品将移入已归档列表，并退出活跃产品和行动汇总。已有部署资源不会被修改。可通过编辑产品恢复阶段。" confirm-text="归档" :busy="busy" @cancel="archiveOpen = false" @confirm="archive" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getProduct, getProductResourceOptions, updateProduct } from '../../api/products.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ProductEditor from './ProductEditor.vue'
import { stages, platforms, channelStatuses, linkKinds, labelFor, dateOnly, dueState, dueLabel, dueClass, stageClass, channelClass, editableProduct } from './product-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
import '../../styles/products.css'

const route = useRoute()
const resource = useAsyncResource(({ signal }, id) => getProduct(id, { signal }))
const product = computed(() => resource.data.value)
const relatedApplications = computed(() => product.value?.related_applications || product.value?.applications || [])
const tabs = [{ id: 'overview', label: '资料与行动' }, { id: 'channels', label: '发行渠道' }, { id: 'resources', label: '运维关联' }, { id: 'showcase', label: '公开作品' }]
const tab = ref('overview'), editorTab = ref('basics'), editorOpen = ref(false), archiveOpen = ref(false), busy = ref(false), error = ref('')
const options = ref({ projects: [], applications: [] })
async function load() { await resource.refresh(route.params.id); if (resource.error.value) error.value = resource.error.value.message }
async function openEditor(initialTab = 'basics') { busy.value = true; try { options.value = await getProductResourceOptions(); editorTab.value = initialTab; editorOpen.value = true } catch (e) { error.value = e.message } finally { busy.value = false } }
async function save(payload) { busy.value = true; try { await updateProduct(product.value.id, payload); editorOpen.value = false; archiveOpen.value = false; await load() } catch (e) { error.value = e.message } finally { busy.value = false } }
async function archive() { await save({ ...editableProduct(product.value), stage: 'archived' }) }
watch(() => route.params.id, () => { resource.data.value = null; editorOpen.value = false; archiveOpen.value = false; tab.value = 'overview'; void load() }, { immediate: true })
</script>

<style scoped>
.product-detail-page > :deep(.section-tabs-header) { height: auto; min-height: var(--tabbed-page-header-height); flex-wrap: wrap; column-gap: var(--space-24); row-gap: 0; }
.product-detail-page > :deep(.section-tabs-header .page-title) { flex: 1 1 240px; min-width: 0; overflow-wrap: anywhere; }
.product-detail-page > :deep(.section-tabs-header .section-tabs), .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { flex-shrink: 0; }
.product-detail-page > :deep(.section-tab) { white-space: nowrap; }
.product-detail-content { display: grid; gap: var(--space-16); min-width: 0; }
.product-detail-list .badge.product-line { width: fit-content; }
.product-channel-notes { max-width: 300px; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 1200px) {
  .product-detail-page > :deep(.section-tabs-header) { align-items: flex-start; gap: var(--space-12); }
  .product-detail-page > :deep(.section-tabs-header .page-title) { flex-basis: 100%; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs-actions) { order: 1; margin: 0; }
  .product-detail-page > :deep(.section-tabs-header .section-tabs) { order: 2; width: 100%; min-width: 0; margin: 0 0 var(--space-12); overflow-x: auto; }
  .product-detail-page > :deep(.section-tab) { flex-shrink: 0; }
}
</style>
