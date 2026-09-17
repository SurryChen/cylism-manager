<template>
  <div>
    <PageHeader title="集群 DNS" description="统一管理 CoreDNS 的外部解析上游；副本数和调度仍在系统组件中管理">
      <template #actions>
        <button class="btn btn-primary" :disabled="loading" @click="load">刷新</button>
      </template>
    </PageHeader>

    <FeedbackBanner v-if="error" tone="warning" :message="error" class="section-gap" />
    <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" class="section-gap" />

    <template v-if="loaded">
      <section class="dns-summary metric-grid section-gap" aria-label="集群 DNS 状态">
        <article class="metric dns-summary-item"><span>当前转发</span><strong>{{ forwardingText }}</strong></article>
        <article class="metric dns-summary-item"><span>平台策略</span><strong>{{ data.active_policy?.resolvers?.length ? `版本 ${data.active_policy.revision}` : '继承宿主机 DNS' }}</strong></article>
        <article class="metric dns-summary-item"><span>CoreDNS</span><strong>{{ readyPods }}/{{ data.pods?.length || 0 }} 就绪</strong></article>
      </section>

      <SurfaceCard class="section-gap">
        <SectionHeading title="外部 DNS 上游" description="仅接受 IP 地址。保存时只替换 CoreDNS 根域的 forward 指令，其余 Corefile 保持不变。">
          <template #actions>
            <button class="btn btn-sm" :disabled="saving || inheritedDNS" @click="openResetConfirm">恢复宿主机 DNS</button>
          </template>
        </SectionHeading>
        <form class="dns-form" @submit.prevent="openConfirm">
          <div v-for="(_, index) in resolvers" :key="index" class="resolver-row">
            <span class="resolver-index">{{ index + 1 }}</span>
            <input v-model.trim="resolvers[index]" class="form-input" inputmode="decimal" placeholder="例如 223.5.5.5" :aria-label="`DNS 上游 ${index + 1}`" />
            <button v-if="resolvers.length > 1" type="button" class="icon-btn" :aria-label="`移除 DNS 上游 ${index + 1}`" @click="resolvers.splice(index, 1)">×</button>
          </div>
          <p v-if="inheritedDNS" class="form-hint inherited-hint">当前未配置外部 DNS，CoreDNS 使用各节点宿主机的 DNS 配置。</p>
          <div class="dns-actions">
            <button v-if="resolvers.length < 3" type="button" class="btn btn-sm" @click="resolvers.push('')">添加备用上游</button>
            <button class="btn btn-primary" :disabled="saving">{{ saving ? '应用中...' : '验证并应用' }}</button>
          </div>
        </form>
      </SurfaceCard>

      <SurfaceCard class="section-gap">
        <SectionHeading title="CoreDNS 副本" description="用于确认策略实际覆盖的 DNS 工作负载。" />
        <div v-if="data.pods?.length" class="pod-list">
          <div v-for="pod in data.pods" :key="pod.name" class="pod-row"><strong>{{ pod.name }}</strong><span>{{ pod.node || '-' }}</span><span>{{ pod.ip || '-' }}</span><span class="badge" :class="pod.ready ? 'badge-online' : 'badge-danger'">{{ pod.ready ? '就绪' : '未就绪' }}</span></div>
        </div>
        <EmptyState v-else message="未发现 CoreDNS Pod" />
      </SurfaceCard>

      <SurfaceCard v-if="data.history?.length" class="section-gap">
        <SectionHeading title="策略历史" description="回滚会以选中版本的上游创建一个新的策略版本。" />
        <div class="history-list">
          <div v-for="policy in data.history" :key="policy.revision" class="history-row"><div><strong>版本 {{ policy.revision }}</strong><small>{{ policy.resolvers?.join('，') }}</small></div><button class="btn btn-sm" :disabled="saving || policy.revision === data.active_policy?.revision" @click="rollback(policy)">{{ policy.revision === data.active_policy?.revision ? '当前版本' : '回滚到此版本' }}</button></div>
        </div>
      </SurfaceCard>
    </template>

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
import { deleteClusterDNS, getClusterDNS, rollbackClusterDNS, updateClusterDNS } from '../../api/cluster-dns.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import PageHeader from '../../components/PageHeader.vue'
import SectionHeading from '../../components/SectionHeading.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const data = ref({ forwarding: [], pods: [], history: [], active_policy: null })
const resolvers = ref([''])
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const error = ref('')
const actionError = ref('')
const confirming = ref(false)
const resetConfirming = ref(false)
const dnsResource = useAsyncResource(({ signal }) => getClusterDNS({ signal }), null)

const normalizedResolvers = computed(() => resolvers.value.map(value => value.trim()).filter(Boolean))
const forwardingText = computed(() => data.value.forwarding?.join('，') || '未识别')
const readyPods = computed(() => (data.value.pods || []).filter(pod => pod.ready).length)
const inheritedDNS = computed(() => !(data.value.active_policy?.resolvers?.length) && data.value.forwarding?.length === 1 && data.value.forwarding[0] === '/etc/resolv.conf')

async function load() {
  error.value = ''
  const result = await dnsResource.refresh()
  loading.value = dnsResource.loading.value
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
    error.value = '请填写有效的 DNS IP 地址'
    return
  }
  confirming.value = true
}

function openResetConfirm() {
  resetConfirming.value = true
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
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.dns-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.dns-summary-item { min-height: 96px; gap: 6px; padding: 16px 18px; }
.dns-summary-item span, .dns-summary-item strong, .history-row small { display: block; }
.dns-summary-item span, .history-row small { color: var(--text-secondary); font-size: 12px; }
.dns-summary-item strong { overflow-wrap: anywhere; font-size: 17px; line-height: 1.35; }
.section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.section-heading h2 { margin: 0; font-size: 15px; }
.section-heading p { margin: 5px 0 0; color: var(--text-secondary); font-size: 13px; line-height: 1.5; }
.dns-form { max-width: 560px; }
.resolver-row { display: flex; align-items: center; gap: 8px; margin-bottom: 9px; }
.resolver-index { width: 22px; color: var(--text-secondary); font-size: 12px; text-align: center; }
.resolver-row .form-input { flex: 1; }
.icon-btn { width: 32px; height: 32px; border: 0; background: transparent; color: var(--text-secondary); font-size: 22px; cursor: pointer; }
.dns-actions { display: flex; gap: 8px; margin-top: 14px; }
.inherited-hint { margin: 10px 0 0; }
.pod-list, .history-list { border-top: 1px solid var(--border); }
.pod-row, .history-row { display: grid; align-items: center; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border); }
.pod-row { grid-template-columns: minmax(0, 2fr) minmax(0, 1.5fr) minmax(0, 1fr) auto; }
.history-row { grid-template-columns: 1fr auto; }
.pod-row span { color: var(--text-secondary); overflow-wrap: anywhere; }
.history-row small { margin-top: 3px; }
@media (max-width: 640px) { .page-header { flex-direction: column; } .page-header .btn { width: 100%; } .dns-summary { grid-template-columns: 1fr; } .pod-row { grid-template-columns: 1fr auto; } .pod-row span { display: none; } }
</style>
