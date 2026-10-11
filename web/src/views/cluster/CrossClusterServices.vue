<template>
  <div class="cross-services">
    <SurfaceCard padding="none">
      <div class="cross-toolbar">
        <div class="cross-filters">
          <span class="cell-secondary">共 {{ items.length }} 条连接</span>
          <SelectMenu v-model="sourceFilter" :options="sourceFilterOptions" aria-label="筛选来源集群" />
          <SelectMenu v-model="targetFilter" :options="targetFilterOptions" aria-label="筛选目标集群" />
        </div>
        <div class="cross-toolbar-actions">
          <button class="icon-button" type="button" title="刷新跨集群服务" aria-label="刷新跨集群服务" :disabled="loading" @click="load"><RefreshCw :size="15" :class="{ 'is-spinning': loading }" /></button>
          <button class="btn btn-primary" type="button" @click="openCreate"><Plus :size="15" />新增访问</button>
        </div>
      </div>
      <FeedbackBanner v-if="loadError" class="cross-error" tone="warning" :message="loadError" />
      <div v-if="visibleItems.length" class="table-wrap cross-table-wrap"><table class="data-table"><thead><tr><th>来源集群</th><th>访问名称</th><th>目标集群</th><th>目标 Service</th><th>端口</th><th>状态</th><th class="action-cell">操作</th></tr></thead><tbody>
        <tr v-for="item in visibleItems" :key="item.id"><td>{{ clusterName(item.source_cluster_id) }}</td><td>{{ item.source_name }}.{{ item.source_namespace }}.svc.cluster.local</td><td>{{ clusterName(item.target_cluster_id) }}</td><td>{{ item.target_namespace }}/{{ item.target_name }}</td><td>{{ item.source_port }} → {{ item.target_service_port }}</td><td><span class="badge" :class="item.status === 'active' ? 'badge-online' : 'badge-danger'">{{ item.status === 'active' ? '已配置' : '配置失败' }}</span><span v-if="checks[item.id]" class="cell-secondary">{{ checks[item.id].message }}</span><span v-else-if="item.last_error" class="cell-secondary">{{ item.last_error }}</span></td><td class="action-cell"><div class="cross-actions"><button class="icon-button icon-button--table" type="button" :title="`检测 ${item.source_name}`" :aria-label="`检测 ${item.source_name}`" :disabled="busyID === item.id" @click="check(item)"><Activity :size="14" /></button><button class="icon-button icon-button--table" type="button" :title="`删除 ${item.source_name}`" :aria-label="`删除 ${item.source_name}`" :disabled="busyID === item.id" @click="deleteTarget = item"><Trash2 :size="14" /></button></div></td></tr>
      </tbody></table></div>
      <EmptyState v-else-if="loading" variant="loading" message="正在读取跨集群服务" />
      <EmptyState v-else-if="!loadError" :message="items.length ? '没有符合条件的跨集群服务' : '暂无跨集群服务'" />
    </SurfaceCard>

    <BaseModal :open="createOpen" title="新增跨集群访问" teleport :show-close="!saving" :close-on-overlay="!saving" :close-on-escape="!saving" @close="closeCreate">
      <form id="cross-service-form" class="cross-form" @submit.prevent="create">
        <div class="form-group"><label class="form-label" for="cross-source-cluster">来源集群</label><SelectMenu id="cross-source-cluster" v-model="form.source_cluster_id" :options="readyClusterOptions" placeholder="选择来源集群" aria-label="来源集群" /></div>
        <div class="form-group"><label class="form-label" for="cross-target-cluster">目标集群</label><SelectMenu id="cross-target-cluster" v-model="form.target_cluster_id" :options="targetClusterOptions" :disabled="!form.source_cluster_id" placeholder="选择目标集群" aria-label="目标集群" /></div>
        <div v-if="form.target_cluster_id" class="cross-precheck"><span v-if="prechecking" class="cell-secondary">正在检查两个集群的组网与 Agent...</span><span v-else-if="precheck?.ready" class="badge badge-online">组网预检通过</span><span v-else class="cell-secondary">{{ precheck?.message || precheckError || '等待预检' }}</span><RouterLink v-if="!prechecking && !precheck?.ready" :to="`/cluster/${form.source_cluster_id}?tab=networking`" @click="closeCreate">查看来源组网</RouterLink><RouterLink v-if="!prechecking && !precheck?.ready" :to="`/cluster/${form.target_cluster_id}?tab=networking`" @click="closeCreate">查看目标组网</RouterLink></div>
        <div class="form-group"><label class="form-label" for="cross-target-service">目标 Service</label><SelectMenu id="cross-target-service" v-model="form.target_service" :options="targetServiceOptions" :disabled="!form.target_cluster_id || serviceLoading" :placeholder="serviceLoading ? '读取中...' : '选择目标 Service'" aria-label="目标 Service" /></div>
        <div class="form-group"><label class="form-label" for="cross-target-port">目标 TCP 端口</label><SelectMenu id="cross-target-port" v-model="form.target_service_port" :options="portOptions" :disabled="!form.target_service" placeholder="选择端口" aria-label="目标 TCP 端口" /></div>
        <div class="form-group"><label class="form-label" for="cross-source-namespace">来源命名空间</label><SelectMenu id="cross-source-namespace" v-model="form.source_namespace" :options="sourceNamespaceOptions" :disabled="!form.source_cluster_id" placeholder="选择命名空间" aria-label="来源命名空间" /></div>
        <div class="cross-form-row"><div class="form-group"><label class="form-label" for="cross-source-name">访问名称</label><input id="cross-source-name" v-model.trim="form.source_name" class="form-input" maxlength="57" placeholder="orders-b" required pattern="[a-z]([-a-z0-9]*[a-z0-9])?" /></div><div class="form-group"><label class="form-label" for="cross-source-port">访问端口</label><input id="cross-source-port" v-model.number="form.source_port" class="form-input" type="number" min="1" max="65535" required /></div></div>
        <span v-if="form.source_name && form.source_namespace" class="cell-secondary">访问地址：{{ form.source_name }}.{{ form.source_namespace }}.svc.cluster.local:{{ form.source_port || '端口' }}</span>
      </form>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="closeCreate">取消</button><button class="btn btn-primary" type="submit" form="cross-service-form" :disabled="!canCreate || saving">{{ saving ? '创建中...' : '创建访问' }}</button></template>
    </BaseModal>
    <BaseModal :open="Boolean(deleteTarget)" title="删除跨集群服务" size="small" teleport @close="deleteTarget = null"><p>删除 {{ deleteTarget?.source_namespace }}/{{ deleteTarget?.source_name }} 的访问入口和服务名称？</p><template #actions><button class="btn" type="button" :disabled="Boolean(busyID)" @click="deleteTarget = null">取消</button><button class="btn btn-danger" type="button" :disabled="Boolean(busyID)" @click="remove">确认删除</button></template></BaseModal>
    <ErrorNoticeModal :open="Boolean(operationError)" :message="operationError" teleport @close="operationError = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Activity, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'
