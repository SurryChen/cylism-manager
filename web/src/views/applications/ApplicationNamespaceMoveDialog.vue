<template>
  <BaseModal teleport open title="迁移运行环境" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
    <EmptyState v-if="loading" variant="loading" message="正在读取运行环境..." />
    <EmptyState v-else-if="!options?.release" message="应用暂无可迁移的成功发布" />
    <EmptyState v-else-if="!options.targets.length" message="所属项目暂无同集群的其他运行环境" />
    <template v-else>
      <p class="form-hint">源命名空间 {{ options.source.namespace }} · 当前部署 {{ options.release.version || `#${options.release.sequence}` }}</p>
      <div class="form-group">
        <label class="form-label">目标环境 · {{ options.project_name }}</label>
        <SelectMenu v-model="targetID" aria-label="目标环境" :disabled="busy">
          <option :value="0" disabled>选择目标环境</option>
          <option v-for="target in options.targets" :key="target.id" :value="target.id">{{ target.name }} · {{ target.namespace }}</option>
        </SelectMenu>
      </div>
      <p v-if="targetID" class="form-hint">请先在目标命名空间准备应用引用的 ConfigMap、Secret 和镜像拉取凭据。</p>
      <label v-if="targetID && !status?.prepared" class="check-row">
        <input v-model="parallelConfirmed" type="checkbox" :disabled="busy" />已确认新旧实例可以并行，包括数据库、会话和后台任务
      </label>
      <p v-if="status?.prepared" class="form-hint">{{ status.ready ? '目标 Deployment 和 Service 已就绪。' : '目标副本已创建，等待 Deployment 和 Service 就绪。' }}</p>
      <template v-if="status?.ready">
        <p class="form-hint">先验证目标读写并切换入口，再确认接管。接管后自动发布会使用新环境，源工作负载继续保留供回退。</p>
        <label class="check-row">
          <input v-model="cutoverConfirmed" type="checkbox" :disabled="busy" />已验证目标应用、完成入口切换并检查实际访问
        </label>
      </template>
    </template>
    <template #actions>
      <button class="btn" :disabled="busy" @click="$emit('close')">关闭</button>
      <template v-if="targetID">
        <button class="btn" :disabled="busy" @click="run('check')">刷新状态</button>
        <button v-if="status?.ready" class="btn btn-primary" :disabled="busy || !cutoverConfirmed" @click="run('adopt')">确认接管</button>
        <button v-else-if="!status?.prepared" class="btn btn-primary" :disabled="busy || !parallelConfirmed" @click="run('prepare')">准备目标副本</button>
      </template>
    </template>
  </BaseModal>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
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

onMounted(async () => {
  try {
    options.value = await getApplicationNamespaceMoveOptions(props.applicationId)
  } catch (error) {
    emit('close')
    emit('error', error.message || '读取迁移选项失败')
  } finally {
    loading.value = false
  }
})

watch(targetID, id => {
  status.value = null
  parallelConfirmed.value = false
  cutoverConfirmed.value = false
  if (id) void run('check')
})

async function run(action) {
  if (!targetID.value || busy.value) return
  busy.value = true
  try {
    const result = await moveApplicationNamespace(props.applicationId, {
      source_environment_id: options.value.source.id,
      target_environment_id: targetID.value,
      release_id: options.value.release.id,
      action,
      confirmed: action === 'prepare' ? parallelConfirmed.value : cutoverConfirmed.value,
    })
    if (result.adopted) emit('adopted', result)
    else status.value = result
  } catch (error) {
    emit('error', error.message || '运行环境迁移失败')
  } finally {
    busy.value = false
  }
}
</script>
