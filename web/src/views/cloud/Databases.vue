<template>
  <section class="page-shell">
    <SectionTabsHeader title="数据库" :tabs="[{ id: 'clusters', label: 'MySQL 集群' }, { id: 'instances', label: '已有实例' }]" :active-tab="activeTab" @select="activeTab = $event" />
    <div class="database-content">
      <SurfaceCard v-if="activeTab === 'clusters'" padding="none">
        <div class="database-toolbar">
          <span class="cell-secondary">{{ loading ? '读取中...' : `共 ${mysqlClusters.length} 个集群` }}</span>
          <div class="row-actions"><button class="btn" @click="openTasks()">数据库任务</button><button class="btn" @click="openCapabilities()">数据库操作</button><button class="btn btn-primary" type="button" @click="openClusterCreate"><Plus :size="16" />新增 MySQL 集群</button></div>
        </div>
        <EmptyState v-if="loading" variant="loading" message="正在读取 MySQL 集群..." />
        <EmptyState v-else-if="!mysqlClusters.length" message="暂无 MySQL 集群" />
        <div v-else class="table-wrap table-inset">
          <table class="data-table database-table">
            <thead><tr><th>集群名称</th><th>Kubernetes 集群</th><th>数据库实例</th><th>Router 实例</th><th>状态</th><th class="actions">操作</th></tr></thead>
            <tbody><tr v-for="item in mysqlClusters" :key="item.id">
              <td class="cell-primary">{{ item.name }}</td><td>{{ clusterName(item.cluster_id) }}</td>
              <td>{{ item.database_instances || item.database_nodes?.length || 0 }} <span class="cell-secondary">· {{ item.database_nodes?.length || 0 }} 台节点</span></td><td>{{ item.router_instances ?? item.router_nodes?.length ?? 0 }} <span v-if="item.router_nodes?.length" class="cell-secondary">· {{ item.router_nodes.length }} 台候选节点</span></td>
              <td><span class="badge" :class="item.status === 'ready' ? 'badge-online' : 'badge-offline'">{{ clusterStatusLabel(item.status) }}</span></td>
              <td class="actions"><div class="row-actions"><button class="btn btn-sm" type="button" @click="router.push(`/cloud-services/databases/clusters/${item.id}`)">查看与配置</button><button v-if="item.status === 'draft'" class="icon-button danger-icon" type="button" title="删除未部署集群" aria-label="删除未部署集群" @click="deletingCluster = item"><Trash2 :size="15" /></button></div></td>
            </tr></tbody>
          </table>
        </div>
      </SurfaceCard>
      <SurfaceCard v-else padding="none">
        <div class="database-toolbar">
          <span class="cell-secondary">{{ loading ? '读取中...' : `共 ${instances.length} 个实例` }}</span>
          <div class="row-actions"><button class="btn" @click="openTasks()">数据库任务</button><button class="btn" type="button" @click="openCreate">接入已有 MySQL</button><button class="btn btn-primary" @click="openCapability('deploy')"><Plus :size="16" />部署 MySQL</button></div>
        </div>
        <EmptyState v-if="loading" variant="loading" message="正在读取数据库..." />
        <EmptyState v-else-if="!instances.length" message="暂无数据库实例" />
        <div v-else class="table-wrap table-inset">
          <table class="data-table database-table">
            <thead><tr><th>实例名称</th><th>管理方式</th><th>位置</th><th>状态</th><th class="actions">操作</th></tr></thead>
            <tbody><tr v-for="item in instances" :key="item.id">
              <td class="cell-primary">{{ item.name }}</td><td>{{ item.mode === 'managed' ? '独立 MySQL' : '已有 MySQL' }}</td>
              <td>{{ item.mode === 'managed' ? item.node_name || clusterName(item.cluster_id) : item.private_gateway ? clusterName(item.private_gateway.cluster_id) : '外部实例' }}</td>
              <td><button v-if="instanceError(item)" class="status-error" @click="actionError = instanceError(item)"><span class="badge badge-danger">异常</span></button><span v-else class="badge" :class="['ready', 'registered'].includes(instanceState(item)) ? 'badge-online' : 'badge-offline'">{{ statusLabel(instanceState(item)) }}</span></td>
              <td class="actions"><button class="btn btn-sm" @click="openDetails(item)">查看与配置</button></td>
            </tr></tbody>
          </table>
        </div>
      </SurfaceCard>
    </div>

    <BaseModal :open="Boolean(detailItem)" :title="detailItem?.name || '实例详情'" size="large" dialog-class="database-detail-dialog" teleport @close="detailItem = null">
      <div v-if="detailItem" class="instance-summary"><div class="instance-symbol"><Database :size="24" /></div><div class="instance-heading"><div class="instance-meta"><span class="badge" :class="detailItem.status === 'ready' || detailItem.status === 'registered' ? 'badge-online' : 'badge-offline'">{{ statusLabel(detailItem.status) }}</span><span>{{ detailItem.mode === 'managed' ? '独立 MySQL' : '外部 MySQL / RDS' }}</span><span v-if="detailItem.node_name">{{ detailItem.node_name }}</span></div><p class="mono" :title="detailItem.host + ':' + detailItem.port">{{ detailItem.host }}:{{ detailItem.port }}</p></div><button class="btn btn-primary btn-sm" @click="terminalItem = detailItem; detailItem = null"><TerminalSquare :size="15" />登录终端</button></div>
      <nav class="table-tabs detail-tabs" aria-label="实例详情"><button v-for="tab in detailTabs" :key="tab.id" class="tab-btn" :class="{ 'tab-active': detailTab === tab.id }" @click="detailTab = tab.id">{{ tab.label }}</button></nav>
      <div v-if="detailItem" class="database-form">
        <template v-if="detailTab === 'overview'">
          <dl class="database-facts overview-facts"><div><dt>管理账号</dt><dd>{{ detailItem.mode === 'managed' ? 'root · 平台保管' : detailItem.admin_username || '未配置' }}</dd></div><div><dt>网络连接</dt><dd>{{ detailItem.private_gateway ? '私网网关' : '直接连接' }}<button v-if="detailItem.private_gateway?.last_error" class="diagnostic-link" @click="actionError = detailItem.private_gateway.last_error">查看诊断</button></dd></div><div v-if="detailItem.mode === 'managed'"><dt>资源上限</dt><dd>{{ detailItem.cpu_limit || '2' }} 核 / {{ detailItem.memory_limit || '2Gi' }}</dd></div><div v-if="detailItem.mode === 'managed'"><dt>数据卷</dt><dd>{{ detailItem.storage_size || '—' }}</dd></div></dl>
          <div class="instance-operation-list">
            <button class="instance-operation" @click="openCapability('inspect', `instance:${detailItem.id}`)"><span><strong>检查实例</strong><span class="cell-secondary">读取版本、业务库与只读状态</span></span><ChevronRight :size="16" /></button>
            <button class="instance-operation" @click="openCapability('migrate', `instance:${detailItem.id}`)"><span><strong>迁移数据</strong><span class="cell-secondary">选择目标实例，创建独立迁移任务</span></span><ChevronRight :size="16" /></button>
            <button class="instance-operation" @click="openCapability('replicate', `instance:${detailItem.id}`)"><span><strong>建立复制</strong><span class="cell-secondary">初始化从库并持续同步</span></span><ChevronRight :size="16" /></button>
            <button v-if="detailItem.mode === 'external'" class="instance-operation" @click="openAdmin(detailItem)"><span><strong>管理凭据</strong><span class="cell-secondary">更新平台保存的实例管理账号</span></span><ChevronRight :size="16" /></button>
          </div>
          <div class="detail-footer"><button class="btn btn-sm" @click="openTasks(`instance:${detailItem.id}`)">查看任务</button><button v-if="detailItem.mode === 'managed'" class="btn btn-sm" @click="inspect(detailItem)">刷新部署状态</button><button v-if="detailItem.mode === 'managed' && ['error', 'missing'].includes(detailItem.status)" class="btn btn-sm" @click="reconcile(detailItem)">重试部署</button><button class="icon-button danger-icon" title="删除实例" aria-label="删除实例" @click="deletingItem = detailItem"><Trash2 :size="15" /></button></div>
        </template>
        <DatabaseMonitor v-else-if="detailTab === 'monitor'" :item="detailItem" @updated="updateDetail" />
        <template v-else-if="detailTab === 'network'">
          <dl class="database-facts"><div><dt>实例地址</dt><dd class="mono">{{ detailItem.host }}:{{ detailItem.port }}</dd></div><div v-if="detailItem.private_gateway"><dt>私网 Service</dt><dd class="mono">{{ detailItem.private_gateway.service_host }}:3306</dd></div><div v-if="detailItem.private_gateway"><dt>网关检查</dt><dd>{{ gatewayStatusLabel(detailItem.private_gateway.status) }}</dd></div></dl>
          <button v-if="detailItem.mode === 'external'" class="btn" @click="gatewayItem = detailItem; detailItem = null">私网网关</button>
        </template>
        <template v-else-if="detailTab === 'applications'">
          <dl class="database-facts"><div><dt>默认业务连接</dt><dd>{{ hasConnection(detailItem) ? detailItem.database_name + ' / ' + detailItem.username : '未配置业务连接' }}</dd></div></dl>
          <div class="capability-actions"><button class="btn" @click="openConnection(detailItem); detailItem = null">业务连接</button><button class="btn" @click="openBindings(detailItem); detailItem = null">应用绑定</button><button class="btn" @click="openCapability('switch', '', `instance:${detailItem.id}`)">切换应用连接</button></div>
        </template>
        <template v-else-if="detailTab === 'replication'">
          <EmptyState v-if="relationsLoading" variant="loading" message="读取复制关系..." />
          <EmptyState v-else-if="!detailRelations.length" message="暂无复制关系" />
          <div v-for="relation in detailRelations" :key="relation.id" class="relation-row"><span>{{ resourceName(relation.source) }} → {{ resourceName(relation.target) }}<span class="cell-secondary">{{ relation.status === 'running' ? '建立时线程已运行，需检查当前状态' : relation.status === 'checking' ? '已创建通道，等待检查线程状态' : '最近一次检查异常' }}</span></span><div class="row-actions"><button class="btn btn-sm" @click="openCapability('replication_check', '', relation.target)">检查</button><button class="btn btn-sm" @click="openCapability('detach', '', relation.target)">解除复制</button></div></div>
        </template>
      </div>
    </BaseModal>

    <DatabaseTerminalDialog :instance="terminalItem" :clusters="clusters" @close="terminalItem = null" />
    <BaseModal :open="capabilitiesOpen" title="数据库操作" size="medium" teleport @close="capabilitiesOpen = false"><div class="capability-menu"><button v-for="item in capabilityOptions" :key="item.kind" class="btn" @click="capabilitiesOpen = false; openCapability(item.kind)">{{ item.label }}</button></div></BaseModal>
    <BaseModal :open="Boolean(adminItem)" title="管理凭据" size="medium" teleport @close="closeAdmin"><div class="database-form"><p class="form-hint">填写实例上已存在的管理账号。用于明确发起的清单检查、迁移与复制任务，与业务账号分开；密码保存后加密保管，不回显。</p><label class="form-group"><span class="form-label">管理账号</span><input v-model.trim="adminForm.username" class="form-input" autocomplete="off"></label><label class="form-group"><span class="form-label">密码</span><input v-model="adminForm.password" class="form-input" type="password" autocomplete="new-password"></label></div><template #actions><button class="btn" :disabled="adminSaving" @click="closeAdmin">取消</button><button class="btn btn-primary" :disabled="adminSaving" @click="saveAdmin">{{ adminSaving ? '保存中...' : '保存管理凭据' }}</button></template></BaseModal>
    <DatabaseCapabilityDialog :kind="capabilityKind" :source="capabilitySource" :target="capabilityTarget" :instances="instances" :mysql-clusters="mysqlClusters" :clusters="clusters" :relations="relations" @close="capabilityKind = ''" @created="capabilityCreated" />
    <DatabaseTaskDialog :open="tasksOpen" :resource="tasksResource" :taskID="selectedTaskID" :resources="resources" :clusters="clusters" @close="tasksOpen = false; refreshInstances()" @updated="refreshInstances" />

    <BaseModal :open="clusterCreateOpen" title="新增 MySQL 集群" size="medium" teleport @close="clusterCreateOpen = false">
      <div class="database-form">
        <label class="form-group"><span class="form-label">集群名称</span><input v-model.trim="clusterForm.name" class="form-input" maxlength="128" placeholder="例如：生产数据库"></label>
        <label class="form-group"><span class="form-label">Kubernetes 集群</span><SelectMenu v-model.number="clusterForm.cluster_id" class="form-select" aria-label="Kubernetes 集群" :options="clusterOptions" /></label>
      </div>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="clusterCreateOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="saving" @click="saveCluster">{{ saving ? '创建中...' : '创建并配置节点' }}</button></template>
    </BaseModal>

    <BaseModal :open="createOpen" title="接入已有 MySQL" size="medium" teleport @close="createOpen = false">
      <form class="database-form" @submit.prevent="save">
        <label class="form-group"><span class="form-label">连接方式</span><SelectMenu v-model="connectionMode" aria-label="连接方式" :options="[{ value: 'direct', label: '直接连接已有地址' }, { value: 'gateway', label: '通过私网网关连接（下一步配置）' }]" /></label>
        <label class="form-group"><span class="form-label">实例名称</span><input v-model.trim="form.name" data-testid="instance-name" class="form-input" required maxlength="128"></label>
        <div class="form-row"><label class="form-group"><span class="form-label">地址</span><input v-model.trim="form.host" data-testid="instance-host" class="form-input" required placeholder="RDS 内网域名或已有 MySQL 地址"></label><label class="form-group narrow"><span class="form-label">端口</span><input v-model.number="form.port" class="form-input" type="number" min="1" max="65535" required></label></div>
        <p class="form-hint">先登记实例和配置网络；应用需要使用时，再配置业务数据库和账号。</p>
      </form>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="createOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="saving" @click="save">{{ saving ? '创建中...' : connectionMode === 'gateway' ? '下一步：配置网关' : '创建' }}</button></template>
    </BaseModal>

    <BaseModal :open="Boolean(connectionItem)" :title="`业务连接 · ${connectionItem?.name || ''}`" size="medium" teleport @close="closeConnection">
      <form class="database-form" @submit.prevent="saveConnection">
        <p class="form-hint">填写已存在的数据库和账号，仅保存连接信息，不会在 MySQL 中创建数据库或账号。修改已绑定的连接前，请先解除应用绑定。</p>
        <label class="form-group"><span class="form-label">数据库名</span><input v-model.trim="connectionForm.database_name" data-testid="connection-database" class="form-input" required maxlength="64" placeholder="app_db"></label>
        <label class="form-group"><span class="form-label">业务用户名</span><input v-model.trim="connectionForm.username" data-testid="connection-username" class="form-input" required maxlength="64" placeholder="app_user"></label>
        <label class="form-group"><span class="form-label">业务密码</span><input v-model="connectionForm.password" data-testid="connection-password" class="form-input" type="password" autocomplete="new-password" required placeholder="填写此账号的现有密码"></label>
      </form>
      <template #actions><button class="btn" type="button" :disabled="connectionSaving" @click="closeConnection">取消</button><button class="btn btn-primary" data-testid="connection-save" type="button" :disabled="connectionSaving" @click="saveConnection">{{ connectionSaving ? '保存中...' : '保存连接' }}</button></template>
    </BaseModal>

    <BaseModal :open="Boolean(bindingItem)" :title="`应用绑定 · ${bindingItem?.name || ''}`" size="medium" teleport @close="bindingItem = null">
      <div class="database-form">
        <template v-if="!hasConnection(bindingItem)"><p class="form-hint">实例已登记。绑定应用前，需要配置业务数据库、用户名和密码；私网网关可以先独立部署和验证。</p><button class="btn" type="button" @click="configureBeforeBinding">配置业务连接</button></template>
        <template v-else>
        <p v-if="bindingItem?.private_gateway" class="form-hint">将绑定私网 Service；如应用启用 TLS，请将证书校验名称配置为 {{ bindingItem.host }}。应用需自行支持此配置。</p>
        <div v-if="bindingsLoading"><EmptyState variant="loading" message="正在读取绑定..." /></div>
        <div v-else-if="bindings.length" class="binding-list">
          <div v-for="binding in bindings" :key="binding.id" class="binding-row"><span>{{ binding.application_name || applicationName(binding.application_id) }} · {{ binding.template_name || `模板 ${binding.template_id}` }}<small class="cell-secondary">{{ binding.prefix }}_*</small></span><button class="icon-button danger-icon" type="button" title="解除绑定" aria-label="解除绑定" @click="unbinding = binding"><Unlink :size="15" /></button></div>
        </div>
        <EmptyState v-else message="尚未绑定应用" />
        <label class="form-group"><span class="form-label">应用</span><SelectMenu v-model.number="bindingForm.application_id" class="form-select" aria-label="应用" :options="applicationOptions" @change="loadTemplates" /></label>
        <label class="form-group"><span class="form-label">上线模板</span><SelectMenu v-model.number="bindingForm.template_id" class="form-select" aria-label="上线模板" :options="templateOptions" /></label>
        <label class="form-group"><span class="form-label">环境变量前缀</span><input v-model.trim="bindingForm.prefix" class="form-input" placeholder="DATABASE"></label>
        </template>
      </div>
      <template #actions><button class="btn" type="button" @click="bindingItem = null">关闭</button><button v-if="hasConnection(bindingItem)" class="btn btn-primary" type="button" :disabled="bindingSaving || !bindingForm.template_id" @click="bind">{{ bindingSaving ? '绑定中...' : '绑定到模板' }}</button></template>
    </BaseModal>

    <ConfirmDialog :open="Boolean(deletingItem)" title="删除数据库实例" :message="deletingItem?.mode === 'managed' ? `将停止并删除 ${deletingItem?.name} 的 MySQL 工作负载、Service 和 Secret。PVC 及其中数据会保留。` : `确定移除 ${deletingItem?.name} 的平台记录？不会删除外部数据库。`" confirm-text="删除" :busy="deleting" @close="deletingItem = null" @cancel="deletingItem = null" @confirm="remove" />
    <ConfirmDialog :open="Boolean(deletingCluster)" title="删除未部署集群" :message="`确定移除 ${deletingCluster?.name || ''} 的逻辑集群记录？`" confirm-text="删除" :busy="deleting" @close="deletingCluster = null" @cancel="deletingCluster = null" @confirm="removeCluster" />
    <ConfirmDialog :open="Boolean(unbinding)" title="解除数据库绑定" message="将从上线模板中移除数据库连接变量；已发布的应用需要重新发布才能生效。" confirm-text="解除绑定" @close="unbinding = null" @cancel="unbinding = null" @confirm="unbind" />
    <DatabaseGatewayDialog :instance="gatewayItem" @close="gatewayItem = null" @updated="refreshInstances" />
    <ErrorNoticeModal :open="Boolean(readError || actionError)" :title="readError ? '读取数据库失败' : '操作失败'" :message="readError || actionError" teleport @close="readError = ''; actionError = ''" />
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ChevronRight, Database, Plus, TerminalSquare, Trash2, Unlink } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import DatabaseTerminalDialog from './DatabaseTerminalDialog.vue'
import DatabaseMonitor from './DatabaseMonitor.vue'
import DatabaseGatewayDialog from './DatabaseGatewayDialog.vue'
import { getApplications, getDeploymentTemplates } from '../../api/applications.js'
import { getClusters } from '../../api/clusters.js'
import { bindDatabase, createDatabase, createMySQLCluster, deleteDatabase, deleteMySQLCluster, getDatabaseBindings, getDatabases, getMySQLClusters, inspectDatabase, reconcileDatabase, unbindDatabase, updateDatabaseConnection, updateDatabaseAdmin, getDatabaseReplications } from '../../api/databases.js'
import DatabaseCapabilityDialog from './DatabaseCapabilityDialog.vue'
import DatabaseTaskDialog from './DatabaseTaskDialog.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const router = useRouter()
const detailItem = ref(null), detailTab = ref('overview'), relations = ref([]), relationsLoading = ref(false)
const terminalItem = ref(null)
const detailTabs = computed(() => [{ id: 'overview', label: '概览' }, ...(detailItem.value?.mode === 'external' ? [{ id: 'monitor', label: '资源监控' }] : []), { id: 'network', label: '网络' }, { id: 'applications', label: '应用' }, { id: 'replication', label: '复制关系' }])
const capabilityOptions = [{ kind: 'deploy', label: '部署独立 MySQL' }, { kind: 'inspect', label: '检查实例' }, { kind: 'migrate', label: '迁移数据' }, { kind: 'replicate', label: '建立复制' }, { kind: 'detach', label: '解除复制' }, { kind: 'replication_check', label: '检查复制' }, { kind: 'switch', label: '切换应用连接' }]
const capabilitiesOpen = ref(false), capabilityKind = ref(''), capabilitySource = ref(''), capabilityTarget = ref('')
const tasksOpen = ref(false), tasksResource = ref(''), selectedTaskID = ref(null)
const adminItem = ref(null), adminSaving = ref(false), adminForm = reactive({ username: '', password: '' })
const resources = computed(() => [...mysqlClusters.value.map(item => ({ ref: `cluster:${item.id}`, name: item.name })), ...instances.value.map(item => ({ ref: `instance:${item.id}`, name: item.name }))])
const resourceName = value => resources.value.find(item => item.ref === value)?.name || value
const detailRelations = computed(() => relations.value.filter(item => [item.source, item.target].includes(`instance:${detailItem.value?.id}`)))
async function loadRelations() { relationsLoading.value = true; try { relations.value = await getDatabaseReplications() } catch (error) { actionError.value = error.message || '读取复制关系失败' } finally { relationsLoading.value = false } }
function updateDetail(item) { Object.assign(detailItem.value, item); const row = instances.value.find(row => row.id === item.id); if (row) Object.assign(row, item) }
function openDetails(item) { detailItem.value = item; detailTab.value = 'overview'; loadRelations() }
function openCapabilities() { capabilitiesOpen.value = true; loadRelations() }
function openCapability(kind, source = '', target = '') { if (['detach', 'replication_check'].includes(kind)) loadRelations(); detailItem.value = null; capabilitySource.value = source; capabilityTarget.value = target; capabilityKind.value = kind }
function openTasks(resource = '', taskID = null) { detailItem.value = null; tasksResource.value = resource; selectedTaskID.value = taskID; tasksOpen.value = true }
async function capabilityCreated(id) { capabilityKind.value = ''; await refreshInstances(); openTasks('', id) }
function openAdmin(item) { adminItem.value = item; detailItem.value = null; adminForm.username = item.admin_username || ''; adminForm.password = '' }
function closeAdmin() { if (adminSaving.value) return; adminForm.password = ''; adminItem.value = null }
async function saveAdmin() { if (adminSaving.value || !adminItem.value) return; adminSaving.value = true; try { await updateDatabaseAdmin(adminItem.value.id, { ...adminForm }); adminForm.password = ''; adminItem.value = null; await refreshInstances() } catch (error) { actionError.value = error.message || '保存管理凭据失败' } finally { adminSaving.value = false } }
const activeTab = ref('clusters')
const gatewayItem = ref(null), connectionMode = ref('direct')
const gatewayStatusLabel = state => ({ ready: '网络已验证', disabled: '已停用', degraded: '服务检查异常', error: '任务失败', pending: '待部署', checking: '测试中', disabling: '停用中' }[state] || '待检查')
async function refreshInstances() { try { instances.value = await getDatabases() } catch (error) { readError.value = error.message || '读取数据库失败' } }
const instances = ref([]), mysqlClusters = ref([]), clusters = ref([]), applications = ref([]), templates = ref([]), bindings = ref([])
const loading = ref(true), readError = ref(''), actionError = ref(''), createOpen = ref(false), saving = ref(false)
const clusterCreateOpen = ref(false), deletingCluster = ref(null)
const clusterForm = reactive({ name: '', cluster_id: 0 })
const bindingItem = ref(null), bindingsLoading = ref(false), bindingSaving = ref(false), deletingItem = ref(null), deleting = ref(false), unbinding = ref(null)
const emptyForm = () => ({ name: '', mode: 'external', host: '', port: 3306 })
const form = reactive(emptyForm())
const connectionItem = ref(null), connectionSaving = ref(false)
const connectionForm = reactive({ database_name: '', username: '', password: '' })
const hasConnection = item => Boolean(item?.database_name && item?.username)
const bindingForm = reactive({ application_id: 0, template_id: 0, prefix: 'DATABASE' })
const clusterOptions = computed(() => [{ value: 0, label: '选择集群' }, ...clusters.value.map(item => ({ value: item.id, label: item.name }))])
const applicationOptions = computed(() => [{ value: 0, label: '选择应用' }, ...applications.value.filter(item => (!bindingItem.value?.private_gateway && bindingItem.value?.mode !== 'managed') || Number(item.environment?.cluster_id) === Number(bindingItem.value?.private_gateway?.cluster_id || bindingItem.value.cluster_id)).map(item => ({ value: item.id, label: `${item.project?.name || '项目'} / ${item.environment?.name || '环境'} / ${item.name}` }))])
const templateOptions = computed(() => [{ value: 0, label: '选择上线模板' }, ...templates.value.map(item => ({ value: item.id, label: item.name }))])
const clusterName = id => clusters.value.find(item => Number(item.id) === Number(id))?.name || `集群 ${id}`
const applicationName = id => applications.value.find(item => Number(item.id) === Number(id))?.name || `应用 ${id}`
const instanceState = item => item.status
const instanceError = item => item.status === 'error' ? item.last_error : ''
const statusLabel = value => ({ pending: '待配置网关', checking: '测试中', disabling: '停用中', disabled: '已停用', degraded: '检查异常', registered: '已登记', creating: '创建中', starting: '启动中', ready: '可用', missing: '资源缺失', error: '失败' }[value] || '待检查')
const clusterStatusLabel = value => ({ draft: '待配置', starting: '部署中', ready: '运行中', missing: '资源缺失', error: '异常', paused: '已暂停' }[value] || '待检查')
async function load() {
  loading.value = true; readError.value = ''
  const results = await Promise.allSettled([getMySQLClusters(), getDatabases(), getClusters(), getApplications()])
  const targets = [mysqlClusters, instances, clusters, applications]
  results.forEach((result, index) => { if (result.status === 'fulfilled') targets[index].value = result.value })
  const failure = results.find(result => result.status === 'rejected')
  if (failure) readError.value = failure.reason?.message || '读取数据库失败'
  loading.value = false
}
function openCreate() { connectionMode.value = 'direct'; Object.assign(form, emptyForm(), { mode: 'external' }); createOpen.value = true }
function openClusterCreate() { Object.assign(clusterForm, { name: '', cluster_id: 0 }); clusterCreateOpen.value = true }
async function saveCluster() {
  if (saving.value) return
  if (!clusterForm.name || !clusterForm.cluster_id) { actionError.value = '请填写名称并选择 Kubernetes 集群'; return }
  saving.value = true
  try { const item = await createMySQLCluster({ ...clusterForm }); clusterCreateOpen.value = false; await router.push(`/cloud-services/databases/clusters/${item.id}`) }
  catch (error) { actionError.value = error.message || '创建集群失败' }
  finally { saving.value = false }
}
async function save() {
  if (saving.value) return
  if (!form.name || !form.host || !Number.isInteger(form.port) || form.port < 1 || form.port > 65535) { actionError.value = '请填写实例名称、有效的地址和端口'; return }
  saving.value = true
  try { const item = await createDatabase({ ...form, connection_mode: connectionMode.value }); createOpen.value = false; if (connectionMode.value === 'gateway') gatewayItem.value = item; await load() }
  catch (error) { actionError.value = error.message || '创建数据库失败'; await load() }
  finally { saving.value = false }
}
function openConnection(item) { Object.assign(connectionForm, { database_name: item.database_name || '', username: item.username || '', password: '' }); connectionItem.value = item }
function closeConnection() { if (connectionSaving.value) return; connectionForm.password = ''; connectionItem.value = null }
function configureBeforeBinding() { const item = bindingItem.value; bindingItem.value = null; openConnection(item) }
async function saveConnection() {
  if (!connectionItem.value || connectionSaving.value) return
  if (!connectionForm.database_name || !connectionForm.username || !connectionForm.password) { actionError.value = '请填写数据库名、用户名和密码'; return }
  connectionSaving.value = true
  try { await updateDatabaseConnection(connectionItem.value.id, { ...connectionForm }); connectionForm.password = ''; connectionItem.value = null; await refreshInstances() }
  catch (error) { actionError.value = error.message || '保存业务连接失败' }
  finally { connectionSaving.value = false }
}
async function inspect(item) { try { const latest = await inspectDatabase(item.id); Object.assign(item, latest) } catch (error) { actionError.value = error.message || '读取状态失败' } }
async function reconcile(item) { try { const latest = await reconcileDatabase(item.id); Object.assign(item, latest); openTasks(`instance:${item.id}`) } catch (error) { actionError.value = error.message || '重试部署失败'; await load() } }
async function remove() { if (!deletingItem.value) return; deleting.value = true; try { await deleteDatabase(deletingItem.value.id); detailItem.value = null; deletingItem.value = null; await load() } catch (error) { actionError.value = error.message || '删除失败' } finally { deleting.value = false } }
async function removeCluster() { if (!deletingCluster.value) return; deleting.value = true; try { await deleteMySQLCluster(deletingCluster.value.id); deletingCluster.value = null; await load() } catch (error) { actionError.value = error.message || '删除失败' } finally { deleting.value = false } }
async function openBindings(item) {
  bindingItem.value = item; bindings.value = []; templates.value = []; Object.assign(bindingForm, { application_id: 0, template_id: 0, prefix: 'DATABASE' }); bindingsLoading.value = true
  if (!hasConnection(item)) { bindingsLoading.value = false; return }
  try { bindings.value = await getDatabaseBindings(item.id) } catch (error) { actionError.value = error.message || '读取绑定失败' } finally { bindingsLoading.value = false }
}
async function loadTemplates() {
  bindingForm.template_id = 0; templates.value = []
  if (!bindingForm.application_id) return
  try { templates.value = await getDeploymentTemplates(bindingForm.application_id) } catch (error) { actionError.value = error.message || '读取上线模板失败' }
}
async function bind() {
  if (!hasConnection(bindingItem.value) || !bindingForm.template_id || bindingSaving.value) return
  bindingSaving.value = true
  try { await bindDatabase(bindingItem.value.id, { ...bindingForm }); bindings.value = await getDatabaseBindings(bindingItem.value.id) }
  catch (error) { actionError.value = error.message || '绑定失败' } finally { bindingSaving.value = false }
}
async function unbind() {
  if (!bindingItem.value || !unbinding.value) return
  try { await unbindDatabase(bindingItem.value.id, unbinding.value.id); bindings.value = await getDatabaseBindings(bindingItem.value.id); unbinding.value = null }
  catch (error) { actionError.value = error.message || '解除绑定失败' }
}
onMounted(load)
</script>

