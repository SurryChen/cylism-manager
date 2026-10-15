<template>
  <section class="page-shell">
    <SectionTabsHeader title="数据库" :tabs="[{ id: 'clusters', label: 'MySQL 集群' }, { id: 'instances', label: '已有实例' }]" :active-tab="activeTab" @select="activeTab = $event" />
    <div class="database-content">
      <SurfaceCard v-if="activeTab === 'clusters'" padding="none">
        <div class="database-toolbar">
          <span class="cell-secondary">{{ loading ? '读取中...' : `共 ${mysqlClusters.length} 个集群` }}</span>
          <button class="btn btn-primary" type="button" @click="openClusterCreate"><Plus :size="16" />新增 MySQL 集群</button>
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
          <button class="btn btn-primary" type="button" @click="openCreate"><Plus :size="16" />接入已有 MySQL</button>
        </div>
        <EmptyState v-if="loading" variant="loading" message="正在读取数据库..." />
        <EmptyState v-else-if="!instances.length" message="暂无数据库实例" />
        <div v-else class="table-wrap table-inset">
          <table class="data-table database-table">
            <thead><tr><th>实例</th><th>来源</th><th>集群</th><th>地址</th><th>业务数据库</th><th>状态</th><th class="actions">操作</th></tr></thead>
            <tbody>
              <tr v-for="item in instances" :key="item.id">
                <td class="cell-primary">{{ item.name }}</td>
                <td>{{ item.mode === 'managed' ? '集群部署' : '已有 MySQL' }}</td>
                <td>{{ item.mode === 'managed' ? (clusterName(item.cluster_id)) : '-' }}</td>
                <td class="mono">{{ item.host }}:{{ item.port }}</td>
                <td class="mono">{{ item.database_name }}</td>
                <td><button v-if="item.status === 'error' && item.last_error" class="status-error" type="button" @click="actionError = item.last_error"><span class="badge badge-danger">失败</span></button><span v-else class="badge" :class="item.status === 'ready' || item.status === 'registered' ? 'badge-online' : 'badge-offline'">{{ statusLabel(item.status) }}</span></td>
                <td class="actions"><div class="row-actions">
                  <button class="btn btn-sm" type="button" @click="openBindings(item)"><Link2 :size="14" />应用绑定</button>
                  <button v-if="item.mode === 'managed'" class="icon-button" type="button" title="刷新状态" aria-label="刷新状态" @click="inspect(item)"><RefreshCw :size="15" /></button>
                  <button v-if="item.mode === 'managed' && (item.status === 'error' || item.status === 'missing')" class="btn btn-sm" type="button" @click="reconcile(item)">重试部署</button>
                  <button class="icon-button danger-icon" type="button" title="删除实例" aria-label="删除实例" @click="deletingItem = item"><Trash2 :size="15" /></button>
                </div></td>
              </tr>
            </tbody>
          </table>
        </div>
      </SurfaceCard>
    </div>

    <BaseModal :open="clusterCreateOpen" title="新增 MySQL 集群" size="medium" teleport @close="clusterCreateOpen = false">
      <div class="database-form">
        <label class="form-group"><span class="form-label">集群名称</span><input v-model.trim="clusterForm.name" class="form-input" maxlength="128" placeholder="例如：生产数据库"></label>
        <label class="form-group"><span class="form-label">Kubernetes 集群</span><SelectMenu v-model.number="clusterForm.cluster_id" class="form-select" aria-label="Kubernetes 集群" :options="clusterOptions" /></label>
      </div>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="clusterCreateOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="saving" @click="saveCluster">{{ saving ? '创建中...' : '创建并配置节点' }}</button></template>
    </BaseModal>

    <BaseModal :open="createOpen" title="接入已有 MySQL" size="medium" teleport @close="createOpen = false">
      <form class="database-form" @submit.prevent="save">
        <label class="form-group"><span class="form-label">实例名称</span><input v-model.trim="form.name" class="form-input" required maxlength="128"></label>
        <div class="form-row"><label class="form-group"><span class="form-label">地址</span><input v-model.trim="form.host" class="form-input" required placeholder="mysql.example.internal"></label><label class="form-group narrow"><span class="form-label">端口</span><input v-model.number="form.port" class="form-input" type="number" min="1" max="65535" required></label></div>
        <div class="form-row"><label class="form-group"><span class="form-label">数据库名</span><input v-model.trim="form.database_name" class="form-input" required placeholder="app_db"></label><label class="form-group"><span class="form-label">业务用户名</span><input v-model.trim="form.username" class="form-input" required placeholder="app_user"></label></div>
        <label class="form-group"><span class="form-label">业务密码</span><input v-model="form.password" class="form-input" type="password" autocomplete="new-password" required></label>
      </form>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="createOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="saving" @click="save">{{ saving ? '创建中...' : '创建' }}</button></template>
    </BaseModal>

    <BaseModal :open="Boolean(bindingItem)" :title="`应用绑定 · ${bindingItem?.name || ''}`" size="medium" teleport @close="bindingItem = null">
      <div class="database-form">
        <div v-if="bindingsLoading"><EmptyState variant="loading" message="正在读取绑定..." /></div>
        <div v-else-if="bindings.length" class="binding-list">
          <div v-for="binding in bindings" :key="binding.id" class="binding-row"><span>{{ binding.application_name || applicationName(binding.application_id) }} · {{ binding.template_name || `模板 ${binding.template_id}` }}<small class="cell-secondary">{{ binding.prefix }}_*</small></span><button class="icon-button danger-icon" type="button" title="解除绑定" aria-label="解除绑定" @click="unbinding = binding"><Unlink :size="15" /></button></div>
        </div>
        <EmptyState v-else message="尚未绑定应用" />
        <label class="form-group"><span class="form-label">应用</span><SelectMenu v-model.number="bindingForm.application_id" class="form-select" aria-label="应用" :options="applicationOptions" @change="loadTemplates" /></label>
        <label class="form-group"><span class="form-label">上线模板</span><SelectMenu v-model.number="bindingForm.template_id" class="form-select" aria-label="上线模板" :options="templateOptions" /></label>
        <label class="form-group"><span class="form-label">环境变量前缀</span><input v-model.trim="bindingForm.prefix" class="form-input" placeholder="DATABASE"></label>
      </div>
      <template #actions><button class="btn" type="button" @click="bindingItem = null">关闭</button><button class="btn btn-primary" type="button" :disabled="bindingSaving || !bindingForm.template_id" @click="bind">{{ bindingSaving ? '绑定中...' : '绑定到模板' }}</button></template>
    </BaseModal>

    <ConfirmDialog :open="Boolean(deletingItem)" title="删除数据库实例" :message="deletingItem?.mode === 'managed' ? `将停止并删除 ${deletingItem?.name} 的 MySQL 工作负载、Service 和 Secret。PVC 及其中数据会保留。` : `确定移除 ${deletingItem?.name} 的平台记录？不会删除外部数据库。`" confirm-text="删除" :busy="deleting" @close="deletingItem = null" @cancel="deletingItem = null" @confirm="remove" />
    <ConfirmDialog :open="Boolean(deletingCluster)" title="删除未部署集群" :message="`确定移除 ${deletingCluster?.name || ''} 的逻辑集群记录？`" confirm-text="删除" :busy="deleting" @close="deletingCluster = null" @cancel="deletingCluster = null" @confirm="removeCluster" />
    <ConfirmDialog :open="Boolean(unbinding)" title="解除数据库绑定" message="将从上线模板中移除数据库连接变量；已发布的应用需要重新发布才能生效。" confirm-text="解除绑定" @close="unbinding = null" @cancel="unbinding = null" @confirm="unbind" />
    <ErrorNoticeModal :open="Boolean(readError || actionError)" :title="readError ? '读取数据库失败' : '操作失败'" :message="readError || actionError" teleport @close="readError = ''; actionError = ''" />
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Link2, Plus, RefreshCw, Trash2, Unlink } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { getApplications, getDeploymentTemplates } from '../../api/applications.js'
import { getClusters } from '../../api/clusters.js'
import { bindDatabase, createDatabase, createMySQLCluster, deleteDatabase, deleteMySQLCluster, getDatabaseBindings, getDatabases, getMySQLClusters, inspectDatabase, reconcileDatabase, unbindDatabase } from '../../api/databases.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const router = useRouter()
const activeTab = ref('clusters')
const instances = ref([]), mysqlClusters = ref([]), clusters = ref([]), applications = ref([]), templates = ref([]), bindings = ref([])
const loading = ref(true), readError = ref(''), actionError = ref(''), createOpen = ref(false), saving = ref(false)
const clusterCreateOpen = ref(false), deletingCluster = ref(null)
const clusterForm = reactive({ name: '', cluster_id: 0 })
const bindingItem = ref(null), bindingsLoading = ref(false), bindingSaving = ref(false), deletingItem = ref(null), deleting = ref(false), unbinding = ref(null)
const emptyForm = () => ({ name: '', mode: 'managed', cluster_id: 0, host: '', port: 3306, database_name: '', username: '', password: '', storage_size: '10Gi', storage_class: '', image: 'mysql:8.4' })
const form = reactive(emptyForm())
const bindingForm = reactive({ application_id: 0, template_id: 0, prefix: 'DATABASE' })
const clusterOptions = computed(() => [{ value: 0, label: '选择集群' }, ...clusters.value.map(item => ({ value: item.id, label: item.name }))])
const applicationOptions = computed(() => [{ value: 0, label: '选择应用' }, ...applications.value.filter(item => bindingItem.value?.mode !== 'managed' || Number(item.environment?.cluster_id) === Number(bindingItem.value.cluster_id)).map(item => ({ value: item.id, label: `${item.project?.name || '项目'} / ${item.environment?.name || '环境'} / ${item.name}` }))])
const templateOptions = computed(() => [{ value: 0, label: '选择上线模板' }, ...templates.value.map(item => ({ value: item.id, label: item.name }))])
const clusterName = id => clusters.value.find(item => Number(item.id) === Number(id))?.name || `集群 ${id}`
const applicationName = id => applications.value.find(item => Number(item.id) === Number(id))?.name || `应用 ${id}`
const statusLabel = value => ({ registered: '已登记', creating: '创建中', starting: '启动中', ready: '运行中', missing: '资源缺失', error: '失败' }[value] || '待检查')
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
function openCreate() { Object.assign(form, emptyForm(), { mode: 'external' }); createOpen.value = true }
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
  if (!form.name || !form.database_name || !form.username || !form.password || (form.mode === 'managed' ? !form.cluster_id : !form.host)) { actionError.value = '请填写完整的数据库配置'; return }
  saving.value = true
  try { await createDatabase({ ...form }); createOpen.value = false; await load() }
  catch (error) { actionError.value = error.message || '创建数据库失败'; await load() }
  finally { saving.value = false }
}
async function inspect(item) { try { const latest = await inspectDatabase(item.id); Object.assign(item, latest) } catch (error) { actionError.value = error.message || '读取状态失败' } }
async function reconcile(item) { try { const latest = await reconcileDatabase(item.id); Object.assign(item, latest) } catch (error) { actionError.value = error.message || '重试部署失败'; await load() } }
async function remove() { if (!deletingItem.value) return; deleting.value = true; try { await deleteDatabase(deletingItem.value.id); deletingItem.value = null; await load() } catch (error) { actionError.value = error.message || '删除失败' } finally { deleting.value = false } }
async function removeCluster() { if (!deletingCluster.value) return; deleting.value = true; try { await deleteMySQLCluster(deletingCluster.value.id); deletingCluster.value = null; await load() } catch (error) { actionError.value = error.message || '删除失败' } finally { deleting.value = false } }
async function openBindings(item) {
  bindingItem.value = item; bindings.value = []; templates.value = []; Object.assign(bindingForm, { application_id: 0, template_id: 0, prefix: 'DATABASE' }); bindingsLoading.value = true
  try { bindings.value = await getDatabaseBindings(item.id) } catch (error) { actionError.value = error.message || '读取绑定失败' } finally { bindingsLoading.value = false }
}
async function loadTemplates() {
  bindingForm.template_id = 0; templates.value = []
  if (!bindingForm.application_id) return
  try { templates.value = await getDeploymentTemplates(bindingForm.application_id) } catch (error) { actionError.value = error.message || '读取上线模板失败' }
}
async function bind() {
  if (!bindingItem.value || !bindingForm.template_id || bindingSaving.value) return
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
.database-content { margin-top: var(--tabbed-page-content-gap); display: grid; gap: var(--space-16); }
.database-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); min-height: 66px; padding: 14px var(--space-20); }
.table-inset { padding: 0 var(--space-20); }
.database-table { min-width: 900px; }
.database-table .actions { text-align: right; width: 240px; }
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
@media (max-width: 760px) { .database-toolbar { padding: 14px; }.table-inset { padding: 0 14px; } }
@media (max-width: 480px) { .database-form .form-row { flex-direction: column; }.database-form .form-row > .narrow { flex: 1; } }
</style>
