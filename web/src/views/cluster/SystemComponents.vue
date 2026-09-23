<template>
  <div>
    <TabbedWorkspaceCard v-if="loaded" class="system-component-workspace">
      <template #actions>
        <button class="icon-button" type="button" title="刷新系统组件" aria-label="刷新系统组件" :disabled="loading" @click="load">
          <RefreshCw :size="16" :class="{ 'is-spinning': loading }" />
        </button>
      </template>
      <div class="table-wrap">
        <table class="data-table system-component-table">
          <thead>
            <tr><th>组件</th><th>命名空间</th><th>控制方式</th><th>运行状态</th><th>配置状态</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.chart_name">
              <td class="cell-primary">{{ item.chart_name }}</td>
              <td>{{ item.namespace }}</td>
              <td>
                <span class="mode-label" :class="`mode-${item.controller_mode || 'unknown'}`" :title="controllerModeDetail(item)">{{ controllerModeText(item.controller_mode) }}</span>
              </td>
              <td>
                <template v-if="item.deployment">
                  <span class="badge" :class="deploymentReady(item) ? 'badge-online' : 'badge-offline'" :title="runtimeDetail(item)">
                    {{ item.deployment.ready_replicas }}/{{ item.deployment.replicas }} 就绪
                  </span>
                </template>
                <template v-else-if="item.workload">
                  <span class="badge" :class="workloadReady(item) ? 'badge-online' : 'badge-offline'" :title="runtimeDetail(item)">
                    {{ item.workload.ready }}/{{ item.workload.desired }} 就绪
                  </span>
                </template>
                <template v-else-if="item.controller_mode === 'helm_chart'">
                  <span class="badge" :class="item.chart_failed ? 'badge-danger' : item.chart_ready ? 'badge-online' : 'badge-warn'" :title="runtimeDetail(item)">
                    {{ item.chart_failed ? '安装失败' : item.chart_ready ? '已安装' : '安装中' }}
                  </span>
                </template>
                <template v-else>
                  <span class="badge" :class="isEmbedded(item) ? 'badge-online' : isMissing(item) ? 'badge-offline' : 'badge-danger'" :title="runtimeDetail(item)">
                    {{ isEmbedded(item) ? '运行中' : isMissing(item) ? '未安装' : '待确认' }}
                  </span>
                </template>
              </td>
              <td>
                <span class="badge" :class="configBadgeClass(item)" :title="configurationDetail(item)">{{ configBadgeText(item) }}</span>
              </td>
              <td>
                <div class="btn-group">
                  <button class="btn btn-sm" :disabled="!canConfigure(item)" @click="edit(item)">配置</button>
                  <button class="btn btn-sm btn-danger" :disabled="!item.has_config || !canRestore(item)" @click="revert(item)">恢复默认</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </TabbedWorkspaceCard>

    <ErrorNoticeModal :open="Boolean(pageError)" :title="pageErrorTitle" :message="pageError" @close="dismissPageError" />

    <div v-if="modal" class="overlay" @click.self="close">
      <div class="modal">
        <h2 class="modal-title">配置 {{ editing?.chart_name }}</h2>
        <p class="modal-copy">{{ modalDescription(editing) }}</p>
        <form @submit.prevent="save">
          <p v-if="componentFormError" class="k8s-banner k8s-banner-warn section-gap">{{ componentFormError }}</p>
          <div class="config-section-title">运行容量</div>
          <div class="form-row">
            <div v-if="canScale(editing) || !isStatic(editing)" class="form-group">
              <label class="form-label">副本数</label>
              <input v-model.number="form.replicas" type="number" min="1" class="form-input" placeholder="默认 1" />
            </div>
            <div class="form-group">
              <label class="form-label">更新时最大不可用</label>
              <SelectMenu v-model="form.maxUnavailable" class="form-select">
                <option value="0">0（保持服务）</option>
                <option value="1">1（允许短暂减少）</option>
              </SelectMenu>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">更新时最大额外副本</label>
            <SelectMenu v-model="form.maxSurge" class="form-select">
              <option value="1">1（先启动新副本）</option>
              <option value="0">0（不额外扩容）</option>
              <option value="25%">25%（Kubernetes 默认）</option>
            </SelectMenu>
          </div>
          <div v-if="isTraefik(editing)" class="form-group">
            <label class="form-label">入口请求读取超时</label>
            <SelectMenu v-model="form.traefikReadTimeoutMode" class="form-select" data-testid="traefik-read-timeout" @change="selectTraefikReadTimeout">
              <option value="">使用 Traefik 默认值（60 秒）</option>
              <option value="5m">5 分钟</option>
              <option value="30m">30 分钟（推荐）</option>
              <option value="1h">1 小时</option>
              <option value="custom">自定义</option>
            </SelectMenu>
            <input v-if="form.traefikReadTimeoutMode === 'custom'" v-model.trim="form.traefikReadTimeout" class="form-input timeout-input" required placeholder="例如 15m" data-testid="traefik-custom-read-timeout" />
          </div>
          <div v-if="canPlace(editing)" class="form-group">
            <div class="config-section-title">节点调度</div>
            <label class="form-label">部署节点</label>
            <SelectMenu v-model="form.nodeName" class="form-select" data-testid="coredns-node-selector">
              <option value="">不固定，由 Kubernetes 调度</option>
              <option v-for="node in schedulableNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option>
            </SelectMenu>
            <span class="form-hint">固定后所有副本都会调度到该节点，节点故障时可能影响服务。</span>
          </div>
          <p v-if="isStatic(editing)" class="baseline-hint">
            {{ availabilityHint(editing) }}
          </p>
          <div class="modal-actions">
            <button type="button" class="btn" @click="close">取消</button>
            <button v-if="canApplyBaseline(editing)" type="button" class="btn" @click="applyBaseline">高可用滚动基线</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存配置' }}</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getClusterNodes, getSystemComponents, revertSystemComponent, updateSystemComponent } from '../../api/system-components.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const items = ref([])
