<template>
  <div class="demo-shell" data-palette="sky">
    <main>
      <SectionTabsHeader title="数据库" :sticky="false" :tabs="pageTabs" :active-tab="page" @select="page = $event" />
      <SurfaceCard padding="none" class="database-surface">
        <div class="toolbar">
          <span class="cell-secondary">共 {{ visibleResources.length }} 个{{ page === 'clusters' ? '集群' : '实例' }}</span>
          <div class="btn-group">
            <button class="btn" @click="dialog = 'tasks'">数据库任务<span v-if="activeCount" class="task-count">{{ activeCount }}</span></button>
            <button v-if="page === 'instances'" class="btn" @click="dialog = 'register'">接入已有实例</button>
            <button class="btn btn-primary" @click="openCreate('deploy')"><Plus :size="16" />部署 MySQL</button>
          </div>
        </div>
        <div class="table-wrap table-inset">
          <table v-if="page === 'clusters'" class="data-table cluster-table">
            <thead><tr><th>集群名称</th><th>Kubernetes 集群</th><th>数据库实例</th><th>Router 实例</th><th>状态</th><th class="align-right">操作</th></tr></thead>
            <tbody><tr v-for="item in visibleResources" :key="item.id"><td class="cell-primary">{{ item.name }}</td><td>{{ item.location }}</td><td>{{ item.databaseCount }}</td><td>{{ item.routerCount }}</td><td><span class="badge badge-online">示例：运行中</span></td><td class="align-right"><button class="btn btn-sm" @click="openDetails(item.id)">查看与配置</button></td></tr></tbody>
          </table>
          <table v-else class="data-table instance-table">
            <thead><tr><th>实例名称</th><th>管理方式</th><th>复制角色</th><th>状态</th><th class="align-right">操作</th></tr></thead>
            <tbody><tr v-for="item in visibleResources" :key="item.id"><td class="cell-primary">{{ item.name }}</td><td>{{ item.type }}</td><td>{{ roleLabel(item.id) }}</td><td><span class="badge" :class="item.registered ? 'badge-offline' : 'badge-online'">{{ item.registered ? '已登记 · 未检测' : '示例：运行中' }}</span></td><td class="align-right"><button class="btn btn-sm" @click="openDetails(item.id)">查看与配置</button></td></tr></tbody>
          </table>
        </div>
      </SurfaceCard>
      <footer class="demo-footer"><span>本地 Demo · 所有操作均为模拟</span><button class="btn btn-sm" @click="reset">重置演示</button></footer>
    </main>

    <BaseModal :open="dialog === 'details'" :title="selectedResource?.name" size="large" teleport @close="dialog = ''">
      <nav class="table-tabs modal-tabs" aria-label="实例详情"><button v-for="tab in detailTabs" :key="tab.id" class="tab-btn" :class="{ 'tab-active': detailTab === tab.id }" @click="detailTab = tab.id">{{ tab.label }}</button></nav>
      <template v-if="detailTab === 'overview'">
        <dl class="facts"><div><dt>管理方式</dt><dd>{{ selectedResource?.type }}</dd></div><div><dt>部署位置</dt><dd>{{ selectedResource?.location }}</dd></div><div><dt>复制角色</dt><dd>{{ roleLabel(selected) }}</dd></div><div><dt>版本</dt><dd>{{ selectedResource?.version || '未检测' }}</dd></div></dl>
        <div class="section-heading"><h3>独立操作</h3></div>
        <div class="capability-list">
          <div><div><strong>数据迁移</strong><p>将所选数据库复制到另一个已有实例。</p></div><button class="btn btn-sm" @click="openCreate('migrate', selected)">迁移数据</button></div>
          <div><div><strong>主从复制</strong><p>选择一个独立 MySQL 实例，建立持续复制关系。</p></div><button class="btn btn-sm" @click="openCreate('replicate', selected)">建立复制</button></div>
          <div><div><strong>应用连接</strong><p>选择应用、预览当前连接，再切换数据库入口。</p></div><button class="btn btn-sm" @click="openCreate('switch', selected)">切换连接</button></div>
        </div>
      </template>
      <template v-else-if="detailTab === 'network'">
        <dl class="facts"><div><dt>实例地址</dt><dd class="mono">{{ selectedResource?.host }}:3306</dd></div><template v-if="selectedResource?.gateway"><div><dt>私网网关</dt><dd class="mono">{{ selectedResource.gateway }}</dd></div><div><dt>Service 入口</dt><dd class="mono">{{ selectedResource.service }}:3306</dd></div></template></dl>
        <div class="section-heading"><h3>连接检测</h3><button class="btn btn-sm" @click="diagnose">模拟检测</button></div>
        <p class="form-hint">选定执行节点到数据库的检测。全节点巡检作为独立排障功能。</p>
        <pre v-if="diagnostics[selected]" class="logs">【本地模拟】执行节点 → 数据库入口：连接成功。未执行真实网络检查。</pre>
      </template>
      <template v-else-if="detailTab === 'replication'">
        <EmptyState v-if="!selectedRelations.length" message="尚未建立复制关系" />
        <div v-for="relation in selectedRelations" :key="relation.id" class="relation-row"><div><strong>{{ resourceName(relation.source) }} → {{ resourceName(relation.target) }}</strong><p class="cell-secondary">{{ relation.active ? '示例：复制运行中 · 延迟 0 秒' : '复制已解除 · 两端实例保留' }}</p></div><button v-if="relation.active" class="btn btn-sm" @click="openDetach(relation)">解除复制</button></div>
        <div class="section-heading"><button class="btn" @click="openCreate('deploy')">部署独立 MySQL</button><button class="btn btn-primary" @click="openCreate('replicate', selected)">建立复制</button></div>
        <p class="form-hint">先部署或接入实例，再建立复制。这里仅展示模拟任务执行完成的关系。</p>
      </template>
      <template v-else>
        <EmptyState v-if="!relatedTasks.length" message="暂无任务记录" />
        <div v-for="task in relatedTasks" :key="task.id" class="relation-row"><div><strong>{{ task.name }}</strong><p class="cell-secondary">{{ taskLabel(task.kind) }} · {{ stateLabel(task) }}</p></div><button class="btn btn-sm" @click="openTask(task.id)">查看任务</button></div>
      </template>
      <template #actions><button class="btn" @click="dialog = ''">关闭</button></template>
    </BaseModal>

    <BaseModal :open="dialog === 'register'" title="接入已有 MySQL · 演示" teleport @close="dialog = ''">
      <form class="create-form" @submit.prevent="registerInstance">
        <label><span class="form-label">实例名称</span><input v-model.trim="registration.name" class="form-input" placeholder="输入实例名称" /></label>
        <label><span class="form-label">管理方式</span><SelectMenu v-model="registration.type" :options="externalTypes" aria-label="管理方式" /></label>
        <label><span class="form-label">主机名 / IP</span><input v-model.trim="registration.host" class="form-input" placeholder="mysql.example.internal" /></label>
        <p class="form-hint">演示仅登记连接信息。正式接入需引用凭据并检测版本、网络和权限。</p>
      </form>
      <template #actions><button class="btn" @click="dialog = ''">取消</button><button class="btn btn-primary" :disabled="!registration.name || !registration.host" @click="registerInstance">登记实例</button></template>
    </BaseModal>

    <BaseModal :open="dialog === 'tasks'" title="数据库任务" size="large" teleport @close="dialog = ''">
      <div class="task-toolbar"><span class="cell-secondary">共 {{ tasks.length }} 个任务</span><button class="btn btn-primary" @click="openCreate('migrate')"><Plus :size="15" />新建任务</button></div>
      <EmptyState v-if="!tasks.length" message="暂无任务，选择能力后单独创建" />
      <div v-else class="table-wrap"><table class="data-table task-table"><thead><tr><th>任务</th><th>能力</th><th>进度</th><th>状态</th><th class="align-right">操作</th></tr></thead><tbody><tr v-for="task in tasks" :key="task.id"><td><strong>{{ task.name }}</strong><span class="cell-secondary subline">{{ taskSummary(task) }}</span></td><td>{{ taskLabel(task.kind) }}</td><td>{{ taskPercent(task) }}%</td><td><span class="badge" :class="stateClass(task)">{{ stateLabel(task) }}</span></td><td class="align-right"><button class="btn btn-sm" @click="openTask(task.id)">查看任务</button></td></tr></tbody></table></div>
      <template #actions><button class="btn" @click="dialog = ''">关闭</button></template>
    </BaseModal>

    <BaseModal :open="dialog === 'create'" :title="'新建任务 · ' + taskLabel(draft.kind)" teleport @close="dialog = 'tasks'">
      <form class="create-form" @submit.prevent="createTask">
        <label><span class="form-label">能力</span><SelectMenu v-model="draft.kind" :options="capabilities" aria-label="任务能力" @change="changeKind" /></label>
        <label><span class="form-label">任务名称</span><input v-model.trim="draft.name" class="form-input" placeholder="可选，自动生成名称" maxlength="80" /></label>
        <template v-if="draft.kind === 'deploy'">
          <label><span class="form-label">实例名称</span><input v-model.trim="draft.instanceName" class="form-input" placeholder="例如：备用 MySQL" maxlength="80" /></label>
          <label><span class="form-label">部署方式</span><SelectMenu v-model="draft.mode" :options="deploymentModes" aria-label="部署方式" /></label>
          <label><span class="form-label">{{ draft.mode === 'server' ? '目标服务器' : 'Kubernetes 集群' }}</span><SelectMenu v-model="draft.location" :options="draft.mode === 'server' ? serverOptions : clusterOptions" aria-label="部署位置" /></label>
          <label><span class="form-label">MySQL 版本</span><SelectMenu v-model="draft.version" :options="['8.0', '8.4']" aria-label="MySQL 版本" /></label>
          <div class="form-columns"><label><span class="form-label">资源规格</span><SelectMenu v-model="draft.size" :options="['1C1G', '2C2G', '2C4G']" aria-label="资源规格" /></label><label><span class="form-label">数据卷</span><SelectMenu v-model="draft.storage" :options="['20 GiB', '50 GiB', '100 GiB']" aria-label="数据卷" /></label></div>
        </template>
        <template v-else-if="draft.kind === 'detach'">
          <label><span class="form-label">复制关系</span><SelectMenu v-model="draft.relation" :options="relationOptions" aria-label="复制关系" /></label>
          <p v-if="!relationOptions.length" class="form-hint">当前没有可解除的复制关系。</p>
        </template>
        <template v-else-if="draft.kind === 'switch'">
          <label><span class="form-label">目标数据库</span><SelectMenu v-model="draft.target" :options="resourceOptions" aria-label="目标数据库" /></label>
          <div class="application-options"><span class="form-label">选择应用</span><label v-for="app in applications" :key="app.id" class="checkbox-label"><input v-model="draft.apps" type="checkbox" :value="app.id" /><span>{{ app.name }}<small class="cell-secondary subline">当前：{{ resourceName(app.database) }}</small></span></label></div>
          <label class="checkbox-label"><input v-model="draft.reviewed" type="checkbox" />已确认目标数据准备完成，并安排写入暂停与最终追平</label>
        </template>
        <template v-else>
          <label><span class="form-label">{{ draft.kind === 'replicate' ? '主库实例' : '源实例' }}</span><SelectMenu v-model="draft.source" :options="resourceOptions" aria-label="源实例" @change="sourceChanged" /></label>
          <label><span class="form-label">{{ draft.kind === 'replicate' ? '备库实例' : '目标实例' }}</span><SelectMenu v-model="draft.target" :options="targetOptions" aria-label="目标实例" /></label>
          <template v-if="draft.kind === 'migrate'">
            <label><span class="form-label">迁移范围</span><SelectMenu v-model="draft.scopeMode" :options="[{ value: 'all', label: '全部业务数据库' }, { value: 'selected', label: '选择数据库' }]" aria-label="迁移范围" /></label>
            <div v-if="draft.scopeMode === 'selected'" class="scope-options"><label v-for="item in availableScope" :key="item" class="checkbox-label"><input v-model="draft.databases" type="checkbox" :value="item" />{{ item }}</label><p v-if="!availableScope.length" class="form-hint">此示例实例暂无数据库。</p></div>
          </template>
          <template v-else>
            <p class="form-hint">选择独立 MySQL 作为备库。云数据库与 Operator 集群的成员不作为此入口的手工复制目标。</p>
            <button class="btn btn-sm" type="button" @click="openCreate('deploy')">先部署一个独立 MySQL</button>
            <label class="checkbox-label"><input v-model="draft.reviewed" type="checkbox" />允许初始化目标备库的数据，正式执行前需检查冲突与复制权限</label>
          </template>
        </template>
        <p class="form-hint capability-hint">{{ capabilityHint }}</p>
      </form>
      <template #actions><button class="btn" @click="dialog = 'tasks'">取消</button><button class="btn btn-primary" :disabled="!canCreate" @click="createTask">创建模拟任务</button></template>
    </BaseModal>

    <BaseModal :open="dialog === 'task'" :title="currentTask?.name" size="large" teleport @close="dialog = 'tasks'">
      <template v-if="currentTask">
        <div class="task-summary"><div><strong>{{ taskLabel(currentTask.kind) }}</strong><span class="cell-secondary subline">{{ taskSummary(currentTask) }}</span></div><span class="badge" :class="stateClass(currentTask)">{{ stateLabel(currentTask) }}</span></div>
        <p v-if="startConflict" class="form-hint conflict-hint">关联实例或应用有未完成的任务，请先处理该任务。</p>
        <nav class="table-tabs modal-tabs" aria-label="任务详情"><button v-for="tab in taskTabs" :key="tab.id" class="tab-btn" :class="{ 'tab-active': taskTab === tab.id }" @click="taskTab = tab.id">{{ tab.label }}</button></nav>
        <ol v-if="taskTab === 'steps'" class="steps"><li v-for="(step, index) in stepsFor(currentTask)" :key="step.title" :class="{ complete: currentTask.stage > index || currentTask.state === 'done', current: currentTask.stage === index && currentTask.state !== 'done' }"><span class="step-number"><Check v-if="currentTask.stage > index || currentTask.state === 'done'" :size="15" /><template v-else>{{ index + 1 }}</template></span><div><strong>{{ step.title }}</strong><p>{{ step.description }}</p><progress v-if="currentTask.stage === index && currentTask.state !== 'done'" :value="currentTask.stepProgress" max="100" :aria-label="step.title + '模拟进度'" /></div></li></ol>
        <template v-else><pre v-if="currentTask.logs.length" class="logs" aria-live="polite">{{ currentTask.logs.join('\n') }}</pre><EmptyState v-else message="启动模拟后展示执行日志" /></template>
      </template>
      <template #actions><button class="btn" @click="dialog = 'tasks'">返回任务列表</button><button v-if="currentTask?.state === 'running'" class="btn" @click="pause"><Pause :size="14" />暂停</button><button v-if="['idle', 'paused'].includes(currentTask?.state)" class="btn btn-primary" :disabled="startConflict" @click="start"><Play :size="14" />{{ currentTask?.state === 'paused' ? '继续模拟' : '开始模拟' }}</button><button v-if="currentTask?.state === 'done'" class="btn btn-primary" @click="openResult">查看结果</button></template>
    </BaseModal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { Check, Pause, Play, Plus } from 'lucide-vue-next'