import { checkCrossClusterService, createCrossClusterService, deleteCrossClusterService, getAllCrossClusterServices, getClusters, precheckCrossClusterService } from '../../api/clusters.js'
import { createClusterKubernetesApi } from '../../api/kubernetes.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const items = ref([])
const clusters = ref([])
const sourceFilter = ref('')
const targetFilter = ref('')
const sourceNamespaces = ref([])
const targetServices = ref([])
const loading = ref(false)
const loadError = ref('')
const operationError = ref('')
const createOpen = ref(false)
const saving = ref(false)
const busyID = ref(0)
const deleteTarget = ref(null)
const checks = ref({})
const precheck = ref(null)
const precheckError = ref('')
const prechecking = ref(false)
const serviceLoading = ref(false)
const form = ref(emptyForm())
const readyClusterOptions = computed(() => clusters.value.filter(cluster => cluster.status === 'ready').map(cluster => ({ value: String(cluster.id), label: cluster.name })))
const targetClusterOptions = computed(() => readyClusterOptions.value.filter(cluster => cluster.value !== form.value.source_cluster_id))
const sourceFilterOptions = computed(() => [{ value: '', label: '全部来源集群' }, ...clusters.value.map(cluster => ({ value: String(cluster.id), label: cluster.name }))])
const targetFilterOptions = computed(() => [{ value: '', label: '全部目标集群' }, ...clusters.value.map(cluster => ({ value: String(cluster.id), label: cluster.name }))])
const visibleItems = computed(() => items.value.filter(item => (!sourceFilter.value || String(item.source_cluster_id) === sourceFilter.value) && (!targetFilter.value || String(item.target_cluster_id) === targetFilter.value)))
const sourceNamespaceOptions = computed(() => sourceNamespaces.value.map(item => ({ value: item.name, label: item.name })))
const targetServiceOptions = computed(() => targetServices.value.filter(item => item.cluster_ip && item.cluster_ip !== 'None' && item.ports?.some(port => port.startsWith('TCP:'))).map(item => ({ value: `${item.namespace}/${item.name}`, label: `${item.namespace} / ${item.name}` })))
const selectedService = computed(() => targetServices.value.find(item => `${item.namespace}/${item.name}` === form.value.target_service))
const portOptions = computed(() => (selectedService.value?.ports || []).filter(port => port.startsWith('TCP:')).map(port => ({ value: port.slice(4), label: port })))
const canCreate = computed(() => precheck.value?.ready && form.value.source_cluster_id && form.value.target_cluster_id && form.value.target_service && form.value.target_service_port && form.value.source_namespace && /^[a-z]([-a-z0-9]*[a-z0-9])?$/.test(form.value.source_name) && form.value.source_port >= 1 && form.value.source_port <= 65535)
let loadController
let optionController
let namespaceController
let optionRequestID = 0

