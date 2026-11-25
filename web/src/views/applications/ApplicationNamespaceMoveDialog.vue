<template>
  <BaseModal teleport open title="迁移运行环境" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
    <EmptyState v-if="loading" variant="loading" message="正在读取运行环境..." />
    <EmptyState v-else-if="!options?.release" message="应用暂无可迁移的成功发布" />
    <EmptyState v-else-if="!options.targets.length" message="所属项目暂无同集群的其他运行环境" />
    <template v-else>
      <p class="form-hint">源命名空间 {{ options.source.namespace }} · 当前部署 {{ options.release.version || `#${options.release.sequence}` }}</p>
      <div class="form-group">
        <label class="form-label">目标环境 · {{ options.project_name }}</label>
        <SelectMenu v-model="targetID" aria-label="目标环境" :disabled="busy || !!status?.phase">
          <option :value="0" disabled>选择目标环境</option>
          <option v-for="target in options.targets" :key="target.id" :value="target.id">{{ target.name }} · {{ target.namespace }}</option>
        </SelectMenu>
      </div>
      <template v-if="status?.maintenance">
        <p class="form-hint" role="status">{{ phaseMessage }}</p>
        <p v-if="status.detail" class="form-hint">{{ status.detail }}</p>
        <p v-if="status.node" class="form-hint">运行节点 {{ status.node }}</p>
        <p v-if="status.claims?.length" class="form-hint">本地卷 {{ status.claims.join('、') }}</p>
        <p v-if="status.config_maps?.length" class="form-hint">ConfigMap {{ status.config_maps.join('、') }}</p>
        <p v-if="status.secrets?.length" class="form-hint">Secret {{ status.secrets.join('、') }}</p>
        <label v-if="!status.phase || status.phase === 'preparing'" class="check-row">
          <input v-model="maintenanceConfirmed" type="checkbox" :disabled="busy || status.running" />确认将列出的配置和 Secret 复制到目标命名空间
        </label>
        <label v-if="status.phase === 'prepared'" class="check-row">
          <input v-model="maintenanceConfirmed" type="checkbox" :disabled="busy || status.running" />确认停止源实例并开始维护切换，现有连接可能中断
        </label>
        <template v-if="canSwitch">
          <p class="form-hint">{{ status.manual_validation ? '应用没有就绪探针。请通过目标环境的 Service 验证关键读写后再切换入口。' : '请通过目标环境的 Service 验证关键读写后再切换入口。' }}</p>
          <label class="check-row">
            <input v-model="maintenanceConfirmed" type="checkbox" :disabled="busy || status.running" />已验证目标应用，确认切换入口
          </label>
        </template>
        <label v-if="status.phase === 'routed'" class="check-row">
          <input v-model="cutoverConfirmed" type="checkbox" :disabled="busy || status.running" />已验证实际访问，确认将运行环境和自动发布接管到目标项目
        </label>
      </template>
      <template v-else-if="targetID && status">
        <p class="form-hint">请先在目标命名空间准备应用引用的 ConfigMap、Secret 和镜像拉取凭据。</p>
        <label v-if="!status.prepared" class="check-row">
          <input v-model="parallelConfirmed" type="checkbox" :disabled="busy" />已确认新旧实例可以并行，包括数据库、会话和后台任务
        </label>
        <p v-if="status.prepared" class="form-hint">{{ status.ready ? '目标 Deployment 和 Service 已就绪。' : '目标副本已创建，等待 Deployment 和 Service 就绪。' }}</p>
        <template v-if="status.ready">
          <p class="form-hint">先验证目标读写并切换入口，再确认接管。接管后自动发布会使用新环境，源工作负载继续保留供回退。</p>
          <label class="check-row">
            <input v-model="cutoverConfirmed" type="checkbox" :disabled="busy" />已验证目标应用、完成入口切换并检查实际访问
          </label>
        </template>
        <button v-if="!status.prepared" class="btn btn-sm" :disabled="busy" @click="selectMaintenance">改用维护窗口</button>
      </template>
    </template>
    <template #actions>
      <button class="btn" :disabled="busy" @click="$emit('close')">关闭</button>
      <template v-if="targetID">
        <button class="btn" :disabled="busy" @click="run('check')">刷新状态</button>
        <template v-if="status?.maintenance && !status.running">
          <button v-if="['preparing', 'prepared', 'canceling'].includes(status.phase)" class="btn" :disabled="busy" @click="run('cancel')">取消迁移</button>
          <button v-if="['stopping', 'restoring'].includes(status.phase) || (status.phase === 'starting' && !status.claims?.length)" class="btn" :disabled="busy" @click="run('restore')">恢复源实例</button>
          <button v-if="!status.phase || status.phase === 'preparing'" class="btn btn-primary" :disabled="busy || !maintenanceConfirmed" @click="run('prepare_maintenance')">预配目标环境</button>
          <button v-else-if="status.phase === 'prepared'" class="btn btn-primary" :disabled="busy || !maintenanceConfirmed" @click="run('start')">开始维护切换</button>
          <button v-else-if="canSwitch" class="btn btn-primary" :disabled="busy || !maintenanceConfirmed" @click="run('switch')">切换入口</button>
          <button v-else-if="['stopping', 'starting'].includes(status.phase)" class="btn btn-primary" :disabled="busy" @click="run('start')">继续启动目标</button>
          <button v-else-if="status.phase === 'routed'" class="btn btn-primary" :disabled="busy || !status.ready || !cutoverConfirmed" @click="run('adopt')">确认接管</button>
        </template>
        <template v-else-if="status && !status.maintenance">
          <button v-if="status.ready" class="btn btn-primary" :disabled="busy || !cutoverConfirmed" @click="run('adopt')">确认接管</button>
          <button v-else-if="!status.prepared" class="btn btn-primary" :disabled="busy || !parallelConfirmed" @click="run('prepare')">准备目标副本</button>
        </template>
      </template>
    </template>
  </BaseModal>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { getApplicationNamespaceMoveOptions, moveApplicationNamespace } from '../../api/applications.js'