import BaseModal from '../../src/components/BaseModal.vue'
import EmptyState from '../../src/components/EmptyState.vue'
import SectionTabsHeader from '../../src/components/SectionTabsHeader.vue'
import SelectMenu from '../../src/components/SelectMenu.vue'
import SurfaceCard from '../../src/components/SurfaceCard.vue'

// Local prototype only. No production APIs or credentials are used.
const pageTabs = [{ id: 'clusters', label: 'MySQL 集群' }, { id: 'instances', label: '已有实例' }]
const detailTabs = [{ id: 'overview', label: '基本信息' }, { id: 'network', label: '连接配置' }, { id: 'replication', label: '复制关系' }, { id: 'history', label: '任务记录' }]
const taskTabs = [{ id: 'steps', label: '执行步骤' }, { id: 'logs', label: '任务日志' }]
const capabilities = [{ value: 'deploy', label: '部署 MySQL' }, { value: 'migrate', label: '数据迁移' }, { value: 'replicate', label: '建立复制' }, { value: 'detach', label: '解除复制' }, { value: 'switch', label: '切换应用连接' }]
const deploymentModes = [{ value: 'standalone', label: '独立 MySQL · Kubernetes StatefulSet' }, { value: 'server', label: '独立 MySQL · 服务器容器' }, { value: 'operator', label: 'Operator 管理的 InnoDBCluster' }]
const externalTypes = ['云数据库 RDS', '自建 MySQL', '其他托管 MySQL']
const serverOptions = ['ubuntudev', '示例服务器']
const clusterOptions = ['自用', '测试集群']
const seedResources = () => [
  { id: 'old', managed: true, name: '自用 MySQL 集群', location: '自用', databaseCount: 2, routerCount: 1, type: 'Oracle Operator', host: 'mysql-source.example.internal', version: '8.0', inventory: ['cylism_account', 'cylism_bill'] },
  { id: 'rds', name: '阿里云 RDS · 广州', location: '阿里云', type: '云数据库 RDS', host: 'rm-7xv240dr20sfm55o4.mysql.cn-guangzhou.rds.aliyuncs.com', gateway: '100.106.227.9:13306', service: 'cylism-db-gateway-1.cylism-databases.svc.cluster.local', version: '8.0（示例）', inventory: [] },
  { id: 'test', name: '测试 MySQL', location: '示例服务器', type: '自建 MySQL', host: 'mysql-test.example.internal', version: '8.0', inventory: ['demo_app'] },
]
const seedApps = () => [{ id: 'account', name: 'cylism-account', database: 'old' }, { id: 'bill', name: 'cylism-bill', database: 'old' }]
const resources = ref(seedResources()), applications = ref(seedApps()), tasks = ref([]), relations = ref([])
const page = ref('clusters'), dialog = ref(''), selected = ref('rds'), detailTab = ref('overview'), taskTab = ref('steps'), activeTaskID = ref(null)
const diagnostics = reactive({})
const registration = reactive({ name: '', type: '自建 MySQL', host: '' })
const emptyDraft = kind => ({ kind, name: '', instanceName: '', mode: 'standalone', location: '', version: '8.0', size: '1C1G', storage: '20 GiB', source: '', target: '', relation: '', scopeMode: 'all', databases: [], apps: [], reviewed: false })
const draft = reactive(emptyDraft('migrate'))
const selectedResource = computed(() => resources.value.find(item => item.id === selected.value))
const visibleResources = computed(() => resources.value.filter(item => Boolean(item.managed) === (page.value === 'clusters')))
const selectedRelations = computed(() => relations.value.filter(item => [item.source, item.target].includes(selected.value)))
const relatedTasks = computed(() => tasks.value.filter(task => taskResources(task).includes(selected.value)))
const currentTask = computed(() => tasks.value.find(task => task.id === activeTaskID.value))
const resourceOptions = computed(() => resources.value.map(item => ({ value: item.id, label: item.name })))
const targetOptions = computed(() => resources.value.filter(item => item.id !== draft.source).map(item => ({ value: item.id, label: item.name, disabled: draft.kind === 'replicate' && (item.managed || ['云数据库 RDS', '其他托管 MySQL'].includes(item.type) || relations.value.some(r => r.active && r.target === item.id)) })))
const relationOptions = computed(() => relations.value.filter(item => item.active).map(item => ({ value: item.id, label: resourceName(item.source) + ' → ' + resourceName(item.target) })))
const availableScope = computed(() => resources.value.find(item => item.id === draft.source)?.inventory || [])
const activeCount = computed(() => tasks.value.filter(task => ['running', 'paused'].includes(task.state)).length)
const canCreate = computed(() => {
  if (draft.kind === 'deploy') return Boolean(draft.instanceName && draft.location && (draft.mode === 'server' ? serverOptions : clusterOptions).includes(draft.location))
  if (draft.kind === 'detach') return relationOptions.value.some(item => item.value === draft.relation)
  if (draft.kind === 'switch') return Boolean(draft.target && draft.apps.length && draft.reviewed)
  const validTarget = targetOptions.value.some(item => item.value === draft.target && !item.disabled)
  if (!draft.source || !validTarget) return false
  if (draft.kind === 'replicate') return draft.reviewed && !wouldCycle(draft.source, draft.target)
  return availableScope.value.length > 0 && (draft.scopeMode === 'all' || draft.databases.length > 0)
})
const capabilityHint = computed(() => ({ deploy: '只部署数据库实例。创建后可单独迁移数据或建立复制关系。', migrate: '迁移所选业务数据与兼容的账号权限。应用连接保持原状，备库由其他任务创建。', replicate: '仅检查选定主库、备库及执行节点；初始化后建立持续复制。RDS 的权限和 binlog 条件需实际检测。', detach: '停止指定复制关系，保留两端实例和数据。', switch: '仅更新选定应用的数据库配置、重新发布并验证读写。数据迁移需要提前完成。' }[draft.kind]))
const resourceName = id => resources.value.find(item => item.id === id)?.name || '未选择实例'
const taskLabel = kind => capabilities.find(item => item.value === kind)?.label || ''
function roleLabel(id) { const incoming = relations.value.some(r => r.active && r.target === id); const outgoing = relations.value.some(r => r.active && r.source === id); return incoming && outgoing ? '级联复制' : incoming ? '备库' : outgoing ? '复制源' : '未关联复制' }
function wouldCycle(source, target) { const visited = new Set(); const queue = [target]; while (queue.length) { const id = queue.pop(); if (id === source) return true; if (visited.has(id)) continue; visited.add(id); queue.push(...relations.value.filter(r => r.active && r.source === id).map(r => r.target)) } return false }
function stepsFor(task) {
  const descriptions = {
    deploy: [['检查部署条件', '检查版本、资源、目标节点和存储。'], ['部署数据库', task.mode === 'operator' ? '创建独立 InnoDBCluster，由 Operator 管理生命周期。' : '创建独立 MySQL、配置和数据卷，不加入 Operator 集群。'], ['验证实例', '检查启动和连接，登记数据库实例。']],
    migrate: [['检查迁移条件', '检查源和目标的版本、选定数据库、账号权限及同名数据冲突。'], ['复制业务数据', '演示全量复制与一致性校验，账号权限按兼容性迁移。'], ['生成迁移报告', '记录数据结果与后续增量或维护窗口要求，应用连接保持原状。']],
    replicate: [['检查复制条件', '检测双方网络、版本、binlog、GTID 和复制账号；确认目标数据可初始化。'], ['初始化备库', '从所选主库准备一致性数据并恢复到已有独立实例。'], ['建立持续复制', '配置复制通道，检查线程状态与延迟后记录关系。']],
    detach: [['检查复制关系', '检查所选通道与当前执行任务。'], ['停止并解除复制', '停止指定通道并清除其复制配置，保留数据。'], ['验证独立运行', '确认两端实例保留，更新复制记录。']],
    switch: [['核对切换清单', '核对所选应用、目标数据库、账号和权限，确认最终数据追平。'], ['更新并发布应用', '模拟暂停写入、修改连接、重新发布。'], ['验证应用读写', '从应用实际运行节点检查读写并记录新连接。']],
  }
  return descriptions[task.kind].map(([title, description]) => ({ title, description }))
}
function taskResources(task) { return [task.source, task.target, task.resultID, ...(task.previousTargets || [])].filter(Boolean) }
function taskSummary(task) { if (task.kind === 'deploy') return task.instanceName + ' · ' + task.location; if (task.kind === 'switch') return task.apps.length + ' 个应用 → ' + resourceName(task.target); return resourceName(task.source) + ' → ' + resourceName(task.target) }
const stateLabel = task => ({ idle: '待执行', running: '模拟执行中', paused: '已暂停', done: '模拟已完成', blocked: '条件已变化' }[task.state])
const stateClass = task => task.state === 'done' ? 'badge-online' : ['running', 'paused'].includes(task.state) ? 'badge-warn' : 'badge-offline'
const taskPercent = task => task.state === 'done' ? 100 : Math.floor((task.stage * 100 + task.stepProgress) / stepsFor(task).length)
const startConflict = computed(() => currentTask.value && tasks.value.some(other => other.id !== currentTask.value.id && ['running', 'paused'].includes(other.state) && (taskResources(other).some(id => taskResources(currentTask.value).includes(id)) || (other.apps || []).some(id => currentTask.value.apps.includes(id)))))
let sequence = 0, timer
function openDetails(id) { selected.value = id; detailTab.value = 'overview'; dialog.value = 'details' }
function openCreate(kind, source = '') { Object.assign(draft, emptyDraft(kind)); if (['migrate', 'replicate'].includes(kind)) draft.source = source; if (kind === 'switch') draft.target = source; dialog.value = 'create' }
function changeKind(kind) { Object.assign(draft, emptyDraft(kind)) }
function sourceChanged() { draft.databases = []; draft.reviewed = false; if (!targetOptions.value.some(item => item.value === draft.target && !item.disabled)) draft.target = '' }
function openDetach(relation) { openCreate('detach'); draft.relation = relation.id }
function openTask(id) { activeTaskID.value = id; taskTab.value = 'steps'; dialog.value = 'task' }
function registerInstance() { if (!registration.name || !registration.host) return; const id = 'registered-' + ++sequence; resources.value.push({ ...registration, id, location: '待检测', registered: true, inventory: [] }); Object.assign(registration, { name: '', type: '自建 MySQL', host: '' }); openDetails(id) }
function createTask() {
  if (!canCreate.value) return
  const task = { ...draft, id: ++sequence, apps: [...draft.apps], scope: availableScope.value.filter(item => draft.scopeMode === 'all' || draft.databases.includes(item)), state: 'idle', stage: 0, stepProgress: 0, logs: [] }
  if (task.kind === 'detach') Object.assign(task, relations.value.find(item => item.id === draft.relation), { id: sequence, relation: draft.relation })
  if (task.kind === 'switch') task.previousTargets = applications.value.filter(app => task.apps.includes(app.id)).map(app => app.database)
  task.name ||= taskLabel(task.kind) + ' · ' + (task.kind === 'deploy' ? task.instanceName : resourceName(task.target))
  tasks.value.push(task); openTask(task.id)
}
function diagnose() { diagnostics[selected.value] = true }
function addLog(task, message) { task.logs.push(new Date().toLocaleTimeString('zh-CN', { hour12: false }) + ' 【本地模拟】' + message) }
function validAtStart(task) {
  if (task.kind === 'replicate') return !relations.value.some(r => r.active && r.target === task.target) && !wouldCycle(task.source, task.target)
  if (task.kind === 'detach') return relations.value.some(r => r.id === task.relation && r.active)
  if (task.kind === 'switch') return applications.value.filter(app => task.apps.includes(app.id)).every(app => task.previousTargets.includes(app.database))
  return true
}
function start() {
  const task = currentTask.value
  if (!task || !['idle', 'paused'].includes(task.state) || startConflict.value) return
  if (!validAtStart(task)) { task.state = 'blocked'; addLog(task, '资源状态已变化，请基于当前状态重新创建任务。'); return }
  addLog(task, task.state === 'paused' ? '继续执行' : '开始：' + stepsFor(task)[task.stage].title); task.state = 'running'
  if (!timer) timer = setInterval(() => { tasks.value.filter(item => item.state === 'running').forEach(tick); if (!tasks.value.some(item => item.state === 'running')) stopTimer() }, 300)
}
function pause() { currentTask.value.state = 'paused'; addLog(currentTask.value, '已在模拟检查点暂停') }
function tick(task) { task.stepProgress = Math.min(100, task.stepProgress + 10); if (task.stepProgress < 100) return; addLog(task, stepsFor(task)[task.stage].title + '：完成'); if (task.stage === stepsFor(task).length - 1) { finish(task); return } task.stage++; task.stepProgress = 0; addLog(task, '开始：' + stepsFor(task)[task.stage].title) }
function finish(task) {
  if (task.kind === 'deploy') { task.resultID = 'deployed-' + task.id; resources.value.push({ id: task.resultID, name: task.instanceName, managed: task.mode === 'operator', location: task.location, type: task.mode === 'operator' ? 'Oracle Operator' : task.mode === 'server' ? '自建 MySQL' : 'Kubernetes StatefulSet', databaseCount: 1, routerCount: task.mode === 'operator' ? 1 : 0, version: task.version, size: task.size, storage: task.storage, host: 'mysql-' + task.id + '.example.internal', inventory: [] }) }
  if (task.kind === 'migrate') { const target = resources.value.find(item => item.id === task.target); target.inventory = [...new Set([...target.inventory, ...task.scope])]; addLog(task, '仅模拟迁移数据；应用连接与复制关系保持原状。') }
  if (task.kind === 'replicate') { const source = resources.value.find(item => item.id === task.source); resources.value.find(item => item.id === task.target).inventory = [...source.inventory]; relations.value.push({ id: 'replication-' + task.id, source: task.source, target: task.target, active: true }) }
  if (task.kind === 'detach') relations.value.find(item => item.id === task.relation).active = false
  if (task.kind === 'switch') applications.value.filter(app => task.apps.includes(app.id)).forEach(app => { app.database = task.target })
  task.state = 'done'; addLog(task, '模拟任务完成；未操作真实数据库或应用。')
}
function openResult() { const task = currentTask.value; openDetails(task.resultID || task.target); detailTab.value = ['replicate', 'detach'].includes(task.kind) ? 'replication' : 'overview' }
function stopTimer() { clearInterval(timer); timer = undefined }
function reset() { stopTimer(); resources.value = seedResources(); applications.value = seedApps(); tasks.value = []; relations.value = []; sequence = 0; Object.keys(diagnostics).forEach(key => delete diagnostics[key]); Object.assign(registration, { name: '', type: '自建 MySQL', host: '' }); dialog.value = ''; page.value = 'clusters'; activeTaskID.value = null }
onBeforeUnmount(stopTimer)
</script>

