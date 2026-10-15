<template>
  <section class="mysql-detail">
    <router-link class="back-link" to="/cloud-services/databases"><ArrowLeft :size="16" />返回数据库列表</router-link>
    <SectionTabsHeader :title="cluster?.name || 'MySQL 集群'" :tabs="tabs" :active-tab="activeTab" @select="activeTab = $event">
      <template v-if="cluster" #actions><span class="badge" :class="cluster.status === 'ready' ? 'badge-online' : 'badge-offline'">{{ cluster.status === 'paused' && cleanupState?.cluster_resource ? '暂停中' : statusLabel(cluster.status) }}</span></template>
    </SectionTabsHeader>
    <SurfaceCard v-if="loading && !cluster" class="detail-content" padding="none"><EmptyState variant="loading" message="正在读取 MySQL 集群..." /></SurfaceCard>
    <SurfaceCard v-else-if="cluster && activeTab === 'topology'" class="detail-content" padding="none">
      <div class="detail-toolbar">
        <button v-if="showOperatorUninstall" class="btn btn-danger" type="button" @click="uninstallOpen = true">卸载 Operator</button>
        <div class="toolbar-actions">
        <button v-if="cluster.status !== 'draft'" class="icon-button" type="button" title="刷新运行状态" aria-label="刷新运行状态" :disabled="refreshing" @click="refresh()"><RefreshCw :size="16" /></button>
        <button v-if="cluster.status !== 'draft'" class="btn" type="button" @click="connectionDetailsOpen = true">连接信息</button>
        <button v-if="['draft', 'ready'].includes(cluster.status)" class="btn" type="button" @click="openMySQLConfig">MySQL 配置</button>
        <button class="btn" type="button" @click="openResourcePool">{{ resourcePoolEditable ? (cluster.database_nodes?.length ? '编辑资源池' : '添加资源池') : '查看资源池' }}</button>
        <button v-if="operatorState === 'missing' && isK3s" class="btn" type="button" :disabled="installing" @click="installOperator">{{ installing ? '安装中...' : '安装 MySQL Operator' }}</button>
        <button v-if="operatorPending || operatorUninstallPending" class="btn" type="button" :disabled="checkingOperator" @click="refreshOperator">{{ checkingOperator ? '检查中...' : operatorUninstallPending ? '检查卸载状态' : '检查安装状态' }}</button>
        <button v-if="operatorCanCancel" class="btn" type="button" :disabled="cancelling" @click="requestCancel">中止安装</button>
        <button v-if="canPauseDeployment" class="btn" type="button" :disabled="pausing" @click="pauseOpen = true">{{ isScalingTask ? '撤回本次追加' : '暂停初次部署' }}</button>
        <button v-if="cluster.status === 'paused' && hasResidualWorkloads" class="btn" type="button" :disabled="cleaning || cleanupState?.cluster_resource" @click="cleanupClaims = false; cleanupOpen = true">清理残留负载</button>
        <button v-if="cluster.status === 'paused' && cleanupState?.retry_available" class="btn" type="button" :disabled="retryingShutdown" @click="retryShutdownOpen = true">重试 Operator 清理</button>
        <button v-if="cluster.status === 'paused' && cleanupState?.claims?.length && !hasResidualWorkloads && !cleanupState.cluster_resource" class="btn btn-danger" type="button" :disabled="cleaning" @click="cleanupClaims = true; cleanupOpen = true">清理数据卷</button>
        <button v-if="editable" class="btn btn-primary" type="button" :disabled="saving || operatorState !== 'ready' || !cluster.database_nodes?.length" :title="!cluster.database_nodes?.length ? '请先添加资源池' : undefined" @click="openDeployment">部署 MySQL</button>
        <button v-if="canAddDatabase" class="btn btn-primary" type="button" :disabled="Boolean(scaling) || retiringRouter" @click="openDatabaseScaling">添加数据库实例</button>
        <button v-if="canAddRouter" class="btn" type="button" :disabled="Boolean(scaling) || retiringRouter" @click="routerInstanceCount === 0 ? openRouterDeployment() : (scaleConfirmation = 'router')">{{ routerInstanceCount === 0 ? '部署 Router' : '添加 Router' }}</button>
      </div></div>
      <div v-if="cluster.status === 'draft' || cluster.status === 'paused' || (cluster.status === 'error' && !runtime.pods.length)" class="topology-empty"><EmptyState :message="cluster.status === 'paused' ? pausedMessage : cluster.status === 'draft' ? '尚未部署 MySQL 集群' : '暂无运行中的 MySQL Pod'" /></div>
      <div v-else-if="runtime.pods.length" class="mysql-topology" aria-label="MySQL 运行拓扑">
        <section v-if="routerInstanceCount > 0 || routerPods.length" class="topology-stage" aria-label="Router 入口">
          <div class="topology-stage-heading"><span class="stage-icon"><Server :size="18" /></span><div><strong>Router</strong><span class="cell-secondary">{{ runtime.router_ready || 0 }}/{{ routerInstanceCount }} 就绪</span></div><div class="topology-stage-actions"><button v-if="routerPlacementMismatch && cluster.status === 'ready'" class="icon-button icon-button--table" type="button" title="修正 Router 调度" aria-label="修正 Router 调度" :disabled="repairingRouter || retiringRouter" @click="routerRepairOpen = true"><Settings2 :size="14" /></button><button v-if="canRetireRouter" class="icon-button icon-button--table" type="button" title="清退一个 Router 实例" aria-label="清退一个 Router 实例" :disabled="retiringRouter || repairingRouter || Boolean(scaling)" @click="routerRetireOpen = true"><Trash2 :size="14" /></button></div></div>
          <div class="topology-nodes">
            <article v-for="pod in routerPods" :key="pod.name" class="topology-node topology-node-router">
              <div class="topology-node-identity"><span class="node-icon"><Server :size="18" /></span><strong>{{ pod.node_name || '等待调度' }}</strong><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-offline'">{{ pod.ready ? '就绪' : (pod.reason || pod.phase || '启动中') }}</span></div>
              <span class="pod-name mono" :title="pod.name">{{ pod.name }}</span>
              <span class="topology-node-usage cell-secondary">{{ podUsageLabel(pod) }}</span>
            </article>
            <span v-if="!routerPods.length" class="cell-secondary">等待 Router Pod 创建</span>
          </div>
        </section>
        <div v-if="routerInstanceCount > 0 || routerPods.length" class="topology-flow" aria-hidden="true"><ArrowDown :size="18" /></div>
        <section class="topology-stage" aria-label="MySQL 实例">
          <div class="topology-stage-heading"><span class="stage-icon"><Database :size="18" /></span><div><strong>MySQL 实例</strong><span class="cell-secondary">{{ runtime.online_instances || 0 }}/{{ cluster.database_instances || cluster.database_nodes?.length || 0 }} 就绪</span></div></div>
          <div class="topology-nodes">
            <article v-for="pod in databasePods" :key="pod.name" class="topology-node topology-node-database">
              <div class="topology-node-identity"><span class="node-icon"><Database :size="18" /></span><strong>{{ pod.node_name || '等待调度' }}</strong><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-offline'">{{ databaseRoleLabel(pod.database_role) }}</span><button v-if="pod.ready" class="icon-button icon-button--table" type="button" title="打开数据库终端" :aria-label="`打开 ${pod.name} 的数据库终端`" @click="terminalPod = pod"><SquareTerminal :size="14" /></button><button v-if="cluster.status === 'ready' && pod.ready && pod.database_role?.toUpperCase() === 'SECONDARY'" class="icon-button icon-button--table" type="button" title="设为主实例" :disabled="switchingPrimary" :aria-label="`将 ${pod.name} 切换为主实例`" @click="switchTarget = pod"><ArrowUpCircle :size="14" /></button></div>
              <span class="pod-name mono" :title="pod.name">{{ pod.name }}</span>
              <span class="topology-node-usage cell-secondary">{{ podUsageLabel(pod) }}</span>
              <span v-if="!pod.ready" class="cell-secondary">{{ pod.reason || pod.phase || '启动中' }}</span>
              <span v-if="pod.claims?.length" class="cell-secondary mono">PVC {{ pod.claims.join('、') }}</span>
            </article>
            <span v-if="!databasePods.length" class="cell-secondary">等待 MySQL Pod 创建</span>
          </div>
        </section>
      </div>
      <div v-else class="topology-empty"><EmptyState :message="runtime.progress || '正在等待 MySQL Operator 创建 Pod'" /></div>
    </SurfaceCard>
    <SurfaceCard v-else-if="cluster && activeTab === 'resources'" class="detail-content" padding="none">
      <div class="detail-toolbar"><div class="toolbar-actions"><button class="icon-button" type="button" title="刷新部署资源" aria-label="刷新部署资源" :disabled="refreshing" @click="refresh()"><RefreshCw :size="16" /></button><span v-if="cluster.status !== 'draft' && cluster.status !== 'paused'" class="cell-secondary">数据库工作负载：{{ runtime.stateful_set_created ? '已创建' : '等待 Operator 创建' }}</span></div></div>
      <EmptyState v-if="cluster.status === 'draft' || cluster.status === 'paused'" :message="cluster.status === 'paused' ? pausedMessage : '暂无部署资源'" />
      <template v-else>
        <div class="table-wrap resource-table"><table class="data-table"><thead><tr><th>Pod</th><th>角色</th><th>实际节点</th><th>状态</th><th>CPU</th><th>内存</th><th>数据卷</th><th>操作</th></tr></thead><tbody>
          <tr v-for="pod in runtime.pods || []" :key="pod.name">
            <td class="mono resource-name" :title="pod.name">{{ pod.name }}</td>
            <td>{{ pod.component === 'mysqld' ? databaseRoleLabel(pod.database_role) : 'Router' }}</td>
            <td class="resource-node" :title="pod.node_name || '等待调度'">{{ pod.node_name || '等待调度' }}</td>
            <td><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-offline'" :title="pod.last_event || pod.reason || pod.phase || '等待中'">{{ podStatusLabel(pod) }}</span></td>
            <td>{{ formatPodCPU(pod.cpu_millicores) }}</td>
            <td>{{ formatPodMemory(pod.memory_bytes) }}</td>
            <td><div v-if="pod.claims?.length" class="resource-claims"><button v-for="claimName in pod.claims" :key="claimName" class="resource-detail-button resource-claim" type="button" :title="claimName" :aria-label="`查看数据卷 ${claimName}`" @click="resourceDetails = { kind: 'claim', item: claimByName[claimName] || { name: claimName } }"><span class="badge" :class="claimByName[claimName]?.phase === 'Bound' ? 'badge-online' : 'badge-offline'">{{ claimByName[claimName]?.phase || '等待创建' }}</span><span>{{ claimByName[claimName]?.capacity || '查看 PVC' }}</span></button></div><span v-else class="cell-secondary">-</span></td>
            <td><div class="row-actions"><button class="btn btn-sm" type="button" :aria-label="`查看 ${pod.name} 的详情`" @click="resourceDetails = { kind: 'pod', item: pod }">详情</button><button v-if="canPullPodImage(pod)" class="btn btn-sm" type="button" title="在该节点拉取当前阻塞 Pod 启动的镜像" :disabled="Boolean(pullingPod) || retryingPod" @click="pullPodImages(pod)">{{ pullingPod === pod.name ? '拉取中...' : '节点拉取' }}</button><button v-if="pod.retryable" class="btn btn-sm" type="button" :disabled="Boolean(pullingPod) || retryingPod" @click="retryTarget = pod">重试</button></div></td>
          </tr>
          <tr v-for="claim in unattachedClaims" :key="`claim-${claim.name}`"><td class="cell-secondary">Pod 尚未创建</td><td class="cell-secondary">-</td><td class="cell-secondary">-</td><td class="cell-secondary">-</td><td><button class="resource-detail-button resource-claim" type="button" :title="claim.name" :aria-label="`查看数据卷 ${claim.name}`" @click="resourceDetails = { kind: 'claim', item: claim }"><span class="badge" :class="claim.phase === 'Bound' ? 'badge-online' : 'badge-offline'">{{ claim.phase || '等待创建' }}</span><span>{{ claim.capacity || '查看 PVC' }}</span></button></td><td></td></tr>
          <tr v-if="!runtime.pods?.length && !unattachedClaims.length"><td colspan="6" class="cell-secondary">Pod 和数据卷尚未创建</td></tr>
        </tbody></table></div>
      </template>
    </SurfaceCard>
    <SurfaceCard v-else-if="cluster" class="detail-content" padding="none">
      <div class="detail-toolbar"><div class="toolbar-actions"><button class="icon-button" type="button" title="刷新任务" aria-label="刷新任务" :disabled="refreshing" @click="refresh()"><RefreshCw :size="16" /></button></div></div>
      <div v-if="cluster.status === 'paused' && cleanupState?.cluster_resource" class="shutdown-progress cell-secondary"><span>删除请求：{{ formatTime(cleanupState.deletion_started_at) }}</span><span>Finalizer：{{ cleanupState.finalizers?.join('、') || '无' }}</span><span>剩余工作负载 {{ cleanupState.workloads.length }} 个、Pod {{ cleanupState.pods.length }} 个</span></div>
      <div v-if="tasks.length" class="table-wrap task-table"><table class="data-table"><thead><tr><th>任务</th><th>状态</th><th>开始时间</th><th>操作</th></tr></thead><tbody><tr v-for="task in tasks" :key="task.id"><td>{{ taskLabel(task) }} #{{ task.id }}</td><td><span class="badge" :class="task.status === 'success' ? 'badge-online' : task.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ taskStatusLabel(task.status) }}</span></td><td>{{ formatTime(task.created_at) }}</td><td><button class="btn btn-sm" type="button" @click="openLogs(task)">查看日志</button></td></tr></tbody></table></div>
      <EmptyState v-else :variant="tasksLoading ? 'loading' : 'empty'" :message="tasksLoading ? '正在读取集群任务...' : '暂无集群任务'" />
      <div v-if="taskTotal > taskPageSize" class="pagination task-pagination"><button class="btn btn-sm" type="button" :disabled="taskOffset === 0 || tasksLoading" @click="changeTaskPage(-1)">上一页</button><span>{{ taskPage }} / {{ taskPageCount }} · 共 {{ taskTotal }} 条</span><button class="btn btn-sm" type="button" :disabled="taskOffset + taskPageSize >= taskTotal || tasksLoading" @click="changeTaskPage(1)">下一页</button></div>
    </SurfaceCard>
    <PodTerminal v-if="terminalPod" :pod="{ name: terminalPod.name, namespace: cluster.namespace, containers: ['mysql'] }" :mysql-cluster-id="id" @close="terminalPod = null" />
    <BaseModal :open="configOpen" title="MySQL 配置" size="large" teleport :show-close="!configSaving" :close-on-overlay="!configSaving" :close-on-escape="!configSaving" @close="configOpen = false">
      <div class="deploy-form">
        <label class="form-group"><span class="form-label">my.cnf 附加配置</span><textarea v-model="configText" class="form-input mysql-config-editor mono" spellcheck="false" :disabled="configLoading || configSaving" aria-label="MySQL my.cnf 附加配置" placeholder="[mysqld]&#10;innodb_buffer_pool_size=128M&#10;innodb_log_buffer_size=64M"></textarea></label>
        <p class="cell-secondary">支持内存及连接数相关参数。保存后需滚动重启 MySQL 实例才能生效；单实例重启期间连接会中断。</p>
        <p v-if="configRestartRequired" class="cell-secondary">配置已保存，等待滚动重启应用。</p>
      </div>
      <template #actions><button class="btn" type="button" :disabled="configSaving" @click="configOpen = false">关闭</button><button v-if="configRestartRequired && cluster?.status === 'ready'" class="btn" type="button" :disabled="configSaving || configText !== configSaved" @click="openConfigRestartConfirmation">滚动重启</button><button class="btn btn-primary" type="button" :disabled="configSaving || configLoading || configText === configSaved" @click="saveMySQLConfig">{{ configSaving ? '保存中...' : '保存配置' }}</button></template>
    </BaseModal>
    <ConfirmDialog :open="configRestartOpen" title="滚动重启 MySQL" message="将按 StatefulSet 的滚动更新顺序重建 MySQL Pod，以应用已保存的配置。单实例集群会短暂不可用；已有连接可能中断。PVC 和集群资源会保留。" confirm-text="确认重启" busy-text="正在提交..." :busy="configRestarting" @confirm="restartMySQLConfig" @cancel="cancelConfigRestartConfirmation" />
    <BaseModal :open="connectionDetailsOpen" title="连接信息" size="large" teleport @close="connectionDetailsOpen = false">
      <div class="connection-details">
        <section class="connection-section"><h3>Router</h3><template v-if="routerInstanceCount > 0 || routerPods.length"><div class="connection-row"><span>读写入口 · 集群内</span><code>{{ runtime.router_read_write || (runtime.connection_details_checked ? 'Router Service 尚未创建' : '正在读取 Router Service') }}</code></div><div class="connection-row"><span>只读入口 · 集群内</span><code>{{ runtime.router_read_only || (runtime.connection_details_checked ? 'Router Service 尚未创建' : '正在读取 Router Service') }}</code></div></template><span v-else class="cell-secondary">尚未部署 Router</span></section>
        <section class="connection-section"><h3>MySQL 实例</h3><div v-for="pod in databasePods" :key="pod.name" class="connection-row"><span>{{ pod.name }} · {{ databaseRoleLabel(pod.database_role) }}</span><code>{{ pod.address || (runtime.connection_details_checked ? '等待地址分配' : '正在读取实例地址') }}</code></div><span v-if="!databasePods.length" class="cell-secondary">尚无 MySQL 实例</span></section>
      </div>
      <template #actions><button class="btn" type="button" @click="connectionDetailsOpen = false">关闭</button></template>
    </BaseModal>
    <BaseModal :open="resourcePoolOpen" title="数据库资源池" size="large" teleport :show-close="!resourcePoolSaving" :close-on-overlay="!resourcePoolSaving" :close-on-escape="!resourcePoolSaving" @close="closeResourcePool">
      <div class="deploy-form">
        <div class="node-picker-heading"><strong>可调度节点</strong><span class="cell-secondary">已选择 {{ resourcePoolSelection.length }} 台</span></div>
        <EmptyState v-if="resourcePoolLoading" variant="loading" message="正在读取节点..." />
        <div v-else class="table-wrap resource-pool-table"><table class="data-table"><thead><tr><th>选择</th><th>节点 / IP</th><th>调度状态</th></tr></thead><tbody><tr v-for="node in nodes" :key="node.name"><td><input v-model="resourcePoolSelection" type="checkbox" :value="node.name" :disabled="!resourcePoolEditable || !available(node) || (!resourcePoolSelection.includes(node.name) && resourcePoolSelection.length >= 9)" :aria-label="`${node.internal_ip || node.name} 加入资源池`"></td><td class="resource-pool-node"><strong>{{ node.name }}</strong><span class="mono cell-secondary">{{ node.internal_ip || '-' }}</span></td><td><span class="badge" :class="available(node) ? 'badge-online' : 'badge-offline'">{{ node.blocked_reason || (node.default_no_schedule ? '数据库可调度' : '可调度') }}</span></td></tr><tr v-if="!nodes.length"><td colspan="3">暂无节点</td></tr></tbody></table></div>
        <p v-if="cluster.status === 'ready'" class="cell-secondary">保存后，新建或重建的实例使用更新后的节点范围；现有实例不会迁移。承载实例或数据卷的节点不能直接移出资源池。</p>
        <p v-else-if="!resourcePoolEditable" class="cell-secondary">请等待当前集群任务完成后再修改资源池。</p>
        <p v-else class="cell-secondary">资源池限定数据库实例可调度的节点。保存不会启动、停止或迁移现有实例；下次部署时生效。</p>
      </div>
      <template #actions><button class="btn" type="button" :disabled="resourcePoolSaving" @click="closeResourcePool">关闭</button><button v-if="resourcePoolEditable" class="btn btn-primary" type="button" :disabled="resourcePoolSaving || resourcePoolLoading || !validResourcePool" @click="saveResourcePool">{{ resourcePoolSaving ? '保存中...' : '保存资源池' }}</button></template>
    </BaseModal>
    <BaseModal :open="deploymentOpen" title="部署 MySQL" size="large" teleport :show-close="!saving" :close-on-overlay="!saving" :close-on-escape="!saving" @close="closeDeployment">
      <div class="deploy-form">
        <label class="form-group"><span class="form-label">本次部署实例数</span><input v-model.number="form.database_instances" class="form-input" type="number" min="1" max="9" step="1" aria-label="数据库实例数"></label>
        <label class="form-group"><span class="form-label">每个实例的数据存储大小</span><input v-model.trim="form.storage_size" class="form-input" placeholder="例如 10Gi" aria-label="每个数据库实例的容量"></label>
        <div class="node-picker-heading"><strong>本次 MySQL 候选节点</strong><span class="cell-secondary">已选择 {{ deploymentSelection.length }} 台</span></div>
        <EmptyState v-if="deploymentNodesLoading" variant="loading" message="正在读取资源池节点..." />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>选择</th><th>节点</th><th>地址</th><th>调度状态</th></tr></thead><tbody><tr v-for="node in databasePoolNodes" :key="node.name"><td><input v-model="deploymentSelection" type="checkbox" :value="node.name" :disabled="!available(node)" :aria-label="`${node.name} 作为本次 MySQL 候选节点`"></td><td>{{ node.name }}</td><td class="mono">{{ node.internal_ip || '-' }}</td><td class="cell-secondary">{{ node.blocked_reason || (available(node) ? '可调度' : '不可调度') }}</td></tr><tr v-if="!databasePoolNodes.length"><td colspan="4" class="cell-secondary">资源池内暂无可选节点</td></tr></tbody></table></div>
        <div class="deployment-context"><span>资源池 {{ cluster.database_nodes?.length || 0 }} 台节点</span><span v-if="preflight?.storage_class">存储类型 {{ preflight.storage_class }}</span></div>
        <p v-if="sharedDatabaseNode" class="cell-secondary">{{ form.database_instances }} 个实例将共享 {{ deploymentSelection.length }} 台节点；单台节点故障可能使集群失去多数派。</p>
        <div class="preflight-heading"><strong>部署前检查</strong><button class="icon-button" type="button" title="重新检查" aria-label="重新检查" :disabled="preflightLoading || !canDeploy" @click="runPreflight"><RefreshCw :size="15" /></button></div>
        <span v-if="preflightLoading" class="cell-secondary">正在检查资源池与数据存储...</span>
        <span v-else-if="preflight" class="badge" :class="preflight.ready ? 'badge-online' : 'badge-danger'">{{ preflight.ready ? '检查通过' : preflight.storage_reason || preflight.nodes?.find(node => !node.ready)?.reason || '部署条件未通过' }}</span>
      </div>
      <template #actions><button class="btn" type="button" :disabled="saving" @click="closeDeployment">取消</button><button v-if="!preflight?.ready" class="btn" type="button" :disabled="saving || preflightLoading || !canDeploy" @click="runPreflight">{{ preflightLoading ? '检查中...' : '检查部署条件' }}</button><button class="btn btn-primary" type="button" :disabled="saving || !canDeploy || !preflight?.ready || preflightLoading" @click="deploy">{{ saving ? '提交中...' : '确认部署' }}</button></template>
    </BaseModal>
    <BaseModal :open="scaleConfirmation !== ''" :title="scaleConfirmation === 'database' ? '添加数据库实例' : '添加 Router'" size="large" teleport :show-close="!scaling" :close-on-overlay="!scaling" :close-on-escape="!scaling" @close="scaleConfirmation = ''">
      <div class="deploy-form">
        <p>本次追加 1 个{{ scaleConfirmation === 'database' ? ' MySQL 实例' : ' Router' }}，总数从 {{ scaleConfirmation === 'database' ? cluster.database_instances : routerInstanceCount }} 个增加到 {{ (scaleConfirmation === 'database' ? cluster.database_instances : routerInstanceCount) + 1 }} 个。</p>
        <template v-if="scaleConfirmation === 'database'"><div class="node-picker-heading"><strong>本次 MySQL 候选节点</strong><span class="cell-secondary">已选择 {{ scaleSelection.length }} 台</span></div><EmptyState v-if="scaleNodesLoading" variant="loading" message="正在读取资源池节点..." /><div v-else class="table-wrap"><table class="data-table"><thead><tr><th>选择</th><th>节点</th><th>地址</th><th>调度状态</th></tr></thead><tbody><tr v-for="node in databasePoolNodes" :key="node.name"><td><input v-model="scaleSelection" type="checkbox" :value="node.name" :disabled="!available(node)" :aria-label="`${node.name} 作为新增 MySQL 实例候选节点`"></td><td>{{ node.name }}</td><td class="mono">{{ node.internal_ip || '-' }}</td><td class="cell-secondary">{{ node.blocked_reason || (available(node) ? '可调度' : '不可调度') }}</td></tr><tr v-if="!databasePoolNodes.length"><td colspan="4" class="cell-secondary">资源池内暂无可选节点</td></tr></tbody></table></div><p class="cell-secondary">新实例沿用现有存储配置（{{ cluster.storage_size || '未设置容量' }}）；现有实例不会迁移。</p></template>
        <p v-else class="cell-secondary">Router 候选节点：{{ cluster.router_nodes?.join('、') || '未配置' }}</p>
      </div>
      <template #actions><button class="btn" type="button" :disabled="Boolean(scaling)" @click="scaleConfirmation = ''">取消</button><button class="btn btn-primary" type="button" :disabled="Boolean(scaling) || (scaleConfirmation === 'database' && (scaleNodesLoading || !validScaleSelection))" @click="addInstance(scaleConfirmation)">{{ scaling ? '提交中...' : '确认追加' }}</button></template>
    </BaseModal>
    <BaseModal :open="routerDeploymentOpen" title="部署 Router" size="large" teleport :show-close="!scaling" :close-on-overlay="!scaling" :close-on-escape="!scaling" @close="routerDeploymentOpen = false">
      <div class="node-picker-heading"><strong>选择 Router 候选节点</strong><span class="cell-secondary">已选择 {{ routerSelection.length }} 台，至少 1 台</span></div>
      <EmptyState v-if="routerDeploymentLoading" variant="loading" message="正在读取节点..." />
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>选择</th><th>节点</th><th>地址</th><th>调度说明</th></tr></thead><tbody><tr v-for="node in routerPoolNodes" :key="node.name"><td><input v-model="routerSelection" type="checkbox" :value="node.name" :disabled="!routerAvailable(node) || (!routerSelection.includes(node.name) && routerSelection.length >= 9)" :aria-label="`${node.name} 部署 Router`"></td><td>{{ node.name }}</td><td class="mono">{{ node.internal_ip || '-' }}</td><td class="cell-secondary">{{ node.router_blocked_reason || (routerAvailable(node) ? '可部署 Router' : '不可部署 Router') }}</td></tr><tr v-if="!routerPoolNodes.length"><td colspan="4" class="cell-secondary">资源池内暂无候选节点，请先编辑资源池</td></tr></tbody></table></div>
      <p class="cell-secondary">本次部署 1 个 Router，Kubernetes 将在资源池内的所选节点中调度。</p>
      <template #actions><button class="btn" type="button" :disabled="Boolean(scaling)" @click="routerDeploymentOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="Boolean(scaling) || routerDeploymentLoading || !validRouterSelection" @click="deployRouter">{{ scaling === 'router' ? '提交中...' : '确认部署' }}</button></template>
    </BaseModal>
    <BaseModal :open="operatorDetailsOpen || Boolean(selectedTask)" :title="selectedTask ? `任务 #${selectedTask.id} · ${taskLabel(selectedTask)}` : 'MySQL Operator 状态'" size="large" teleport @close="closeLogs">
      <div class="task-progress"><span class="badge" :class="selectedTask?.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ checkingOperator ? (operatorUninstallPending ? '正在检查卸载状态...' : '正在检查安装状态...') : taskStatusLabel(selectedTask?.status || operatorState) }}</span><span>{{ selectedTask?.kind === 'uninstall_operator' ? selectedTask.last_progress : operatorDetailsOpen || selectedTask?.kind === 'install_operator' ? (selectedTask && selectedTask.status !== 'running' && selectedTask.status !== 'cancelling' ? selectedTask.last_progress : operatorProgress) : selectedTask?.last_progress }}</span></div>
      <div class="deployment-log" role="log"><div v-for="entry in logs" :key="entry.id">{{ formatTime(entry.created_at) }} {{ entry.message }}</div><span v-if="!logs.length" class="cell-secondary">{{ logsLoading ? '正在读取日志...' : '暂无任务日志' }}</span></div>
      <template #actions><button v-if="operatorDetailsOpen || ['install_operator', 'uninstall_operator'].includes(selectedTask?.kind)" class="btn" type="button" :disabled="checkingOperator" @click="refreshOperator">{{ checkingOperator ? '检查中...' : selectedTask?.kind === 'uninstall_operator' ? '检查卸载状态' : '检查安装状态' }}</button><button v-if="(operatorDetailsOpen || selectedTask?.kind === 'install_operator') && operatorCanCancel" class="btn btn-danger" type="button" :disabled="cancelling" @click="requestCancel">中止安装</button><button class="btn" type="button" @click="closeLogs">关闭</button></template>
    </BaseModal>
    <BaseModal :open="Boolean(resourceDetails)" :title="resourceDetails?.item?.name || '资源详情'" teleport @close="resourceDetails = null">
      <div v-if="resourceDetails" class="resource-modal-content">
        <template v-if="resourceDetails.kind === 'pod'">
          <div class="resource-modal-row"><span>角色</span><strong>{{ selectedResourcePod.component === 'mysqld' ? databaseRoleLabel(selectedResourcePod.database_role) : 'Router' }}</strong></div>
          <div class="resource-modal-row"><span>节点</span><strong>{{ selectedResourcePod.node_name || '等待调度' }}</strong></div>
          <div class="resource-modal-row"><span>状态</span><strong>{{ podStatusLabel(selectedResourcePod) }}</strong></div>
          <div class="resource-modal-row"><span>CPU 使用</span><strong>{{ formatPodCPU(selectedResourcePod.cpu_millicores) }}</strong></div>
          <div class="resource-modal-row"><span>内存使用</span><strong>{{ formatPodMemory(selectedResourcePod.memory_bytes) }}</strong></div>
          <div v-if="selectedResourcePod.container_usage?.length" class="resource-modal-block"><span>容器使用</span><ul class="resource-image-list"><li v-for="container in selectedResourcePod.container_usage" :key="container.name" class="resource-container-usage"><strong>{{ container.name }}</strong><span>CPU {{ formatPodCPU(container.cpu_millicores) }} · 内存 {{ formatPodMemory(container.memory_bytes) }}</span></li></ul></div>
          <div v-if="selectedResourcePod.message" class="resource-modal-block"><span>错误原因</span><p>{{ selectedResourcePod.message }}</p></div>
          <div v-if="selectedResourcePod.last_event" class="resource-modal-block"><span>最近事件</span><p>{{ selectedResourcePod.last_event }}</p></div>
          <div v-if="selectedResourcePod.claims?.length" class="resource-modal-block"><span>数据卷</span><ul class="resource-image-list"><li v-for="name in selectedResourcePod.claims" :key="name" class="mono">{{ name }} · {{ claimByName[name]?.phase || '等待创建' }}</li></ul></div>
          <div class="resource-modal-block"><span>镜像</span><ul v-if="uniqueImages(selectedResourcePod).length" class="resource-image-list"><li v-for="image in uniqueImages(selectedResourcePod)" :key="image" class="mono">{{ image }}</li></ul><p v-else>暂无镜像信息</p></div>
        </template>
        <template v-else><div class="resource-modal-row"><span>状态</span><strong>{{ resourceDetails.item.phase || '等待创建' }}</strong></div><div class="resource-modal-row"><span>存储类型</span><strong>{{ resourceDetails.item.storage_class || '-' }}</strong></div><div class="resource-modal-row"><span>容量</span><strong>{{ resourceDetails.item.capacity || '-' }}</strong></div><div class="resource-modal-row"><span>绑定卷</span><strong class="mono resource-modal-value">{{ resourceDetails.item.volume_name || '-' }}</strong></div><div v-if="resourceDetails.item.reason" class="resource-modal-block"><span>错误原因</span><p>{{ resourceDetails.item.reason }}</p></div></template>
      </div>
      <template #actions><button class="btn" type="button" @click="resourceDetails = null">关闭</button></template>
    </BaseModal>
    <ConfirmDialog :open="cancelOpen" title="中止 MySQL Operator 安装" :message="`中止后将停止 ${kubernetesClusterName} 的 Operator 安装，并由 Helm 清理本次安装资源；清理完成后可以重新安装。是否继续？`" confirm-text="中止安装" busy-text="正在中止..." :busy="cancelling" @confirm="cancelOperator" @cancel="cancelOpen = false" />
    <ConfirmDialog :open="uninstallOpen" title="卸载 MySQL Operator" :message="`将卸载 ${kubernetesClusterName} 共用的 MySQL Operator。该集群中仍有 InnoDBCluster 资源时会拒绝卸载；数据库 PVC 不会删除。是否继续？`" confirm-text="卸载 Operator" busy-text="正在卸载..." :busy="uninstalling" @confirm="uninstallOperator" @cancel="uninstallOpen = false" />
    <ConfirmDialog :open="pauseOpen" :title="isScalingTask ? '撤回本次实例追加' : '暂停 MySQL 初次部署'" :message="isScalingTask ? `将${currentDeploymentTask?.kind === 'scale_database' ? ' MySQL' : ' Router'} 目标数量恢复到本次追加前。新实例会退出；已有实例、集群资源和 PVC 保留。若新实例已就绪，系统会拒绝撤回。` : '初次部署尚未完成。暂停后 Operator 会移除当前集群资源及数据库、Router 负载，现有连接会中断；PVC 暂时保留。'" :confirm-text="isScalingTask ? '确认撤回' : '暂停初次部署'" busy-text="正在处理..." :busy="pausing" @confirm="pauseDeployment" @cancel="pauseOpen = false" />
    <ConfirmDialog :open="retryShutdownOpen" title="重试 Operator 清理" message="将重启此 Kubernetes 集群共用的 MySQL Operator，以重新处理卡住的删除流程；其他 MySQL 集群的管理协调会短暂中断。目标集群的数据卷会保留。" confirm-text="重试清理" busy-text="正在重试..." :busy="retryingShutdown" @confirm="retryShutdown" @cancel="retryShutdownOpen = false" />
    <ConfirmDialog :open="cleanupOpen" :title="cleanupClaims ? '清理 MySQL 数据卷' : '清理 MySQL 残留负载'" :message="cleanupClaims ? `将删除 ${cluster?.resource_name} 的 ${cleanupState?.claims?.length || 0} 个 PVC，卷内数据库数据会永久丢失。请确认已有可恢复的备份。` : `将清理 ${cluster?.resource_name} 的残留工作负载和 Pod。数据卷会保留；待负载完全退出后可单独清理数据卷。`" :confirm-text="cleanupClaims ? '删除数据卷' : '清理负载'" busy-text="正在清理..." :busy="cleaning" @confirm="cleanupResources" @cancel="cleanupOpen = false" />
    <ConfirmDialog :open="Boolean(switchTarget)" title="切换 MySQL 主实例" :message="`将 ${switchTarget?.node_name || ''} 上的 ${switchTarget?.name || ''} 切换为主实例。切换期间已有连接可能短暂中断，应用需重新连接。`" confirm-text="切换主实例" busy-text="正在切换..." confirm-tone="primary" :busy="switchingPrimary" @confirm="switchPrimary" @cancel="switchTarget = null" />
    <ConfirmDialog :open="Boolean(retryTarget)" title="重试异常 Pod" :message="`将删除异常 Pod ${retryTarget?.name || ''}，由原控制器在原有集群中重建；PVC 和已就绪实例不会删除。请先排除镜像、存储或节点故障。`" confirm-text="重建 Pod" busy-text="正在重建..." confirm-tone="primary" :busy="retryingPod" @confirm="retryPod" @cancel="retryTarget = null" />
    <ConfirmDialog :open="routerRepairOpen" title="修正 Router 调度" :message="`Router 当前运行在候选节点范围外。将按已保存的候选节点（${cluster?.router_nodes?.join('、') || '无'}）更新调度规则并滚动替换 Router Pod；替换期间连接可能短暂中断。`" confirm-text="修正调度" busy-text="正在提交..." confirm-tone="primary" :busy="repairingRouter" @confirm="repairRouterPlacement" @cancel="routerRepairOpen = false" />
    <ConfirmDialog :open="routerRetireOpen" title="清退 Router" :message="`将 Router 从 ${routerInstanceCount} 个缩减到 ${routerInstanceCount - 1} 个。Kubernetes 会决定终止哪个 Pod${routerInstanceCount === 1 ? '；这是最后一个 Router，读写入口将不可用' : '；该 Pod 上的连接可能中断'}。MySQL 实例和数据卷会保留。`" confirm-text="确认清退" busy-text="正在提交..." :busy="retiringRouter" @confirm="retireRouter" @cancel="routerRetireOpen = false" />
    <ErrorNoticeModal :open="Boolean(error)" title="MySQL 集群操作失败" :message="error" teleport @close="error = ''" />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { ArrowDown, ArrowLeft, ArrowUpCircle, Database, RefreshCw, Server, Settings2, SquareTerminal, Trash2 } from 'lucide-vue-next'