function emptyForm() { return { source_cluster_id: '', target_cluster_id: '', target_service: '', target_service_port: '', source_namespace: '', source_name: '', source_port: 0 } }
function clusterName(id) { return clusters.value.find(cluster => Number(cluster.id) === Number(id))?.name || `集群 #${id}` }

async function load() {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  loading.value = true
  loadError.value = ''
  try {
    const [records, clusterList] = await Promise.all([getAllCrossClusterServices({ signal: controller.signal }), getClusters({ signal: controller.signal })])
    if (!controller.signal.aborted) { items.value = records || []; clusters.value = clusterList || [] }
  } catch (error) { if (!controller.signal.aborted) loadError.value = error.message || '读取跨集群服务失败' }
  finally { if (!controller.signal.aborted) loading.value = false }
}

function openCreate() {
  form.value = emptyForm()
  precheck.value = null
  precheckError.value = ''
  sourceNamespaces.value = []
  targetServices.value = []
  createOpen.value = true
}

function closeCreate() { if (!saving.value) { createOpen.value = false; optionController?.abort(); namespaceController?.abort() } }

watch(() => form.value.source_cluster_id, async sourceID => {
  namespaceController?.abort()
  form.value.target_cluster_id = ''
  form.value.source_namespace = ''
  sourceNamespaces.value = []
  if (!sourceID || !createOpen.value) return
  const controller = new AbortController()
  namespaceController = controller
  try {
    const result = await createClusterKubernetesApi(sourceID).getNamespaceNames({ signal: controller.signal })
    if (!controller.signal.aborted) sourceNamespaces.value = result || []
  } catch (error) { if (!controller.signal.aborted) operationError.value = error.message || '读取来源命名空间失败' }
})

watch(() => form.value.target_cluster_id, async targetID => {
  optionController?.abort()
  const controller = new AbortController()
  optionController = controller
  const requestID = ++optionRequestID
  form.value.target_service = ''
  form.value.target_service_port = ''
  targetServices.value = []
  precheck.value = null
  precheckError.value = ''
  if (!targetID || !form.value.source_cluster_id || !createOpen.value) return
  prechecking.value = true
  serviceLoading.value = true
  void precheckCrossClusterService(form.value.source_cluster_id, targetID, { signal: controller.signal }).then(result => { if (requestID === optionRequestID && !controller.signal.aborted) precheck.value = result }).catch(error => { if (requestID === optionRequestID && !controller.signal.aborted) precheckError.value = error.message || '预检失败' }).finally(() => { if (requestID === optionRequestID) prechecking.value = false })
  void createClusterKubernetesApi(targetID).getLightweightServiceDiscovery('', { signal: controller.signal }).then(result => { if (requestID === optionRequestID && !controller.signal.aborted) targetServices.value = result || [] }).catch(error => { if (requestID === optionRequestID && !controller.signal.aborted) operationError.value = error.message || '读取目标 Service 失败' }).finally(() => { if (requestID === optionRequestID) serviceLoading.value = false })
})