const props = defineProps({ applicationId: { type: String, required: true } })
const emit = defineEmits(['close', 'error', 'adopted'])
const options = ref(null)
const status = ref(null)
const targetID = ref(0)
const loading = ref(true)
const busy = ref(false)
const parallelConfirmed = ref(false)
const cutoverConfirmed = ref(false)
const maintenanceConfirmed = ref(false)
const maintenance = ref(false)
let poll
let closed = false

const canSwitch = computed(() => status.value?.ready && ['starting', 'switching'].includes(status.value.phase))
const phaseMessage = computed(() => ({
  preparing: status.value.running ? '正在准备目标环境，源服务继续运行。' : '目标环境预配未完成，可以重试或取消。',
  prepared: '目标环境已预配。维护期间源服务会暂停，本地卷校验完成后启动目标实例。',
  stopping: '正在等待源实例退出并同步本地数据。',
  starting: status.value.ready ? '目标实例已就绪，等待业务验证。' : '等待目标实例就绪。目标可能已经写入数据，源实例保持停止。',
  switching: '入口切换进行中。',
  routed: '入口已切换。请验证实际访问后接管运行环境。',
  restoring: '正在恢复源实例，可以刷新状态。',
  canceling: '正在取消迁移并清理预配资源。',
}[status.value.phase] || '此迁移需要维护窗口。先预配目标环境，源服务继续运行。'))

onMounted(async () => {
  try {
    options.value = await getApplicationNamespaceMoveOptions(props.applicationId)
    if (options.value.maintenance) targetID.value = options.value.maintenance.target_environment_id
  } catch (error) {
    emit('close')
    emit('error', error.message || '读取迁移选项失败')
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  closed = true
  clearTimeout(poll)
})

watch(targetID, id => {
  status.value = null
  maintenance.value = false
  parallelConfirmed.value = false
  cutoverConfirmed.value = false
  maintenanceConfirmed.value = false
  if (id) void run('check')
})

function selectMaintenance() {
  maintenance.value = true
  status.value = null
  void run('check')
}

async function run(action, automatic = false) {
  if (!targetID.value || busy.value) return
  clearTimeout(poll)
  busy.value = true
  try {
    const result = await moveApplicationNamespace(props.applicationId, {
      source_environment_id: options.value.source.id,
      target_environment_id: targetID.value,
      release_id: options.value.release.id,
      action,
      maintenance: maintenance.value || !!status.value?.maintenance,
      confirmed: { prepare: parallelConfirmed.value, prepare_maintenance: maintenanceConfirmed.value, adopt: cutoverConfirmed.value,
        switch: maintenanceConfirmed.value, start: maintenanceConfirmed.value || ['stopping', 'starting'].includes(status.value?.phase), restore: true, cancel: true }[action] ?? false,
    })
    if (closed) return
    if (result.adopted) {
      emit('adopted', result)
      return
    }
    if (status.value?.phase !== result.phase || action !== 'check') maintenanceConfirmed.value = false
    status.value = result
    if (result.running || (result.phase === 'starting' && !result.ready && !result.detail)) {
      poll = setTimeout(() => void run('check', true), 3000)
    }
  } catch (error) {
    if (!automatic) emit('error', error.message || '运行环境迁移失败')
  } finally {
    busy.value = false
  }
}
</script>
