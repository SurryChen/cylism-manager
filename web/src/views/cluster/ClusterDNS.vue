<template>
  <div>
    <TabbedWorkspaceCard class="dns-workspace dns-runtime-workspace">
      <template #actions>
        <button class="btn btn-primary" data-testid="open-dns-config" :disabled="!loaded" @click="openConfig">配置 DNS 上游</button>
        <button class="icon-button" type="button" title="刷新 CoreDNS 状态" aria-label="刷新 CoreDNS 状态" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button>
      </template>
      <template v-if="loaded">
        <div v-if="data.pods?.length" class="table-wrap">
          <table class="data-table dns-pod-table">
            <thead><tr><th>Pod</th><th>节点</th><th>Pod IP</th><th>状态</th></tr></thead>
            <tbody><tr v-for="pod in data.pods" :key="pod.name"><td class="cell-primary">{{ pod.name }}</td><td>{{ pod.node || '-' }}</td><td>{{ pod.ip || '-' }}</td><td><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-danger'">{{ pod.ready ? '就绪' : '未就绪' }}</span></td></tr></tbody>
          </table>
        </div>
        <EmptyState v-else message="未发现 CoreDNS Pod" />
      </template>
      <EmptyState v-else variant="loading" message="正在读取 CoreDNS 状态" />
    </TabbedWorkspaceCard>

    <TabbedWorkspaceCard v-if="loaded && data.history?.length" class="dns-workspace dns-history-workspace">
      <div class="table-wrap">
        <table class="data-table dns-history-table">
          <thead><tr><th>版本</th><th>DNS 上游</th><th>创建时间</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="policy in data.history" :key="policy.revision">
              <td class="cell-primary">版本 {{ policy.revision }}</td>
              <td><span class="dns-upstream-value" :title="policy.resolvers?.join('，') || '继承宿主机 DNS'">{{ policy.resolvers?.join('，') || '继承宿主机 DNS' }}</span></td>
              <td>{{ formatTime(policy.created_at) }}</td>
              <td><span class="badge" :class="policy.revision === data.active_policy?.revision ? 'badge-online' : 'badge-offline'">{{ policy.revision === data.active_policy?.revision ? '当前' : '历史' }}</span></td>
              <td><button class="btn btn-sm" :disabled="saving || policy.revision === data.active_policy?.revision" @click="rollback(policy)">{{ policy.revision === data.active_policy?.revision ? '当前版本' : '回滚' }}</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </TabbedWorkspaceCard>

    <ErrorNoticeModal :open="Boolean(pageError)" :message="pageError" @close="dismissPageError" />

    <BaseModal :open="configuring" title="配置 DNS 上游" size="medium" @close="closeConfig">
      <form id="dns-config-form" class="dns-form" @submit.prevent="openConfirm">
        <div v-for="(_, index) in resolvers" :key="index" class="resolver-row">
          <label class="resolver-label" :for="`dns-resolver-${index}`">DNS 上游 {{ index + 1 }}</label>
          <input :id="`dns-resolver-${index}`" v-model.trim="resolvers[index]" class="form-input" inputmode="decimal" placeholder="例如 223.5.5.5" :aria-label="`DNS 上游 ${index + 1}`" />
          <button v-if="resolvers.length > 1" type="button" class="icon-btn" :aria-label="`移除 DNS 上游 ${index + 1}`" @click="resolvers.splice(index, 1)">×</button>
        </div>
        <button v-if="resolvers.length < 3" type="button" class="btn btn-sm add-resolver-button" @click="resolvers.push('')">添加备用上游</button>
        <p class="dns-form-status"><span class="badge" :class="inheritedDNS ? 'badge-offline' : 'badge-online'">{{ inheritedDNS ? '继承宿主机 DNS' : '平台 DNS 策略' }}</span><span>仅支持 IP 地址；保存时仅替换 CoreDNS 根域的转发规则。</span></p>
        <FeedbackBanner v-if="configError" tone="warning" :message="configError" />
      </form>
      <template #actions>
        <button class="btn" :disabled="saving || inheritedDNS" @click="openResetConfirm">恢复宿主机 DNS</button>
        <button class="btn" @click="closeConfig">取消</button>
        <button class="btn btn-primary" type="submit" form="dns-config-form" data-testid="save-dns-config" :disabled="saving">验证并应用</button>
      </template>
    </BaseModal>

    <BaseModal :open="confirming" title="应用集群 DNS 策略" size="small" @close="confirming = false">
      <p class="confirm-copy">CoreDNS 将改用：{{ normalizedResolvers.join('，') }}。此操作影响所有通过集群 DNS 解析的工作负载。</p>
      <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
      <template #actions>
        <button class="btn" @click="confirming = false">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="apply">确认应用</button>
      </template>
    </BaseModal>
    <BaseModal :open="resetConfirming" title="恢复宿主机 DNS" size="small" @close="resetConfirming = false">
      <p class="confirm-copy">CoreDNS 将恢复为 <code>/etc/resolv.conf</code>，由各节点宿主机决定外部 DNS。此操作影响所有通过集群 DNS 解析的工作负载。</p>
      <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
      <template #actions>
        <button class="btn" @click="resetConfirming = false">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="reset">确认恢复</button>
      </template>
    </BaseModal>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { deleteClusterDNS, getClusterDNS, rollbackClusterDNS, updateClusterDNS } from '../../api/cluster-dns.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const data = ref({ forwarding: [], pods: [], history: [], active_policy: null })