import { getClusters } from '../../api/clusters.js'
import { formatBytes } from '../../utils/formatters.js'
import { addMySQLClusterInstance, addMySQLClusterRouter, cancelMySQLOperator, cleanupMySQLCluster, deployMySQLCluster, getMySQLCluster, getMySQLClusterCleanup, getMySQLClusterConfig, getMySQLClusterNodes, getMySQLClusterRuntime, getMySQLClusterTaskLogs, getMySQLClusterTasks, getMySQLOperator, installMySQLOperator, pauseMySQLClusterDeployment, preflightMySQLCluster, pullMySQLClusterPodImages, repairMySQLClusterRouterPlacement, restartMySQLClusterForConfig, retireMySQLClusterRouter, retryMySQLClusterPod, retryMySQLClusterShutdown, switchMySQLClusterPrimary, uninstallMySQLOperator, updateMySQLClusterConfig, updateMySQLClusterResourcePool } from '../../api/databases.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import PodTerminal from '../resources/PodTerminal.vue'

const props = defineProps({ id: { type: String, required: true } })
const tabs = [{ id: 'topology', label: '拓扑' }, { id: 'resources', label: '部署资源' }, { id: 'tasks', label: '集群任务' }]
const activeTab = ref('topology'), cluster = ref(null), nodes = ref([]), kubernetesClusters = ref([]), tasks = ref([]), logs = ref([])
const taskPageSize = 10
const taskOffset = ref(0), taskTotal = ref(0), tasksLoading = ref(false), uninstallTaskPending = ref(false)
const taskPage = computed(() => Math.floor(taskOffset.value / taskPageSize) + 1)
const taskPageCount = computed(() => Math.max(1, Math.ceil(taskTotal.value / taskPageSize)))
const runtime = ref({ pods: [] }), selectedTask = ref(null)
const loading = ref(true), refreshing = ref(false), saving = ref(false), installing = ref(false), deploymentOpen = ref(false), error = ref(''), operatorState = ref('checking')
const checkingOperator = ref(false), operatorDetailsOpen = ref(false), operatorProgress = ref(''), operatorCanCancel = ref(false), cancelOpen = ref(false), cancelling = ref(false), logsLoading = ref(false)
const operatorCanUninstall = ref(false), uninstallOpen = ref(false), uninstalling = ref(false)
const switchTarget = ref(null), switchingPrimary = ref(false)
const retryTarget = ref(null), retryingPod = ref(false)
const resourceDetails = ref(null)
const connectionDetailsOpen = ref(false)
const terminalPod = ref(null)
const configOpen = ref(false), configLoading = ref(false), configSaving = ref(false), configRestartOpen = ref(false), configRestarting = ref(false)
const configText = ref(''), configSaved = ref(''), configRestartRequired = ref(false)
const pullingPod = ref('')
const scaling = ref('')
const scaleConfirmation = ref('')
const deploymentNodesLoading = ref(false), deploymentSelection = ref([])
const scaleNodesLoading = ref(false), scaleSelection = ref([])
const routerDeploymentOpen = ref(false), routerDeploymentLoading = ref(false), routerSelection = ref([])
const routerRepairOpen = ref(false), repairingRouter = ref(false)
const routerRetireOpen = ref(false), retiringRouter = ref(false)
const resourcePoolOpen = ref(false), resourcePoolLoading = ref(false), resourcePoolSaving = ref(false), resourcePoolSelection = ref([])
const pauseOpen = ref(false), pausing = ref(false)
const retryShutdownOpen = ref(false), retryingShutdown = ref(false)
const cleanupOpen = ref(false), cleanupClaims = ref(false), cleaning = ref(false), cleanupState = ref(null)
const preflight = ref(null), preflightLoading = ref(false)
const operatorPending = computed(() => ['installing', 'failed', 'cancelling'].includes(operatorState.value))
const operatorUninstallPending = computed(() => uninstallTaskPending.value)
const form = reactive({ database_instances: 1, storage_size: '10Gi' })
const sharedDatabaseNode = computed(() => form.database_instances > deploymentSelection.value.length)
const databasePoolNodes = computed(() => nodes.value.filter(node => cluster.value?.database_nodes?.includes(node.name)))
const validDatabaseSelection = selection => selection.length >= 1 && selection.length <= 9 && selection.every(name => databasePoolNodes.value.some(node => node.name === name && available(node)))
const validScaleSelection = computed(() => validDatabaseSelection(scaleSelection.value))
const routerPoolNodes = computed(() => nodes.value.filter(node => cluster.value?.database_nodes?.includes(node.name)))
const validRouterSelection = computed(() => routerSelection.value.length >= 1 && routerSelection.value.length <= 9 && routerSelection.value.every(name => routerPoolNodes.value.some(node => node.name === name && routerAvailable(node))))
const hasResidualResources = computed(() => Boolean(cleanupState.value?.cluster_resource || cleanupState.value?.workloads?.length || cleanupState.value?.pods?.length || cleanupState.value?.claims?.length))
const hasResidualWorkloads = computed(() => Boolean(cleanupState.value?.workloads?.length || cleanupState.value?.pods?.length))
const showOperatorUninstall = computed(() => isK3s.value && operatorCanUninstall.value && !operatorUninstallPending.value && (
  cluster.value?.status === 'draft' || (cluster.value?.status === 'paused' && cleanupState.value && !cleanupState.value.cluster_resource && !hasResidualWorkloads.value)
))
const pausedMessage = computed(() => cleanupState.value?.cluster_resource ? `Operator 正在停止数据库负载${cleanupState.value.deletion_started_at ? `（删除请求：${formatTime(cleanupState.value.deletion_started_at)}）` : ''}；${cleanupState.value.retry_available ? '等待超时，可重试 Operator 清理' : cleanupState.value.recent_restart ? 'Operator 最近已重启，请等待清理结果' : '请等待资源退出'}` : hasResidualResources.value ? `待清理：${cleanupState.value.workloads.length} 个工作负载、${cleanupState.value.pods.length} 个 Pod、${cleanupState.value.claims.length} 个数据卷` : '部署已暂停，可重新部署')
const editable = computed(() => ['draft', 'error', 'missing'].includes(cluster.value?.status) || (cluster.value?.status === 'paused' && cleanupState.value && !hasResidualResources.value))
const resourcePoolEditable = computed(() => ['draft', 'paused', 'ready'].includes(cluster.value?.status))
const validResourcePool = computed(() => resourcePoolSelection.value.length >= 1 && resourcePoolSelection.value.length <= 9 && resourcePoolSelection.value.every(name => nodes.value.some(node => node.name === name && available(node))))
const currentDeploymentTask = computed(() => taskOffset.value === 0 ? tasks.value.find(task => ['deploy', 'scale_database', 'scale_router', 'scale_router_down'].includes(task.kind)) : null)
const isScalingTask = computed(() => ['scale_database', 'scale_router'].includes(currentDeploymentTask.value?.kind))
const canPauseDeployment = computed(() => (cluster.value?.status === 'starting' && currentDeploymentTask.value?.status === 'running' && ['deploy', 'scale_database', 'scale_router'].includes(currentDeploymentTask.value.kind)) || (cluster.value?.status === 'error' && currentDeploymentTask.value?.kind === 'deploy' && currentDeploymentTask.value?.status === 'failed'))
const canDeploy = computed(() => !deploymentNodesLoading.value && validDatabaseSelection(deploymentSelection.value) && Number.isInteger(form.database_instances) && form.database_instances >= 1 && form.database_instances <= 9 && /^([1-9][0-9]*)(Mi|Gi|Ti)$/.test(form.storage_size))
const routerInstanceCount = computed(() => cluster.value?.router_instances ?? cluster.value?.router_nodes?.length ?? 0)
const canAddDatabase = computed(() => cluster.value?.status === 'ready' && operatorState.value === 'ready' && (cluster.value.database_instances || 0) < 9)
const canAddRouter = computed(() => cluster.value?.status === 'ready' && operatorState.value === 'ready' && (routerInstanceCount.value === 0 || routerInstanceCount.value < (cluster.value.router_nodes?.length || 0)))
const canRetireRouter = computed(() => cluster.value?.status === 'ready' && operatorState.value === 'ready' && routerInstanceCount.value > 0)
const isK3s = computed(() => kubernetesClusters.value.find(item => Number(item.id) === Number(cluster.value?.cluster_id))?.distribution === 'k3s')
const kubernetesClusterName = computed(() => kubernetesClusters.value.find(item => Number(item.id) === Number(cluster.value?.cluster_id))?.name || cluster.value?.name || '当前集群')
const routerPods = computed(() => (runtime.value.pods || []).filter(pod => pod.component === 'mysqlrouter'))
const routerPlacementMismatch = computed(() => routerPods.value.some(pod => pod.node_name && !cluster.value?.router_nodes?.includes(pod.node_name)))
const databasePods = computed(() => (runtime.value.pods || []).filter(pod => pod.component === 'mysqld'))
const selectedResourcePod = computed(() => resourceDetails.value?.kind === 'pod' ? (runtime.value.pods || []).find(pod => pod.name === resourceDetails.value.item.name) || resourceDetails.value.item : null)
const available = node => node.database_allowed ?? (node.ready && !node.evicted)
const routerAvailable = node => node.router_allowed ?? (node.ready && !node.evicted && !node.default_no_schedule)
const databaseRoleLabel = value => ({ PRIMARY: '主实例', primary: '主实例', SECONDARY: '副本实例', secondary: '副本实例' }[value] || '角色确认中')
const uniqueImages = pod => [...new Set(pod.images || [])]
const formatPodCPU = value => value == null ? '-' : `${value} m`
const formatPodMemory = value => value == null ? '-' : formatBytes(value)
const podUsageLabel = pod => pod.cpu_millicores == null && pod.memory_bytes == null ? '实时指标暂不可用' : `CPU ${formatPodCPU(pod.cpu_millicores)} · 内存 ${formatPodMemory(pod.memory_bytes)}`
const podStatusLabel = pod => {
  if (pod.ready) return '就绪'
  if (pod.reason === 'PodInitializing') return pod.last_event?.startsWith('Pulling:') ? '拉取镜像中' : '初始化中'
  if (pod.reason === 'ContainerCreating') return '创建容器中'
  if (pod.reason === 'Running' || (!pod.reason && pod.phase === 'Running')) return '启动中'
  return pod.reason || pod.phase || '等待中'
}
const canPullPodImage = pod => !pod.ready && Boolean(pod.node_name) && (pod.reason === 'PodInitializing' || ['ImagePullBackOff', 'ErrImagePull', 'ContainerCreating'].includes(pod.reason))
const claimByName = computed(() => Object.fromEntries((runtime.value.claims || []).map(claim => [claim.name, claim])))
const unattachedClaims = computed(() => {
  const attached = new Set((runtime.value.pods || []).flatMap(pod => pod.claims || []))
  return (runtime.value.claims || []).filter(claim => !attached.has(claim.name))
})
const statusLabel = value => ({ draft: '待部署', starting: '部署中', ready: '运行中', missing: '资源缺失', error: '异常', paused: '已暂停' }[value] || '待检查')
const taskLabel = task => ({ install_operator: '安装 MySQL Operator', uninstall_operator: '卸载 MySQL Operator', switch_primary: '切换 MySQL 主实例', retry_shutdown: '重试 Operator 清理', scale_database: '添加数据库实例', scale_router: '部署 Router', scale_router_down: '清退 Router' }[task.kind] || '部署 MySQL')
const taskStatusLabel = value => ({ running: '运行中', success: '成功', failed: '失败', paused: '已暂停', installing: '安装中', ready: '已就绪', missing: '未安装', cancelling: '正在中止', cancelled: '已中止', checking: '检查中' }[value] || value)
const formatTime = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-'