<style scoped>
.instance-summary { display: flex; align-items: center; gap: 14px; padding-bottom: 20px; }.instance-symbol { display: grid; place-items: center; width: 48px; height: 48px; flex: 0 0 auto; border-radius: var(--radius-md); background: var(--surface-hover); color: var(--accent); }.instance-heading { flex: 1; min-width: 0; }.instance-heading p { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin: 8px 0 0; color: var(--text-secondary); font-size: 11px; }.instance-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 11px; color: var(--text-secondary); }.overview-facts { padding-bottom: 16px; border-bottom: 1px solid var(--border-muted); }.instance-operation-list { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 0 20px; }.instance-operation { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 16px 0; border: 0; border-bottom: 1px solid var(--border-muted); background: transparent; color: var(--text-secondary); text-align: left; cursor: pointer; }.instance-operation:hover { color: var(--accent); }.instance-operation strong { display: block; color: var(--text-primary); font-size: 13px; font-weight: 500; margin-bottom: 5px; }.instance-operation .cell-secondary { font-size: 11px; line-height: 1.6; }.detail-footer { display: flex; align-items: center; gap: 8px; margin-top: 16px; }.detail-footer .danger-icon { margin-left: auto; }.diagnostic-link { margin-left: 8px; padding: 0; border: 0; background: transparent; color: var(--warning); font-size: 11px; cursor: pointer; }
@media (max-width:640px) { .instance-summary { flex-wrap: wrap; }.instance-summary > .btn { margin-left: 62px; }.instance-operation-list { grid-template-columns: 1fr; }.detail-tabs { overflow-x: auto; flex-wrap: nowrap; } }