const loaded = ref(false)
const loading = ref(false)
const modal = ref(false)
const editing = ref(null)
const saving = ref(false)
const error = ref('')
const componentFormError = ref('')
const componentActionError = ref('')
const pageError = computed(() => componentActionError.value || error.value)
const pageErrorTitle = computed(() => componentActionError.value ? '组件操作失败' : '加载系统组件失败')
const form = ref(blankForm())
const nodes = ref([])
const schedulableNodes = computed(() => nodes.value.filter(node => node.ready && !node.evicted))
const componentResource = useAsyncResource(async ({ signal }) => {
  const [items, nodeList] = await Promise.all([getSystemComponents({ signal }), getClusterNodes({ signal })])
  return { items, nodeList }
}, null)

function blankForm() {
  return { replicas: null, maxUnavailable: '0', maxSurge: '1', nodeName: '', traefikReadTimeout: '', traefikReadTimeoutMode: '' }
}

function dismissPageError() {
  if (componentActionError.value) componentActionError.value = ''
  else error.value = ''
}

async function load() {
  loading.value = true
  error.value = ''
  const result = await componentResource.refresh()
  if (result) { items.value = result.items || []; nodes.value = result.nodeList || [] }
  else if (componentResource.error.value) error.value = componentResource.error.value.message || '加载系统组件失败'
  loading.value = false
  loaded.value = true
}

function configBadgeClass(item) {
  if (!item.has_config) return 'badge-offline'
  if (item.apply_status === 'failed') return 'badge-danger'
  if (item.apply_status === 'succeeded' && item.effective === false) return 'badge-danger'
  return 'badge-online'
}

function configBadgeText(item) {
  if (item.controller_mode === 'unknown') return '控制源未识别'
  if (item.controller_mode === 'embedded') return '内置管理'
  if (!item.has_config) return '默认配置'
  if (item.apply_status === 'failed') return '失败'
  if (item.apply_status === 'succeeded' && item.effective === false) return '已保存未生效'
  return '已应用'
}

function configNeedsAttention(item) {
  return item.apply_status === 'failed' || item.effective === false
}

function isTraefik(item) {
  return item?.chart_name === 'traefik'
}

function traefikTimeoutStatus(item) {
  const state = item?.traefik
  if (!state?.read_timeout) return '读取超时使用默认 60 秒'
  return state.read_timeout_effective ? `读取超时 ${state.effective_read_timeout}，已生效` : `读取超时 ${state.read_timeout}，等待生效`
}

function isTraefikTimeoutPreset(value) {
  return ['', '5m', '30m', '1h'].includes(value)
}

function selectTraefikReadTimeout() {
  if (form.value.traefikReadTimeoutMode !== 'custom') {
    form.value.traefikReadTimeout = form.value.traefikReadTimeoutMode
  } else if (isTraefikTimeoutPreset(form.value.traefikReadTimeout)) {
    form.value.traefikReadTimeout = ''
  }
}