const resolvers = ref([''])
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const error = ref('')
const actionError = ref('')
const configError = ref('')
const configuring = ref(false)
const confirming = ref(false)
const resetConfirming = ref(false)
const dnsResource = useAsyncResource(({ signal }) => getClusterDNS({ signal }), null)
const pageError = computed(() => error.value || ((!confirming.value && !resetConfirming.value) ? actionError.value : ''))

function dismissPageError() {
  if (error.value) error.value = ''
  else actionError.value = ''
}

const normalizedResolvers = computed(() => resolvers.value.map(value => value.trim()).filter(Boolean))
const inheritedDNS = computed(() => !(data.value.active_policy?.resolvers?.length) && data.value.forwarding?.length === 1 && data.value.forwarding[0] === '/etc/resolv.conf')

async function load() {
  loading.value = true
  error.value = ''
  const result = await dnsResource.refresh()
  if (result) {
    data.value = result || { forwarding: [], pods: [], history: [], active_policy: null }
    const inherited = data.value.forwarding?.length === 1 && data.value.forwarding[0] === '/etc/resolv.conf'
    resolvers.value = data.value.active_policy?.resolvers?.length ? [...data.value.active_policy.resolvers] : inherited ? [''] : [...(data.value.forwarding || [])]
    if (!resolvers.value.length) resolvers.value = ['']
  } else if (dnsResource.error.value) error.value = dnsResource.error.value.message || '加载集群 DNS 状态失败'
  loading.value = false
  loaded.value = true
}

function openConfirm() {
  const malformed = normalizedResolvers.value.some(value => !/^([0-9]{1,3}\.){3}[0-9]{1,3}$|:/.test(value))
  if (!normalizedResolvers.value.length || malformed) {
    configError.value = '请填写有效的 DNS IP 地址'
    return
  }
  configError.value = ''
  configuring.value = false
  confirming.value = true
}

function openConfig() {
  configError.value = ''
  configuring.value = true
}

function closeConfig() {
  configuring.value = false
  configError.value = ''
}

function openResetConfirm() {
  closeConfig()
  resetConfirming.value = true
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { hour12: false })
}

async function apply() {
  saving.value = true
  actionError.value = ''
  try {
    await updateClusterDNS({ resolvers: normalizedResolvers.value })
    confirming.value = false
    await load()
  } catch (err) {
    actionError.value = err.message || '应用集群 DNS 策略失败'
  } finally {
    saving.value = false
  }
}

async function reset() {
  saving.value = true
  actionError.value = ''
  try {
    await deleteClusterDNS()
    resetConfirming.value = false
    await load()
  } catch (err) {
    actionError.value = err.message || '恢复宿主机 DNS 失败'
  } finally {
    saving.value = false
  }
}

async function rollback(policy) {
  if (!window.confirm(`回滚到 DNS 策略版本 ${policy.revision}？`)) return
  saving.value = true
  actionError.value = ''
  try {
    await rollbackClusterDNS(policy.revision)
    await load()
  } catch (err) {
    actionError.value = err.message || '回滚 DNS 策略失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.dns-history-workspace { margin-top: var(--space-12); }
.dns-form { padding: 0; }
.resolver-row { display: flex; align-items: center; gap: 8px; margin-bottom: 9px; }
.resolver-label { width: 78px; flex: 0 0 78px; color: var(--text-secondary); font-size: 12px; }
.resolver-row .form-input { flex: 1; }
.icon-btn { width: 32px; height: 32px; border: 0; background: transparent; color: var(--text-secondary); font-size: 22px; cursor: pointer; }
.add-resolver-button { margin-left: 86px; }
.dns-form-status { display: flex; align-items: center; gap: var(--space-8); margin: var(--space-12) 0 0; color: var(--text-secondary); font-size: 12px; }
.dns-upstream-value { display: block; overflow: hidden; max-width: 360px; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 640px) {
  .dns-form-status { align-items: flex-start; flex-direction: column; }
  .dns-pod-table th:nth-child(2), .dns-pod-table td:nth-child(2), .dns-pod-table th:nth-child(3), .dns-pod-table td:nth-child(3), .dns-history-table th:nth-child(3), .dns-history-table td:nth-child(3), .dns-history-table th:nth-child(4), .dns-history-table td:nth-child(4) { display: none; }
}
</style>