.database-facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; margin: 4px 0 16px; }
.database-facts div { min-width: 0; }.database-facts dt { color: var(--text-secondary); font-size: 11px; margin-bottom: 6px; }.database-facts dd { margin: 0; font-size: 13px; overflow-wrap: anywhere; }
.detail-tabs { margin-bottom: 20px; }.capability-actions { display: flex; flex-wrap: wrap; gap: 8px; }.capability-menu { display: grid; gap: 10px; grid-template-columns: repeat(2, 1fr); }
.relation-row { display: flex; justify-content: space-between; gap: 12px; padding: 12px 0; border-bottom: 1px solid var(--border-muted); font-size: 12px; }.relation-row .cell-secondary { display: block; margin-top: 6px; }
.database-content { margin-top: var(--tabbed-page-content-gap); display: grid; gap: var(--space-16); }
.database-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); min-height: 66px; padding: 14px var(--space-20); }
.table-inset { padding: 0 var(--space-20); }
.database-table { min-width: 900px; }
.database-table .actions { text-align: right; width: 160px; }
.row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.danger-icon { color: var(--danger); }
.status-error { padding: 0; border: 0; background: transparent; cursor: pointer; }
.database-form { display: grid; gap: var(--space-12); }
.database-form .form-group { margin: 0; min-width: 0; }
.database-form .form-row { display: flex; gap: var(--space-12); }
.database-form .form-row > * { flex: 1; }
.database-form .form-row > .narrow { flex: 0 0 110px; }
.mode-switch { display: flex; gap: 8px; margin-bottom: 4px; }
.binding-list { border-block: 1px solid var(--border-muted); }
.binding-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 0; font-size: 12px; }
.binding-row + .binding-row { border-top: 1px solid var(--border-muted); }
.binding-row small { display: block; margin-top: 2px; }
@media (max-width: 760px) { .database-toolbar { align-items: stretch; flex-direction: column; }.database-toolbar .row-actions { flex-wrap: wrap; justify-content: flex-start; }.database-facts { grid-template-columns: 1fr; } .database-toolbar { padding: 14px; }.table-inset { padding: 0 14px; } }
@media (max-width: 480px) { .database-form .form-row { flex-direction: column; }.database-form .form-row > .narrow { flex: 1; } }
</style>