function setCluster(item) {
  cluster.value = item
  if (!deploymentOpen.value) Object.assign(form, { database_instances: item.database_instances || 1, storage_size: item.storage_size || '10Gi' })
}
async function openMySQLConfig() {
  configOpen.value = true
  configLoading.value = true
  try {
    const result = await getMySQLClusterConfig(props.id)
    configText.value = result.mycnf || ''
    configSaved.value = configText.value
    configRestartRequired.value = Boolean(result.restart_required)
  } catch (failure) { configOpen.value = false; error.value = failure.message || '读取 MySQL 配置失败' }
  finally { configLoading.value = false }
}
async function saveMySQLConfig() {
  if (configSaving.value || configLoading.value || configText.value === configSaved.value) return
  configSaving.value = true
  try {
    const item = await updateMySQLClusterConfig(props.id, configText.value)
    setCluster(item)
    const result = await getMySQLClusterConfig(props.id)
    configText.value = result.mycnf || ''
    configSaved.value = configText.value
    configRestartRequired.value = Boolean(result.restart_required)
  } catch (failure) { error.value = failure.message || '保存 MySQL 配置失败' }
  finally { configSaving.value = false }
}
function openConfigRestartConfirmation() {
  configOpen.value = false
  configRestartOpen.value = true
}
function cancelConfigRestartConfirmation() {
  configRestartOpen.value = false
  configOpen.value = true
}
async function restartMySQLConfig() {
  if (configRestarting.value) return
  configRestarting.value = true
  try {
    await restartMySQLClusterForConfig(props.id)
    configRestartOpen.value = false
    configOpen.value = false
    await refresh()
  } catch (failure) { error.value = failure.message || '提交 MySQL 滚动重启失败' }
  finally { configRestarting.value = false }
}
async function openResourcePool() {
  resourcePoolSelection.value = [...(cluster.value?.database_nodes || [])]
  resourcePoolOpen.value = true
  resourcePoolLoading.value = true
  try { nodes.value = await getMySQLClusterNodes(props.id) }
  catch (failure) { resourcePoolOpen.value = false; error.value = failure.message || '读取资源池节点失败' }
  finally { resourcePoolLoading.value = false }
}
function closeResourcePool() { if (!resourcePoolSaving.value) resourcePoolOpen.value = false }
async function saveResourcePool() {
  if (!resourcePoolEditable.value || !validResourcePool.value || resourcePoolSaving.value) return
  resourcePoolSaving.value = true
  try {
    const item = await updateMySQLClusterResourcePool(props.id, [...resourcePoolSelection.value])
    setCluster(item)
    resourcePoolOpen.value = false
  } catch (failure) { error.value = failure.message || '保存数据库资源池失败' }
  finally { resourcePoolSaving.value = false }
}
async function load() {
  loading.value = true
  try {
    const item = await getMySQLCluster(props.id)
    setCluster(item)
    const [clustersResult, detailResult] = await Promise.allSettled([getClusters(), refreshData(item)])
    if (clustersResult.status === 'fulfilled') kubernetesClusters.value = clustersResult.value
    if (detailResult.status === 'rejected') throw detailResult.reason
    if (clustersResult.status === 'rejected') throw clustersResult.reason
  } catch (failure) { error.value = failure.message || '读取 MySQL 集群失败' }
  finally { loading.value = false }
}
let refreshRequest
async function refresh(silent = false) {
  if (!cluster.value) return
  if (!refreshRequest) {
    refreshing.value = true
    refreshRequest = refreshData().finally(() => { refreshing.value = false; refreshRequest = null })
  }
  return refreshRequest.catch(failure => { if (!silent) error.value = failure.message || '刷新 MySQL 集群失败' })
}
let taskRequestID = 0
async function loadTasks() {
  const requestID = ++taskRequestID
  const clusterID = props.id
  tasksLoading.value = true
  try {
    const result = await getMySQLClusterTasks(clusterID, { limit: taskPageSize, offset: taskOffset.value })
    if (requestID !== taskRequestID || clusterID !== props.id) return
    tasks.value = result.items || []
    taskTotal.value = result.total || 0
    uninstallTaskPending.value = Boolean(result.operator_uninstall_pending)
    if (selectedTask.value) {
      const task = tasks.value.find(entry => entry.id === selectedTask.value.id)
      if (task) selectedTask.value = task
      const taskID = selectedTask.value.id
      const entries = await getMySQLClusterTaskLogs(clusterID, taskID)
      if (requestID === taskRequestID && clusterID === props.id && selectedTask.value?.id === taskID) logs.value = entries
    }
  } finally {
    if (requestID === taskRequestID) tasksLoading.value = false
  }
}
async function changeTaskPage(direction) {
  if (tasksLoading.value) return
  const offset = taskOffset.value + direction * taskPageSize
  if (offset < 0 || offset >= taskTotal.value) return
  closeLogs()
  taskOffset.value = offset
  try { await loadTasks() }
  catch (failure) { error.value = failure.message || '读取集群任务失败' }
}
function showLatestTasks() {
  taskOffset.value = 0
  activeTab.value = 'tasks'
  void loadTasks().catch(failure => { error.value = failure.message || '读取集群任务失败' })
}
async function refreshData(currentItem) {
  const item = currentItem || await getMySQLCluster(props.id)
  setCluster(item)
  const results = await Promise.allSettled([
    getMySQLOperator(props.id).then(operator => {
      operatorState.value = operator.state
      operatorProgress.value = operator.progress || ''
      operatorCanCancel.value = Boolean(operator.can_cancel)
      operatorCanUninstall.value = Boolean(operator.can_uninstall)
    }),
    (item.status === 'draft' ? Promise.resolve(null) : item.status === 'paused' ? getMySQLClusterCleanup(props.id) : getMySQLClusterRuntime(props.id)).then(snapshot => {
      if (item.status === 'paused') {
        cleanupState.value = snapshot
        runtime.value = { pods: [] }
      } else if (item.status === 'draft') {
        cleanupState.value = null
        runtime.value = { pods: [] }
      } else {
        runtime.value = snapshot
        item.status = snapshot.status
        item.online_instances = snapshot.online_instances
        item.router_ready = snapshot.router_ready
        if (snapshot.failure) { item.last_error = snapshot.failure; if (cluster.value?.last_error !== snapshot.failure) error.value = snapshot.failure }
        setCluster(item)
      }
    }),
    loadTasks(),
  ])
  const failure = results.find(result => result.status === 'rejected')
  if (failure) throw failure.reason
}
async function refreshOperator() {
  if (checkingOperator.value) return
  checkingOperator.value = true
  operatorDetailsOpen.value = true
  showLatestTasks()
  selectedTask.value = tasks.value.find(task => ['install_operator', 'uninstall_operator'].includes(task.kind)) || null
  try {
    await refresh()
    await loadTasks()
    const task = tasks.value.find(task => ['install_operator', 'uninstall_operator'].includes(task.kind))
    if (task && operatorDetailsOpen.value) await openLogs(task)
  }
  catch (failure) { error.value = failure.message || '检查 MySQL Operator 失败' }
  finally { checkingOperator.value = false }
}
async function cancelOperator() {
  if (cancelling.value) return
  cancelling.value = true
  try {
    const result = await cancelMySQLOperator(props.id)
    operatorState.value = result.state
    operatorProgress.value = result.progress || ''
    operatorCanCancel.value = Boolean(result.can_cancel)
    cancelOpen.value = false
    showLatestTasks()
    // Wait for any older poll before reading the cancellation result.
    if (refreshRequest) await refreshRequest
    await refresh()
  } catch (failure) { cancelOpen.value = false; error.value = failure.message || '中止 MySQL Operator 安装失败' }
  finally { cancelling.value = false }
}
async function pauseDeployment() {
  if (pausing.value) return
  pausing.value = true
  try {
    await pauseMySQLClusterDeployment(props.id)
    pauseOpen.value = false
    if (refreshRequest) await refreshRequest
    await refresh()
  } catch (failure) { pauseOpen.value = false; error.value = failure.message || '暂停部署失败' }
  finally { pausing.value = false }
}
async function retryShutdown() {
  if (retryingShutdown.value) return
  retryingShutdown.value = true
  try {
    const result = await retryMySQLClusterShutdown(props.id)
    retryShutdownOpen.value = false
    showLatestTasks()
    await refresh()
    if (result.task) await openLogs(result.task)
  } catch (failure) { retryShutdownOpen.value = false; error.value = failure.message || '重试 Operator 清理失败' }
  finally { retryingShutdown.value = false }
}
async function cleanupResources() {
  if (cleaning.value || !cluster.value?.resource_name) return
  cleaning.value = true
  try {
    await cleanupMySQLCluster(props.id, cluster.value.resource_name, cleanupClaims.value)
    cleanupOpen.value = false
    await refresh()
  } catch (failure) { cleanupOpen.value = false; error.value = failure.message || '清理 MySQL 残留资源失败' }
  finally { cleaning.value = false }
}
async function installOperator() {
  installing.value = true
  try {
    const result = await installMySQLOperator(props.id)
    operatorState.value = result.state
    if (result.task) tasks.value = [result.task, ...tasks.value.filter(task => task.id !== result.task.id)]
    showLatestTasks()
    await refresh()
  }
  catch (failure) { error.value = failure.message || '安装 MySQL Operator 失败'; showLatestTasks(); await refresh() }
  finally { installing.value = false }
}
async function uninstallOperator() {
  if (uninstalling.value) return
  uninstalling.value = true
  try {
    const result = await uninstallMySQLOperator(props.id)
    uninstallOpen.value = false
    operatorCanUninstall.value = false
    if (result.task) tasks.value = [result.task, ...tasks.value.filter(task => task.id !== result.task.id)]
    showLatestTasks()
    await refresh()
    if (result.task) await openLogs(result.task)
  } catch (failure) { uninstallOpen.value = false; error.value = failure.message || '卸载 MySQL Operator 失败' }
  finally { uninstalling.value = false }
}
async function openDeployment() {
  preflight.value = null
  deploymentSelection.value = [...(cluster.value?.database_nodes || [])]
  deploymentOpen.value = true
  deploymentNodesLoading.value = true
  try { nodes.value = await getMySQLClusterNodes(props.id) }
  catch (failure) { deploymentOpen.value = false; error.value = failure.message || '读取资源池节点失败'; return }
  finally { deploymentNodesLoading.value = false }
  if (canDeploy.value) await runPreflight()
}
async function runPreflight() {
  if (!canDeploy.value) return
  preflightLoading.value = true
  preflight.value = null
  const payload = { database_instances: form.database_instances, storage_size: form.storage_size, candidate_nodes: [...deploymentSelection.value] }
  const fingerprint = JSON.stringify(payload)
  try {
    const result = await preflightMySQLCluster(props.id, payload)
    if (fingerprint === JSON.stringify({ database_instances: form.database_instances, storage_size: form.storage_size, candidate_nodes: [...deploymentSelection.value] })) preflight.value = result
  }
  catch (failure) { error.value = failure.message || '部署前检查失败' }
  finally { preflightLoading.value = false }
}
watch(form, () => { preflight.value = null })
watch(deploymentSelection, () => { preflight.value = null }, { deep: true })
function closeDeployment() { if (!saving.value) deploymentOpen.value = false }
async function deploy() {
  if (saving.value || !canDeploy.value || !preflight.value?.ready) return
  saving.value = true
  try {
    const result = await deployMySQLCluster(props.id, { database_instances: form.database_instances, storage_size: form.storage_size, candidate_nodes: [...deploymentSelection.value] })
    deploymentOpen.value = false
    setCluster(result.cluster)
    tasks.value = [result.task, ...tasks.value]
    showLatestTasks()
    await refresh()
  } catch (failure) { deploymentOpen.value = false; error.value = failure.message || '部署失败'; await refresh() }
  finally { saving.value = false }
}
async function addInstance(component) {
  if (scaling.value || scaleConfirmation.value !== component || (component === 'database' ? !canAddDatabase.value || scaleNodesLoading.value || !validScaleSelection.value : !canAddRouter.value)) return
  scaling.value = component
  try {
    const result = await (component === 'database' ? addMySQLClusterInstance(props.id, { candidate_nodes: [...scaleSelection.value] }) : addMySQLClusterRouter(props.id))
    scaleConfirmation.value = ''
    setCluster(result.cluster)
    showLatestTasks()
    await refresh()
    if (result.task) await openLogs(result.task)
  } catch (failure) {
    error.value = failure.message || '追加实例失败'
    await refresh()
  } finally { scaling.value = '' }
}
async function openDatabaseScaling() {
  scaleSelection.value = [...(cluster.value?.database_nodes || [])]
  scaleConfirmation.value = 'database'
  scaleNodesLoading.value = true
  try { nodes.value = await getMySQLClusterNodes(props.id) }
  catch (failure) { scaleConfirmation.value = ''; error.value = failure.message || '读取资源池节点失败' }
  finally { scaleNodesLoading.value = false }
}
async function openRouterDeployment() {
  routerSelection.value = (cluster.value?.router_nodes || []).filter(name => cluster.value?.database_nodes?.includes(name))
  routerDeploymentOpen.value = true
  routerDeploymentLoading.value = true
  try {
    nodes.value = await getMySQLClusterNodes(props.id)
  } catch (failure) { routerDeploymentOpen.value = false; error.value = failure.message || '读取 Router 候选节点失败' }
  finally { routerDeploymentLoading.value = false }
}
async function deployRouter() {
  if (scaling.value || routerDeploymentLoading.value || !validRouterSelection.value) return
  scaling.value = 'router'
  try {
    const result = await addMySQLClusterRouter(props.id, { router_nodes: [...routerSelection.value] })
    routerDeploymentOpen.value = false
    setCluster(result.cluster)
    showLatestTasks()
    await refresh()
    if (result.task) await openLogs(result.task)
  } catch (failure) {
    error.value = failure.message || '部署 Router 失败'
    await refresh()
  } finally { scaling.value = '' }
}
async function repairRouterPlacement() {
  if (repairingRouter.value || !routerPlacementMismatch.value) return
  repairingRouter.value = true
  try {
    await repairMySQLClusterRouterPlacement(props.id)
    routerRepairOpen.value = false
    await refresh()
  } catch (failure) { error.value = failure.message || '修正 Router 调度失败' }
  finally { repairingRouter.value = false }
}
async function retireRouter() {
  if (retiringRouter.value || !canRetireRouter.value) return
  retiringRouter.value = true
  try {
    const result = await retireMySQLClusterRouter(props.id)
    routerRetireOpen.value = false
    setCluster(result.cluster)
    showLatestTasks()
    await refresh()
    if (result.task) await openLogs(result.task)
  } catch (failure) {
    error.value = failure.message || '清退 Router 失败'
    await refresh()
  } finally { retiringRouter.value = false }
}
async function openLogs(task) {
  selectedTask.value = task; logs.value = []
  logsLoading.value = true
  try {
    const entries = await getMySQLClusterTaskLogs(props.id, task.id)
    if (selectedTask.value?.id === task.id) logs.value = entries
  }
  catch (failure) { error.value = failure.message || '读取部署日志失败' }
  finally { logsLoading.value = false }
}
function requestCancel() { closeLogs(); cancelOpen.value = true }
async function switchPrimary() {
  if (!switchTarget.value || switchingPrimary.value) return
  switchingPrimary.value = true
  try {
    const result = await switchMySQLClusterPrimary(props.id, switchTarget.value.name)
    switchTarget.value = null
    await refresh()
    for (const pod of runtime.value.pods || []) {
      if (pod.component === 'mysqld' && pod.ready) pod.database_role = pod.name === result.pod_name ? 'PRIMARY' : 'SECONDARY'
    }
  } catch (failure) {
    switchTarget.value = null
    error.value = failure.message || '切换主实例失败'
    await refresh()
  } finally { switchingPrimary.value = false }
}
async function retryPod() {
  if (!retryTarget.value || retryingPod.value) return
  retryingPod.value = true
  try {
    await retryMySQLClusterPod(props.id, retryTarget.value.name)
    retryTarget.value = null
    await refresh()
  } catch (failure) { retryTarget.value = null; error.value = failure.message || '重建 Pod 失败' }
  finally { retryingPod.value = false }
}
async function pullPodImages(pod) {
  if (pullingPod.value) return
  pullingPod.value = pod.name
  try { await pullMySQLClusterPodImages(props.id, pod.name); await refresh() }
  catch (failure) { error.value = failure.message || '节点拉取镜像失败' }
  finally { pullingPod.value = '' }
}
function closeLogs() { selectedTask.value = null; operatorDetailsOpen.value = false }
let polling
watch(() => props.id, () => { clearInterval(polling); taskRequestID++; cluster.value = null; runtime.value = { pods: [], claims: [] }; cleanupState.value = null; tasks.value = []; taskOffset.value = 0; taskTotal.value = 0; tasksLoading.value = false; uninstallTaskPending.value = false; selectedTask.value = null; resourceDetails.value = null; connectionDetailsOpen.value = false; terminalPod.value = null; configOpen.value = false; configRestartOpen.value = false; scaleConfirmation.value = ''; routerDeploymentOpen.value = false; routerRepairOpen.value = false; routerRetireOpen.value = false; operatorDetailsOpen.value = false; cancelOpen.value = false; uninstallOpen.value = false; pauseOpen.value = false; retryShutdownOpen.value = false; cleanupOpen.value = false; switchTarget.value = null; retryTarget.value = null; activeTab.value = 'topology'; void load() }, { immediate: true })
watch([activeTab, () => cluster.value?.status, operatorState, operatorUninstallPending], () => { clearInterval(polling); if (cluster.value && (cluster.value.status !== 'draft' || ['installing', 'cancelling'].includes(operatorState.value) || operatorUninstallPending.value)) polling = setInterval(() => { void refresh(true) }, 5000) })
onBeforeUnmount(() => clearInterval(polling))
</script>

