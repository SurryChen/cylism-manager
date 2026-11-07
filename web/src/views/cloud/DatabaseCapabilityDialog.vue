<template>
  <BaseModal :open="Boolean(kind)" :title="labels[kind] || '数据库操作'" size="large" teleport @close="close">
    <div class="capability-form">
      <label v-if="!['deploy', 'detach', 'replication_check', 'switch'].includes(kind)" class="form-group"><span class="form-label">源数据库</span><SelectMenu v-model="form.source" aria-label="源数据库" :options="sourceOptions" /></label>
      <template v-if="kind === 'deploy'">
        <label class="form-group"><span class="form-label">实例名称</span><input v-model.trim="form.name" class="form-input" maxlength="128" placeholder="例如：独立 MySQL"></label>
        <div class="form-row"><label class="form-group"><span class="form-label">MySQL 镜像</span><input v-model.trim="form.mysql_image" class="form-input" placeholder="mysql:8.4"></label><label class="form-group"><span class="form-label">资源上限</span><SelectMenu v-model="form.resources" aria-label="资源上限" :options="resourceOptions" /></label></div>
        <p class="form-hint">创建单实例 StatefulSet 和独立数据卷。部署后可单独建库或作为复制目标，不依赖 Oracle Operator。</p>
      </template>
      <label v-else-if="kind !== 'inspect'" class="form-group"><span class="form-label">{{ ['detach', 'replication_check'].includes(kind) ? '现有复制目标' : '目标数据库' }}</span><SelectMenu v-model="form.target" aria-label="目标数据库" :options="targetOptions" /></label>
      <div class="form-row">
        <label class="form-group"><span class="form-label">{{ kind === 'deploy' ? '部署集群' : '执行集群' }}</span><SelectMenu v-model="form.cluster_id" aria-label="执行集群" :options="clusterOptions" /></label>
        <label class="form-group"><span class="form-label">{{ kind === 'deploy' ? '部署节点' : '执行节点' }}</span><SelectMenu v-model="form.node_name" aria-label="执行节点" :disabled="nodesLoading" :options="nodeOptions" /></label>
      </div>
      <template v-if="['deploy', 'migrate', 'replicate'].includes(kind)">
        <div class="form-row"><label class="form-group"><span class="form-label">{{ kind === 'deploy' ? '数据卷容量' : '快照临时卷容量' }}</span><input v-model.trim="form.storage_size" class="form-input" placeholder="10Gi"></label><label class="form-group"><span class="form-label">存储类型</span><input v-model.trim="form.storage_class" class="form-input" placeholder="留空使用集群默认值"></label></div>
      </template>
      <template v-if="kind === 'migrate'">
        <label class="form-group"><span class="form-label">迁移数据库</span><input v-model="form.schemas" class="form-input" placeholder="app_db, another_db（不包含系统库和账号）"></label>
        <p class="form-hint">导出一致性快照并导入所选数据库，目标不能已有同名库，并须开启 local_infile。完成后需另行准备业务账号、切换应用并核验。两端需为同一 MySQL 8.0 或 8.4 系列。</p>
        <label class="check-line"><input v-model="form.writes_stopped" type="checkbox">已停止所选源库的业务写入，将保持停写至切换完成</label>
      </template>
      <template v-if="kind === 'replicate'">
        <p class="form-hint">使用完整业务快照初始化所选独立 MySQL，再建立 GTID 异步复制。目标将保持只读。</p>
        <div class="form-row"><label class="form-group"><span class="form-label">源库复制账号</span><input v-model.trim="form.replication_user" class="form-input" autocomplete="off"></label><label class="form-group"><span class="form-label">复制账号密码</span><input v-model="form.replication_password" class="form-input" type="password" autocomplete="new-password"></label></div>
        <details class="execution-options"><summary>前置条件与复制范围</summary><p class="form-hint">两端需为同一版本系列。目标须为空、没有 GTID 历史或应用绑定。源库须开启 binlog/GTID，预先创建复制账号并允许目标访问。</p><p class="form-hint">初始化不复制系统账号；后续原生复制会执行源库 binlog 中的变更，账号与授权变更需按 MySQL 复制规则管理。此功能不提供自动故障切换。</p></details>
      </template>
      <p v-if="kind === 'detach'" class="form-hint">只停止并删除平台建立的指定复制通道，保留两个实例和数据。目标保持只读；此操作不等同于切主。</p>
      <p v-if="kind === 'inspect'" class="form-hint">在所选节点验证 TLS 管理连接，读取版本、业务库清单和只读状态，不要求所有节点检查通过。</p>
      <template v-if="kind === 'switch'">
        <p class="form-hint">只更新选中的已绑定上线模板。每个模板单独验证目标业务账号，保存后需到应用页面发布。迁移数据和创建账号分别执行。</p>
        <EmptyState v-if="bindingsLoading" variant="loading" message="读取应用绑定..." />
        <EmptyState v-else-if="!bindingRows.length" message="暂无应用绑定；先通过实例详情绑定上线模板" />
        <div v-else class="switch-list"><div v-for="binding in bindingRows" :key="binding.id" class="switch-row">
          <label class="check-line"><input v-model="binding.selected" type="checkbox">{{ binding.application_name }} · {{ binding.template_name }}<span class="cell-secondary">当前：{{ binding.instance_name }}</span></label>
          <div v-if="binding.selected" class="form-row"><input v-model.trim="binding.database" class="form-input" aria-label="目标数据库名" placeholder="目标数据库名"><input v-model.trim="binding.username" class="form-input" aria-label="目标业务账号" placeholder="目标业务账号"><input v-model="binding.password" class="form-input" type="password" aria-label="目标业务密码" placeholder="目标业务密码" autocomplete="new-password"></div>
        </div></div>
      </template>
      <details v-if="kind !== 'deploy'" class="execution-options"><summary>执行选项</summary><label class="form-group"><span class="form-label">MySQL Shell 镜像</span><input v-model.trim="form.image" class="form-input" placeholder="使用默认 MySQL Shell 工具镜像"></label><p class="form-hint">镜像必须包含 mysqlsh；管理权限不足会保留任务与目标数据供检查。阶段没有总时限，暂停在当前阶段完成后生效。</p></details>
    </div>
    <template #actions><button class="btn" :disabled="saving" @click="close">取消</button><button class="btn btn-primary" :disabled="saving || nodesLoading" @click="submit">{{ saving ? '提交中...' : '创建后台任务' }}</button></template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { createDatabase, createDatabaseOperation, getDatabaseBindings, getDatabaseExecutionNodes } from '../../api/databases.js'