function configIssueText(item) {
  if (item.effective === false) return '配置与实际状态不一致，请重新保存'
  return '配置未应用，请打开配置重试'
}

function deploymentReady(item) {
  return Number(item?.deployment?.ready_replicas) >= Number(item?.deployment?.replicas) && Number(item?.deployment?.replicas) > 0
}

function workloadReady(item) {
  return Number(item?.workload?.ready) >= Number(item?.workload?.desired) && Number(item?.workload?.desired) > 0
}

function isMissing(item) {
  return /not found/i.test(item.deployment_error || '')
}

function isEmbedded(item) {
  return item?.controller_mode === 'embedded'
}

function isStatic(item) {
  return item?.controller_mode === 'static_deployment'
}

function canConfigure(item) {
  return Boolean(item?.capabilities?.configure)
}

function canPlace(item) {
  return Boolean(item?.capabilities?.node_placement)
}

function canScale(item) {
  return Boolean(item?.capabilities?.replica_scaling)
}

function canApplyBaseline(item) {
  return Boolean(item?.capabilities?.safe_baseline)
}

function canRestore(item) {
  return Boolean(item?.capabilities?.restore)
}

function controllerModeText(mode) {
  const labels = {
    helm_chart: 'Helm 管理',
    static_deployment: 'K3s 静态组件',
    embedded: 'K3s 内置',
    unknown: '待确认',
  }
  return labels[mode] || labels.unknown
}

function controllerModeDetail(item) {
  if (isEmbedded(item)) return '由 K3s 内置控制器提供'
  if (isStatic(item)) return '由 K3s 静态清单控制'
  if (item?.controller_mode === 'helm_chart') return '由 Helm 控制器管理'
  return '未识别组件控制来源'
}

function runtimeDetail(item) {
  if (item?.deployment?.fixed_node) return `固定节点：${item.deployment.fixed_node}`
  if (item?.deployment && isStatic(item)) return '由 Kubernetes 自动调度'
  if (item?.workload) return `${item.workload.kind}：${item.workload.name}`
  if (isEmbedded(item)) return '由 K3s 内置控制器提供'
  return ''
}

function configurationDetail(item) {
  const details = []
  if (isTraefik(item)) details.push(traefikTimeoutStatus(item))
  if (configNeedsAttention(item)) details.push(configIssueText(item))
  if (item?.availability?.description) details.push(item.availability.description)
  if (item?.last_applied_at) details.push(`应用于 ${formatTime(item.last_applied_at)}`)
  return details.join('\n')
}

function modalDescription(item) {
  if (isStatic(item)) return canScale(item) ? '设置高可用副本、滚动更新和节点调度。平台会在 K3s 重启后自动恢复这些设置。' : '该组件副本策略由 K3s 或组件 profile 管理；平台仅允许配置受支持的滚动更新字段。'
  return '设置会写入 HelmChartConfig，并由 Helm 控制器负责生效。'
}

function availabilityHint(item) {
  return item?.availability?.description || '未定义高可用 profile，副本策略由组件自身管理。'
}

function formatTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function parseValues(content, item) {
  const rollingUpdate = item?.deployment?.strategy?.rollingUpdate
  const parsed = {
    ...blankForm(),
    replicas: Number(item?.deployment?.replicas) || null,
    maxUnavailable: String(rollingUpdate?.maxUnavailable ?? '0'),
    maxSurge: String(rollingUpdate?.maxSurge ?? '1'),
    nodeName: item?.deployment?.fixed_node || '',
  }
  for (const line of String(content || '').split('\n')) {
    const hostname = line.match(/^\s*kubernetes\.io\/hostname\s*:\s*(.+?)\s*$/)
    if (hostname) {
      parsed.nodeName = hostname[1]
      continue
    }
    const match = line.match(/^\s*([A-Za-z][A-Za-z0-9]*)\s*:\s*(.+?)\s*$/)
    if (!match) continue
    const key = match[1]
    const value = match[2]
    if (key === 'replicas') parsed.replicas = Number(value)
    else if (key === 'maxUnavailable') parsed.maxUnavailable = String(value)
    else if (key === 'maxSurge') parsed.maxSurge = String(value)
  }
  const webTimeout = String(content || '').match(/--entryPoints\.web\.transport\.respondingTimeouts\.readTimeout=([^'"\s]+)/)?.[1] || ''
  const webSecureTimeout = String(content || '').match(/--entryPoints\.websecure\.transport\.respondingTimeouts\.readTimeout=([^'"\s]+)/)?.[1] || ''
  if (webTimeout && webTimeout === webSecureTimeout) {
    parsed.traefikReadTimeout = webTimeout
    parsed.traefikReadTimeoutMode = isTraefikTimeoutPreset(webTimeout) ? webTimeout : 'custom'
  }
  return parsed
}

function renderValues(form) {
  if (isStatic(editing.value)) {
    const lines = []
    if (form.replicas && form.replicas > 0) lines.push(`replicas: ${form.replicas}`)
    lines.push('deploymentStrategy:')
    lines.push('  type: RollingUpdate')
    lines.push('  rollingUpdate:')
    lines.push(`    maxUnavailable: ${form.maxUnavailable}`)
    lines.push(`    maxSurge: ${form.maxSurge}`)
    if (canPlace(editing.value) && form.nodeName) {
      lines.push('nodeSelector:')
      lines.push(`  kubernetes.io/hostname: ${form.nodeName}`)
    } else if (canPlace(editing.value)) {
      lines.push('nodeSelector: {}')
    }
    return lines.join('\n') + '\n'
  }
  const lines = []
  if (form.replicas && form.replicas > 0) lines.push(`replicas: ${form.replicas}`)
  if (form.maxUnavailable !== '') lines.push(`maxUnavailable: ${form.maxUnavailable}`)
  if (form.maxSurge !== '') lines.push(`maxSurge: ${form.maxSurge}`)
  return lines.join('\n') + '\n'
}

function edit(item) {
  editing.value = item
  componentFormError.value = ''
  form.value = parseValues(item.values_content, item)
  modal.value = true
}

function applyBaseline() {
  form.value.maxUnavailable = '0'
  form.value.maxSurge = '1'
  if (canApplyBaseline(editing.value)) form.value.replicas = 2
}

function close() {
  modal.value = false
  editing.value = null
  componentFormError.value = ''
}

async function save() {
  if (!editing.value) return
  saving.value = true
  componentFormError.value = ''
  try {
    const payload = { values_content: renderValues(form.value) }
    if (isTraefik(editing.value)) payload.traefik_read_timeout = form.value.traefikReadTimeout
    await updateSystemComponent(editing.value.chart_name, payload)
    close()
    await load()
  } catch (err) {
    componentFormError.value = err.message || '保存系统组件配置失败'
  } finally {
    saving.value = false
  }
}

async function revert(item) {
  if (!window.confirm(`恢复 ${item.chart_name} 为默认配置？${isStatic(item) ? '将重置平台受控的 Deployment 字段。' : '将删除 HelmChartConfig。'}`)) return
  componentActionError.value = ''
  try {
    await revertSystemComponent(item.chart_name)
    await load()
  } catch (err) {
    componentActionError.value = err.message || '恢复默认配置失败'
  }
}

onMounted(load)
</script>

<style scoped>
.system-component-table th, .system-component-table td { vertical-align: middle; }
.system-component-table th:nth-child(1) { width: 16%; }
.system-component-table th:nth-child(2) { width: 14%; }
.system-component-table th:nth-child(3) { width: 16%; }
.system-component-table th:nth-child(4) { width: 16%; }
.system-component-table th:nth-child(5) { width: 18%; }
.system-component-table th:nth-child(6) { width: 20%; }
.mode-label { display: inline-flex; align-items: center; padding: 4px 7px; border-radius: 5px; background: var(--surface-subtle); color: var(--text-secondary); font: 10px/1 var(--font-mono); white-space: nowrap; }
.mode-static_deployment { color: var(--text-primary); background: color-mix(in srgb, var(--action-primary) 12%, var(--surface-subtle)); }
.mode-helm_chart { color: var(--text-primary); background: color-mix(in srgb, var(--focus) 12%, var(--surface-subtle)); }
.mode-embedded { color: var(--text-primary); background: color-mix(in srgb, var(--action-primary) 12%, var(--surface-subtle)); }
.mode-unknown { color: var(--warning); background: color-mix(in srgb, var(--warning) 14%, var(--surface-subtle)); }
.config-section-title { margin: 20px 0 10px; color: var(--text-primary); font-size: 13px; font-weight: 600; }
.config-section-title:first-child { margin-top: 0; }
.baseline-hint { margin: 12px 0 0; padding: 9px 11px; border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.timeout-input { margin-top: var(--space-8); }
@media (max-width: 640px) {
  .system-component-table th:nth-child(2), .system-component-table td:nth-child(2), .system-component-table th:nth-child(3), .system-component-table td:nth-child(3) { display: none; }
}
</style>