<style scoped>
.mysql-detail { min-width: 0; }
.task-progress { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; margin-bottom: var(--space-12); color: var(--text-secondary); font-size: 12px; overflow-wrap: anywhere; }
.back-link { display: inline-flex; width: fit-content; align-items: center; gap: 6px; margin-bottom: var(--space-16); color: var(--text-secondary); font-size: 13px; font-weight: 600; text-decoration: none; }
.back-link:hover { color: var(--action-primary); }
.detail-content { margin-top: var(--tabbed-page-content-gap); }
.detail-toolbar { display: flex; min-height: 62px; align-items: center; flex-wrap: wrap; gap: 12px; padding: 12px var(--space-20); }
.toolbar-actions { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-left: auto; }
.topology-empty { min-height: 280px; border-top: 1px solid var(--border-muted); background: var(--surface-subtle); }
.mysql-topology { display: grid; align-content: start; justify-items: center; padding: 30px var(--space-20) 42px; border-top: 1px solid var(--border-muted); background: var(--surface-subtle); }
.topology-stage { width: 100%; min-width: 0; }
.topology-stage-heading { display: flex; align-items: center; justify-content: center; gap: 10px; margin-bottom: 16px; }
.topology-stage-heading > div { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; }
.topology-stage-heading .topology-stage-actions { align-items: center; }
.topology-stage-heading strong { color: var(--text-primary); font-size: 13px; }
.stage-icon { display: grid; width: 34px; height: 34px; flex: 0 0 34px; place-items: center; border-radius: var(--radius-control); background: var(--surface-hover); color: var(--action-primary); }
.cell-secondary { color: var(--text-secondary); font-size: 11px; overflow-wrap: anywhere; }
.connection-details { display: grid; gap: 24px; }
.connection-section { display: grid; gap: 10px; min-width: 0; }
.connection-section + .connection-section { border-top: 1px solid var(--border-muted); padding-top: 20px; }
.connection-section h3 { margin: 0; color: var(--text-primary); font-size: 13px; }
.connection-row { display: grid; grid-template-columns: minmax(130px, 180px) minmax(0, 1fr); gap: 12px; align-items: start; min-width: 0; font-size: 12px; }
.connection-row > span { color: var(--text-secondary); overflow-wrap: anywhere; }
.connection-row code { color: var(--text-primary); font: 12px/1.5 var(--font-mono); overflow-wrap: anywhere; }
.topology-nodes { display: flex; justify-content: center; flex-wrap: wrap; align-items: stretch; gap: 14px; width: 100%; }
.topology-node { display: grid; align-content: start; gap: 10px; width: min(340px, 100%); min-width: 0; padding: 18px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow-soft); }
.topology-node-identity { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; min-width: 0; }
.topology-node-identity strong { min-width: 0; flex: 1 1 110px; color: var(--text-primary); font-size: 13px; overflow-wrap: anywhere; }
.node-icon { display: grid; width: 32px; height: 32px; flex: 0 0 32px; place-items: center; border-radius: var(--radius-control); background: var(--surface-hover); color: var(--action-primary); }
.pod-name { min-width: 0; color: var(--text-secondary); font-size: 11px; overflow-wrap: anywhere; }
.topology-node-usage { font-size: 11px; }
.topology-flow { display: flex; height: 78px; align-items: center; flex-direction: column; color: var(--action-primary); }
.topology-flow::before { width: 1px; height: 48px; background: currentColor; content: ''; }
.mono { font-family: var(--font-mono); }
.preflight-heading strong { font-size: 12px; }
.resource-table { padding: 12px var(--space-20) 14px; overflow-x: auto; }
.resource-table .data-table { min-width: 1120px; table-layout: fixed; }
.resource-table .data-table td { height: 48px; }
.resource-table .data-table th:first-child { width: 24%; }
.resource-table .data-table th:nth-child(2) { width: 11%; }
.resource-table .data-table th:nth-child(3) { width: 13%; }
.resource-table .data-table th:nth-child(4) { width: 10%; }
.resource-table .data-table th:nth-child(5) { width: 8%; }
.resource-table .data-table th:nth-child(6) { width: 9%; }
.resource-table .data-table th:nth-child(7) { width: 14%; }
.resource-table .data-table th:nth-child(8) { width: 11%; }
.resource-table .data-table td { overflow: hidden; }
.resource-name, .resource-node { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.resource-claims { display: grid; gap: 4px; }
.resource-claim { display: inline-flex; max-width: 100%; align-items: center; gap: 6px; overflow: hidden; text-align: left; }
.resource-claim .badge { flex: none; text-decoration: none; }
.resource-detail-button { border: 0; padding: 2px 0; background: none; color: var(--action-primary); font: inherit; cursor: pointer; white-space: nowrap; }
.resource-detail-button:hover { text-decoration: underline; }
.resource-table .badge { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.resource-modal-content { display: grid; gap: 16px; font-size: 12px; }
.resource-modal-row { display: flex; justify-content: space-between; gap: 16px; }
.resource-modal-row > span, .resource-modal-block > span { color: var(--text-secondary); }
.resource-modal-value { max-width: 75%; overflow-wrap: anywhere; text-align: right; }
.resource-modal-block p { margin: 8px 0 0; overflow-wrap: anywhere; line-height: 1.5; }
.resource-image-list { display: grid; gap: 8px; margin: 8px 0 0; padding: 0; list-style: none; }
.resource-image-list li { border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 8px 10px; background: var(--surface-subtle); overflow-wrap: anywhere; }
.resource-container-usage { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 4px 12px; }
.preflight-heading { display: flex; align-items: center; justify-content: space-between; }
.task-table { padding: 0 var(--space-20) var(--space-16); overflow-x: auto; }
.task-pagination { justify-content: flex-end; margin-top: 0; padding: 0 var(--space-20) var(--space-16); color: var(--text-secondary); font-size: 12px; }
.shutdown-progress { display: flex; flex-wrap: wrap; gap: 4px 18px; padding: 0 var(--space-20) var(--space-12); }
.task-table .data-table { min-width: 560px; }
.deploy-form { display: grid; gap: var(--space-16); }
.deploy-form p { margin: 0; line-height: 1.6; }
.deploy-form .table-wrap { overflow-x: auto; max-height: min(40dvh, 320px); }
.deploy-form .data-table { min-width: 550px; }
.deploy-form input[type=checkbox] { accent-color: var(--action-primary); }
.mysql-config-editor { display: block; width: 100%; min-height: 220px; resize: vertical; line-height: 1.5; }
.resource-pool-table { max-height: min(48dvh, 400px); overflow: auto; }
.resource-pool-table .data-table { min-width: 0; table-layout: fixed; }
.resource-pool-table .data-table th:first-child { width: 48px; }
.resource-pool-table .data-table th:last-child { width: 120px; }
.resource-pool-node { overflow-wrap: anywhere; }
.resource-pool-node strong, .resource-pool-node span { display: block; }
@media (max-width: 640px) { .resource-pool-table .data-table th:first-child { width: 36px; }.resource-pool-table .data-table th:last-child { width: 86px; }.resource-pool-table .data-table td { padding-right: 4px; padding-left: 4px; }.resource-pool-table .badge { white-space: normal; } }
.deployment-context { display: flex; flex-wrap: wrap; gap: 4px 16px; color: var(--text-secondary); font-size: 12px; }
.form-group { margin: 0; }
.node-picker-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.node-picker-heading strong { font-size: 13px; }
.deployment-log { min-height: 170px; max-height: min(58dvh, 580px); overflow: auto; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 12px; background: var(--surface-subtle); color: var(--text-secondary); font: 11px/1.6 var(--font-mono); white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 640px) { .detail-toolbar, .mysql-topology { padding-right: 14px; padding-left: 14px; }.mysql-topology { padding-top: 24px; }.topology-node { width: 100%; }.task-table, .resource-table, .shutdown-progress { padding-right: 14px; padding-left: 14px; } }
@media (max-width: 640px) { .connection-row { grid-template-columns: minmax(0, 1fr); gap: 3px; } }
</style>