import BaseModal from '../../components/BaseModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import EmptyState from '../../components/EmptyState.vue'

const props = defineProps({ kind: { type: String, default: '' }, source: { type: String, default: '' }, target: { type: String, default: '' }, instances: { type: Array, default: () => [] }, mysqlClusters: { type: Array, default: () => [] }, clusters: { type: Array, default: () => [] }, relations: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'created'])
const labels = { deploy: '部署独立 MySQL', inspect: '检查实例', migrate: '迁移数据', replicate: '建立主从复制', detach: '解除复制', replication_check: '检查复制', switch: '切换应用连接' }
const initial = () => ({ name: '', source: '', target: '', cluster_id: 0, node_name: '', storage_size: '10Gi', storage_class: '', resources: '1/1Gi', mysql_image: 'mysql:8.4', schemas: '', writes_stopped: false, replication_user: '', replication_password: '', image: '' })
const form = reactive(initial()), nodes = ref([]), nodesLoading = ref(false), saving = ref(false), error = ref(''), bindingRows = ref([]), bindingsLoading = ref(false)
let generation = 0, nodeGeneration = 0
const resourceOptions = [{ value: '1/1Gi', label: '1 核 / 1 GiB' }, { value: '2/2Gi', label: '2 核 / 2 GiB' }, { value: '2/4Gi', label: '2 核 / 4 GiB' }]
const sourceOptions = computed(() => [{ value: '', label: '选择源数据库' }, ...props.mysqlClusters.map(item => ({ value: `cluster:${item.id}`, label: `${item.name}（Operator 集群）` })), ...props.instances.map(item => ({ value: `instance:${item.id}`, label: item.name }))])
const targetOptions = computed(() => [{ value: '', label: '选择目标数据库' }, ...props.instances.filter(item => {
  if (`instance:${item.id}` === form.source) return false
  if (['replicate', 'detach', 'replication_check'].includes(props.kind) && item.mode !== 'managed') return false
  if (['detach', 'replication_check'].includes(props.kind) && !props.relations.some(relation => relation.target === `instance:${item.id}`)) return false
  return true
}).map(item => ({ value: `instance:${item.id}`, label: item.name }))])
const clusterOptions = computed(() => [{ value: 0, label: '选择 Kubernetes 集群' }, ...props.clusters.map(item => ({ value: item.id, label: item.name }))])
const nodeOptions = computed(() => [{ value: '', label: nodesLoading.value ? '读取节点中...' : '选择节点' }, ...nodes.value.map(item => ({ value: item.name, label: `${item.name}${item.database_allowed ? '' : ` · ${item.blocked_reason}`}`, disabled: !item.database_allowed }))])
function close() { if (saving.value) return; wipe(); emit('close') }
function wipe() { form.replication_password = ''; bindingRows.value.forEach(item => { item.password = '' }) }
watch(() => props.kind, async kind => {
  const current = ++generation; ++nodeGeneration; wipe(); Object.assign(form, initial(), { source: props.source, target: props.target }); nodes.value = []; nodesLoading.value = false; bindingRows.value = []; error.value = ''
  if (kind !== 'switch') return
  bindingsLoading.value = true
  const results = await Promise.allSettled(props.instances.map(async instance => (await getDatabaseBindings(instance.id)).map(binding => ({ ...binding, instance_name: instance.name, database: binding.database_name || instance.database_name || '', username: binding.username || instance.username || '', password: '', selected: false }))))
  if (current !== generation) return
  bindingRows.value = results.filter(item => item.status === 'fulfilled').flatMap(item => item.value)
  const failure = results.find(item => item.status === 'rejected'); if (failure) error.value = failure.reason?.message || '读取绑定失败'
  bindingsLoading.value = false
}, { immediate: true })
watch(() => form.cluster_id, async id => {
  const current = ++nodeGeneration; form.node_name = ''; nodes.value = []; nodesLoading.value = false
  if (!props.kind || !id) return
  nodesLoading.value = true
  try { const result = await getDatabaseExecutionNodes(id); if (current === nodeGeneration) nodes.value = result }
  catch (cause) { if (current === nodeGeneration) error.value = cause.message || '读取节点失败' }
  finally { if (current === nodeGeneration) nodesLoading.value = false }
})
async function submit() {
  if (saving.value) return
  if (!form.cluster_id || !form.node_name) { error.value = '请选择执行集群和节点'; return }
  if (props.kind === 'deploy' && !form.name) { error.value = '请填写实例名称'; return }
  if (['inspect', 'migrate', 'replicate'].includes(props.kind) && !form.source) { error.value = '请选择源数据库'; return }
  if (!['deploy', 'inspect'].includes(props.kind) && !form.target) { error.value = '请选择目标数据库'; return }
  if (props.kind === 'migrate' && !form.schemas.trim()) { error.value = '请填写需要迁移的业务数据库名，多个名称用逗号分隔'; return }
  if (props.kind === 'migrate' && !form.writes_stopped) { error.value = '请先停止所选源库的业务写入，再确认保持停写至切换完成'; return }
  saving.value = true
  try {
    let result
    if (props.kind === 'deploy') {
      const [cpu_limit, memory_limit] = form.resources.split('/')
      result = await createDatabase({ name: form.name, mode: 'managed', cluster_id: Number(form.cluster_id), node_name: form.node_name, cpu_limit, memory_limit, storage_size: form.storage_size, storage_class: form.storage_class, image: form.mysql_image })
    } else {
      const payload = { kind: props.kind, source: ['inspect', 'migrate', 'replicate'].includes(props.kind) ? form.source : '', target: form.target, cluster_id: Number(form.cluster_id), node_name: form.node_name, image: form.image }
      if (['migrate', 'replicate'].includes(props.kind)) Object.assign(payload, { storage_size: form.storage_size, storage_class: form.storage_class })
      if (props.kind === 'migrate') Object.assign(payload, { schemas: form.schemas.split(/[,，\s]+/).filter(Boolean), writes_stopped: form.writes_stopped })
      if (props.kind === 'replicate') Object.assign(payload, { replication_user: form.replication_user, replication_password: form.replication_password })
      if (props.kind === 'switch') payload.connections = bindingRows.value.filter(item => item.selected).map(item => ({ binding_id: item.id, database: item.database, username: item.username, password: item.password }))
      result = await createDatabaseOperation(payload)
    }
    wipe(); emit('created', props.kind === 'deploy' ? null : result.id)
  } catch (cause) { error.value = cause.message || '创建任务失败' }
  finally { saving.value = false }
}
</script>

<style scoped>
.capability-form { display: grid; gap: var(--space-16); }
.form-group { margin: 0; min-width: 0; }
.form-row { display: flex; gap: var(--space-12); }
.form-row > * { flex: 1; min-width: 0; }
.check-line { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font-size: 12px; }
.switch-row { padding: 12px 0; border-bottom: 1px solid var(--border-muted); display: grid; gap: 12px; }
.execution-options { color: var(--text-secondary); font-size: 12px; }
.execution-options summary { cursor: pointer; margin-bottom: 12px; }
@media (max-width: 600px) { .form-row { flex-direction: column; } }
</style>