<style scoped>
.demo-shell { position: relative; min-height: 100vh; overflow: clip; background: var(--canvas); }
.demo-shell::before, .demo-shell::after { position: fixed; z-index: 0; content: ''; pointer-events: none; transform: skewY(-17deg); }
.demo-shell::before { top: -17vh; right: -21vw; width: 85vw; height: 58vh; background: var(--band-a); opacity: .92; }
.demo-shell::after { bottom: -16vh; left: -22vw; width: 87vw; height: 54vh; background: var(--band-b); opacity: .9; }
main { position: relative; padding: 12px var(--space-20) var(--space-24); }
main :deep(.section-tabs-header) { position: relative; }
.database-surface { margin-top: var(--tabbed-page-content-gap); }
.toolbar, .task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-8); padding: 14px var(--space-20); min-height: 66px; }
.task-toolbar { padding: 0 0 var(--space-16); }
.table-inset { padding: 0 var(--space-20); }
.cluster-table { min-width: 800px; }.instance-table { min-width: 700px; }.task-table { min-width: 630px; }
.data-table .align-right { text-align: right; white-space: nowrap; }.data-table strong { color: var(--text-primary); font-size: 12px; }
.subline { display: block; margin-top: var(--space-4); line-height: 1.5; }
.demo-footer { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-12); color: var(--text-muted); font-size: 11px; margin-top: var(--space-16); }
.task-count { min-width: 17px; padding: 0 var(--space-4); border-radius: var(--radius-control); background: var(--neutral-surface); font-size: 10px; }
.create-form { display: grid; gap: var(--space-16); }.form-columns { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-12); }.scope-options, .application-options { display: grid; gap: var(--space-12); }
.checkbox-label { align-items: flex-start; line-height: 1.7; }.checkbox-label input { margin-top: 4px; }
.create-form .form-hint { margin-top: 0; }.capability-hint { border-top: 1px solid var(--border-muted); padding-top: var(--space-12); }.conflict-hint { color: var(--warning); margin-bottom: var(--space-16); }
.modal-tabs { margin-bottom: var(--space-20); flex-wrap: wrap; }
.facts { margin: 0; font-size: 12px; }.facts > div { display: grid; grid-template-columns: 112px minmax(0, 1fr); gap: var(--space-16); padding: var(--space-12) 0; border-bottom: 1px solid var(--border-muted); }.facts dt { color: var(--text-secondary); }.facts dd { margin: 0; line-height: 1.7; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); margin: var(--space-20) 0 var(--space-12); }.section-heading h3 { font-size: 13px; margin: 0; }
.capability-list > div, .relation-row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); padding: var(--space-12) 0; border-bottom: 1px solid var(--border-muted); font-size: 12px; }.capability-list strong, .relation-row strong { font-size: 13px; }.capability-list p, .relation-row p { margin: var(--space-4) 0 0; line-height: 1.7; color: var(--text-secondary); }.capability-list .btn, .relation-row .btn { flex-shrink: 0; }
.form-hint { color: var(--text-secondary); font-size: 12px; line-height: 1.8; margin: var(--space-16) 0 0; }
.task-summary { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-16); margin-bottom: var(--space-20); }.task-summary strong { font-size: 14px; }
.steps { list-style: none; padding: 0; margin: 0; }.steps li { display: flex; gap: var(--space-12); padding: var(--space-16) 0; border-bottom: 1px solid var(--border-muted); }.steps li:first-child { padding-top: 0; }.steps li:last-child { border: 0; }.steps li > div { flex: 1; min-width: 0; }.steps strong { font-size: 13px; }.steps p { margin: 6px 0 0; font-size: 12px; line-height: 1.7; color: var(--text-secondary); }.step-number { display: grid; place-items: center; width: 26px; height: 26px; flex-shrink: 0; background: var(--neutral-surface); color: var(--text-secondary); border-radius: 50%; font-size: 12px; }.current .step-number { color: var(--action-contrast); background: var(--action-primary); }.complete .step-number { background: var(--success-surface); color: var(--success); }
progress { display: block; width: 100%; height: 5px; margin: var(--space-12) 0 var(--space-8); accent-color: var(--action-primary); border: 0; border-radius: var(--radius-control); overflow: hidden; background: var(--neutral-surface); }progress::-webkit-progress-bar { background: var(--neutral-surface); }progress::-webkit-progress-value { background: var(--action-primary); }
.logs { min-height: 130px; max-height: 320px; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; padding: var(--space-16); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-primary); font: 12px/1.9 var(--font-mono); }
@media (max-width: 760px) { main { padding: var(--space-16) 14px; }.toolbar { align-items: flex-start; flex-direction: column; padding: 14px; }.toolbar .btn-group { width: 100%; flex-direction: column; }.toolbar .btn-group > .btn { width: 100%; }.table-inset { padding-inline: 14px; }.section-heading { flex-wrap: wrap; }.facts > div { grid-template-columns: 1fr; gap: var(--space-4); }.form-columns { grid-template-columns: 1fr; }.task-summary { flex-wrap: wrap; } }
</style>