watch(() => form.value.target_service, () => {
  form.value.target_service_port = portOptions.value.length === 1 ? portOptions.value[0].value : ''
  form.value.source_port = Number(form.value.target_service_port) || 0
})
watch(() => form.value.target_service_port, value => { if (value) form.value.source_port = Number(value) })

async function create() {
  if (!canCreate.value || saving.value) return
  saving.value = true
  const [target_namespace, target_name] = form.value.target_service.split('/')
  try {
    await createCrossClusterService(form.value.source_cluster_id, { target_cluster_id: Number(form.value.target_cluster_id), target_namespace, target_name, target_service_port: Number(form.value.target_service_port), source_namespace: form.value.source_namespace, source_name: form.value.source_name, source_port: Number(form.value.source_port) })
    createOpen.value = false
    await load()
  } catch (error) { operationError.value = error.message || '创建跨集群服务失败'; await load() }
  finally { saving.value = false }
}

async function check(item) {
  if (busyID.value) return
  busyID.value = item.id
  try { checks.value = { ...checks.value, [item.id]: await checkCrossClusterService(item.source_cluster_id, item.id) } }
  catch (error) { operationError.value = error.message || '检测连接失败' }
  finally { busyID.value = 0 }
}

async function remove() {
  if (!deleteTarget.value || busyID.value) return
  busyID.value = deleteTarget.value.id
  try { await deleteCrossClusterService(deleteTarget.value.source_cluster_id, deleteTarget.value.id); deleteTarget.value = null; await load() }
  catch (error) { operationError.value = error.message || '删除跨集群服务失败'; await load() }
  finally { busyID.value = 0 }
}

void load()
onBeforeUnmount(() => { loadController?.abort(); optionController?.abort(); namespaceController?.abort() })
</script>

<style scoped>
.cross-toolbar{display:flex;flex-wrap:wrap;min-height:62px;align-items:center;justify-content:space-between;gap:12px;padding:12px 20px}
.cross-filters{display:flex;flex:1 1 470px;flex-wrap:wrap;align-items:center;gap:8px;min-width:0}
.cross-filters>.cell-secondary{flex:none;margin-right:12px;white-space:nowrap}
.cross-filters :deep(.select-menu){flex:1 1 160px;min-width:150px;max-width:220px}
.cross-toolbar-actions,.cross-actions{display:flex;align-items:center;gap:8px}
.cross-toolbar-actions{flex:none;margin-left:auto}
.cross-actions{justify-content:flex-end}
.cross-table-wrap{overflow-x:auto;padding:12px 20px}
.cross-table-wrap .data-table{min-width:760px}
.cross-table-wrap td:nth-child(2){white-space:nowrap}
.cross-error{margin:0 20px 12px}
.cross-form{display:grid;gap:12px}
.cross-form .form-group{display:grid;gap:6px;margin:0}
.cross-form-row{display:grid;grid-template-columns:minmax(0,2fr) minmax(110px,1fr);gap:12px}
.cross-precheck{display:flex;align-items:center;flex-wrap:wrap;gap:8px;font-size:12px}
.cross-precheck a{color:var(--action-primary)}
@media(max-width:640px){.cross-toolbar,.cross-table-wrap{padding-right:14px;padding-left:14px}.cross-toolbar,.cross-filters{align-items:stretch;flex-direction:column}.cross-filters{flex-basis:auto}.cross-filters :deep(.select-menu){flex:none;width:100%;max-width:none}.cross-form-row{grid-template-columns:1fr}}
</style>
