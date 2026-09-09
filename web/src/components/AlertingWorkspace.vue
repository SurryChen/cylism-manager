<template>
  <section v-if="!monitoringReady" class="card alerting-empty">
    <div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">等待 VictoriaMetrics 就绪后再启用告警</span></div>
  </section>

  <section v-else-if="loading && !status" class="card alerting-empty">
    <div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">正在读取告警状态...</span></div>
  </section>

  <section v-else-if="status?.state === 'not_installed'" class="card alerting-install-card">
    <div class="card-header"><div><h2 class="card-title">告警尚未启用</h2><p class="status-copy">规则将在集群内每分钟评估，并通过 Alertmanager 汇总和通知。</p></div><span class="badge badge-offline">未安装</span></div>
    <form class="alerting-form" @submit.prevent="install">
      <div class="form-group"><label class="form-label">告警节点</label><select v-model="installForm.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ displayNode(node) }}</option></select><p class="form-hint">Alertmanager 的 1Gi 本地 PVC 会绑定到该节点。默认优先选择与指标数据节点不同的节点。</p></div>
      <div class="form-group"><label class="form-label">飞书机器人地址</label><input v-model.trim="installForm.feishu_webhook_url" class="form-input" type="url" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." /><p class="form-hint">可稍后在告警设置中填写。地址仅写入 Kubernetes Secret，不会再次展示。</p></div>
      <div class="modal-actions status-actions"><button class="btn btn-primary" :disabled="installing || !installForm.node_name">{{ installing ? '正在提交...' : '启用告警' }}</button><button class="btn" type="button" :disabled="installing" @click="refresh">重新检测</button></div>
    </form>
  </section>

  <template v-else-if="status">
    <section v-if="status.state !== 'ready'" class="card alerting-empty"><div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">{{ status.message || '等待告警组件就绪' }}</span></div></section>

    <template v-else>
      <section class="alert-summary metric-grid section-gap">
        <article class="metric"><span>正在告警</span><strong :class="overview.firing ? 'is-danger' : ''">{{ overview.firing || 0 }}</strong><small>{{ overview.firing ? '需要优先处理' : '当前无触发告警' }}</small></article>
        <article class="metric"><span>活跃告警</span><strong>{{ overview.active.length }}</strong><small>包含已静默告警</small></article>
        <article class="metric"><span>已静默</span><strong>{{ overview.silenced || 0 }}</strong><small>不会重复发送通知</small></article>
        <article class="metric"><span>通知渠道</span><strong><span class="badge" :class="status.notification_configured ? 'badge-online' : 'badge-offline'">{{ status.notification_configured ? '已配置' : '未配置' }}</span></strong><small>{{ status.node_name || '-' }}</small></article>
      </section>

      <section class="alert-section-heading section-gap"><div><h2>自动处置</h2><p>告警先由指定 Nanobot 生成诊断报告；清理动作始终需要人工审批。</p></div></section>
      <section class="automation-panel section-gap">
        <div class="automation-toggle"><div><strong>启用告警自动分析</strong><small>只开放固定的告警、指标和节点诊断接口</small></div><label class="automation-switch" aria-label="启用告警自动分析"><input v-model="automationPolicy.enabled" type="checkbox" /><span aria-hidden="true" /></label></div>
        <div class="automation-fields">
          <label class="form-group"><span class="form-label">处理 Runtime</span><select v-model.number="automationPolicy.runtime_id" class="form-select" :disabled="!automationPolicy.enabled"><option :value="0" disabled>选择 Nanobot Runtime</option><option v-for="runtime in nanobotRuntimes" :key="runtime.id" :value="runtime.id">{{ runtime.name }} · {{ runtime.status }}</option></select></label>
          <label class="form-group"><span class="form-label">最低严重程度</span><select v-model="automationPolicy.minimum_severity" class="form-select" :disabled="!automationPolicy.enabled"><option value="warning">警告</option><option value="critical">严重</option></select></label>
          <label class="form-group"><span class="form-label">处置模式</span><select v-model="automationPolicy.mode" class="form-select" :disabled="!automationPolicy.enabled"><option value="report_only">仅生成报告</option><option value="diagnose_and_request_approval">诊断后可请求审批</option></select></label>
          <label class="form-group"><span class="form-label">重复分析冷却</span><div class="field-suffix"><input v-model.number="automationPolicy.cooldown_minutes" class="form-input" type="number" min="5" max="1440" :disabled="!automationPolicy.enabled" /><span>分钟</span></div></label>
        </div>
        <p v-if="automationNotice" class="automation-notice" :class="`is-${automationNotice.type}`" role="status">{{ automationNotice.text }}</p>
        <div class="automation-footer"><small>不会授予 Shell、SSH、kubectl 或任意命令执行权限。</small><button class="btn btn-primary" :disabled="automationSaving || (automationPolicy.enabled && !automationPolicy.runtime_id)" @click="saveAutomationPolicy">{{ automationSaving ? '保存中...' : '保存自动处置策略' }}</button></div>
      </section>

      <section v-if="automationEvents.length" class="automation-events section-gap">
        <div class="alert-section-heading"><div><h2>自动处置事件</h2><p>诊断、审批、执行与复测状态以持久化事件为准。</p></div></div>
        <div class="event-list">
          <article v-for="event in automationEvents" :key="event.id" class="event-row">
            <div class="event-state" :class="`state-${event.status}`"><Bot :size="16" /></div>
            <div class="event-copy">
              <strong>{{ event.alert_name }}<span v-if="event.node_name"> · {{ event.node_name }}</span></strong>
              <small>{{ automationEventState(event) }} · {{ formatTime(event.updated_at) }}</small>
              <p v-if="event.diagnostic_summary">{{ event.diagnostic_summary }}</p>
              <p v-else-if="event.last_error" class="event-error">{{ event.last_error }}</p>
              <details v-if="event.report" class="event-report">
                <summary>Agent 报告</summary>
                <div class="event-report-markdown" v-html="renderReportMarkdown(event.report)" />
              </details>
            </div>
            <span v-if="event.operation_id" class="badge badge-deploying">等待审批</span>
          </article>
        </div>
      </section>

      <section class="alert-section-heading section-gap"><div><h2>正在告警</h2><p>按严重程度排序，优先处理影响服务可用性的异常</p></div><div class="icon-actions"><button class="icon-button" title="刷新告警" aria-label="刷新告警" :disabled="loading" @click="refresh"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button><button class="icon-button" title="告警设置" aria-label="告警设置" @click="openSettings"><Settings2 :size="16" /></button></div></section>

      <section v-if="overview.active.length" class="alert-list section-gap">
        <article v-for="alert in sortedAlerts" :key="alert.fingerprint || alertKey(alert)" class="alert-row" :class="severityClass(alert)">
          <div class="alert-severity"><BellRing :size="17" /><span class="badge" :class="severityBadge(alert)">{{ severityLabel(alert) }}</span></div>
          <div class="alert-copy"><strong>{{ alert.annotations?.summary || alert.labels?.alertname || '集群告警' }}</strong><small>{{ alertDescription(alert) }}</small><small v-if="alertCurrentValue(alert)" class="alert-reading"><span>当前值</span><strong>{{ alertCurrentValue(alert) }}</strong><span v-if="alertThreshold(alert)">阈值 {{ alertThreshold(alert) }}</span></small><small>开始于 {{ formatTime(alert.startsAt) }}</small></div>
          <div class="alert-actions"><button v-if="alertTarget(alert)" class="icon-button" title="查看关联资源" aria-label="查看关联资源" @click="navigate(alert)"><ArrowUpRight :size="16" /></button><button class="icon-button" title="静默告警" aria-label="静默告警" @click="openSilence(alert)"><VolumeX :size="16" /></button></div>
        </article>
      </section>
      <section v-else class="alerting-healthy card section-gap"><CheckCircle2 :size="20" /><div><strong>所有告警均已恢复</strong><small>当前 {{ status.rules?.filter(rule => rule.enabled).length || 0 }} 条规则正在评估。</small></div></section>

      <section v-if="overview.resolved.length" class="alert-section-heading section-gap"><div><h2>最近恢复</h2><p>仅保留 Alertmanager 当前可见的恢复事件</p></div></section>
      <section v-if="overview.resolved.length" class="card table-wrap alert-resolved"><table class="data-table"><thead><tr><th>告警</th><th>对象</th><th>恢复时间</th></tr></thead><tbody><tr v-for="alert in overview.resolved" :key="alert.fingerprint || alertKey(alert)"><td class="cell-primary">{{ alert.annotations?.summary || alert.labels?.alertname || '-' }}</td><td>{{ alertTarget(alert) || '-' }}</td><td>{{ formatTime(alert.endsAt) }}</td></tr></tbody></table></section>
    </template>
  </template>

  <div v-if="silencingAlert" class="overlay" @click.self="silencingAlert = null"><form class="modal alert-silence-modal" @submit.prevent="createSilence"><div class="card-header"><div><h2 class="modal-title">静默告警</h2><p class="status-copy">{{ silencingAlert.annotations?.summary || silencingAlert.labels?.alertname }}</p></div><button class="icon-button" type="button" title="关闭" aria-label="关闭" @click="silencingAlert = null"><X :size="16" /></button></div><div class="form-group"><label class="form-label">静默时长</label><select v-model.number="silenceForm.duration_minutes" class="form-select"><option :value="60">1 小时</option><option :value="240">4 小时</option><option :value="1440">24 小时</option></select></div><div class="form-group"><label class="form-label">说明</label><input v-model.trim="silenceForm.comment" class="form-input" maxlength="120" placeholder="计划维护" /></div><div class="modal-actions"><button class="btn" type="button" @click="silencingAlert = null">取消</button><button class="btn btn-danger" :disabled="silencing" type="submit">{{ silencing ? '正在静默...' : '确认静默' }}</button></div></form></div>

  <Teleport to="body">
    <div v-if="settingsOpen" class="overlay alert-settings-overlay" @click.self="settingsOpen = false">
      <form class="modal alert-settings-modal" @submit.prevent="saveSettings">
        <header class="drawer-header">
          <div><h2>告警设置</h2><p>规则、通知渠道与提醒频率</p></div>
          <button class="icon-button" type="button" title="关闭" aria-label="关闭" @click="settingsOpen = false"><X :size="17" /></button>
        </header>
        <section class="drawer-section">
          <div class="drawer-section-heading"><div><h3>飞书通知</h3><p>地址仅写入 Secret，不会再次展示</p></div><span class="badge" :class="status?.feishu_configured ? 'badge-online' : 'badge-offline'">{{ status?.feishu_configured ? '已配置' : '未配置' }}</span></div>
          <input v-model.trim="settingsForm.feishu_webhook_url" class="form-input" type="url" placeholder="填写新的飞书机器人地址以更新" />
          <div class="drawer-actions"><button class="btn" type="button" :disabled="testingChannel !== '' || !status?.feishu_configured" @click="testNotification('feishu')">{{ testingChannel === 'feishu' ? '发送中...' : '测试飞书通知' }}</button></div>
        </section>
        <section class="drawer-section">
          <div class="drawer-section-heading"><div><h3>邮件通知</h3><p>通过 SMTP 发送，可与飞书并行</p></div><span class="badge" :class="status?.email_configured ? 'badge-online' : 'badge-offline'">{{ status?.email_configured ? '已配置' : '未配置' }}</span></div>
          <label class="check-row"><input v-model="settingsForm.email.enabled" type="checkbox" /> 配置 SMTP 邮件通知</label>
          <div v-if="settingsForm.email.enabled" class="email-fields">
            <div class="form-row"><div class="form-group"><label class="form-label">SMTP 主机</label><input v-model.trim="settingsForm.email.smtp_host" class="form-input" required placeholder="smtp.example.com" /></div><div class="form-group"><label class="form-label">端口</label><input v-model.number="settingsForm.email.smtp_port" class="form-input" type="number" min="1" max="65535" required /></div></div>
            <div class="form-row"><div class="form-group"><label class="form-label">TLS 模式</label><select v-model="settingsForm.email.tls_mode" class="form-select"><option value="starttls">STARTTLS (587)</option><option value="tls">TLS (465)</option></select></div><div class="form-group"><label class="form-label">SMTP 用户名</label><input v-model.trim="settingsForm.email.username" class="form-input" /></div></div>
            <div class="form-group"><label class="form-label">SMTP 密码</label><input v-model="settingsForm.email.password" class="form-input" type="password" /><p class="form-hint">账号认证可留空；填写用户名时必须同时填写密码。</p></div>
            <div class="form-row"><div class="form-group"><label class="form-label">发件人</label><input v-model.trim="settingsForm.email.from" class="form-input" type="email" required placeholder="alerts@example.com" /></div><div class="form-group"><label class="form-label">收件人</label><input v-model.trim="settingsForm.email.to" class="form-input" required placeholder="ops@example.com, admin@example.com" /></div></div>
          </div>
          <div class="drawer-actions"><button class="btn" type="button" :disabled="testingChannel !== '' || !status?.email_configured" @click="testNotification('email')">{{ testingChannel === 'email' ? '发送中...' : '测试邮件通知' }}</button></div>
        </section>
        <section class="drawer-section">
          <div class="drawer-section-heading"><div><h3>通知频率</h3><p>按固定标签维度合并相同告警</p></div></div>
          <div class="alert-timing-fields">
            <div class="form-group"><label class="form-label">首次等待</label><input v-model.number="settingsForm.notification_policy.group_wait_seconds" class="form-input" type="number" min="5" max="3600" required /><p class="form-hint">秒</p></div>
            <div class="form-group"><label class="form-label">同组新增间隔</label><input v-model.number="settingsForm.notification_policy.group_interval_minutes" class="form-input" type="number" min="1" max="1440" required /><p class="form-hint">分钟</p></div>
            <div class="form-group"><label class="form-label">重复提醒间隔</label><input v-model.number="settingsForm.notification_policy.repeat_interval_minutes" class="form-input" type="number" min="5" max="10080" required /><p class="form-hint">分钟</p></div>
          </div>
        </section>
        <section class="drawer-section">
          <div class="drawer-section-heading"><div><h3>告警规则</h3><p>修改后将重载规则评估</p></div></div>
          <div class="rule-list"><div v-for="rule in settingsForm.rules" :key="rule.id" class="rule-row"><label class="rule-title"><input v-model="rule.enabled" type="checkbox" /><span>{{ rule.name }}</span></label><div class="rule-fields"><label v-if="ruleSupportsThreshold(rule.id)"><span>阈值</span><input v-model.number="rule.threshold" class="form-input" type="number" min="0" max="100000" /></label><label><span>持续</span><input v-model.number="rule.duration_minutes" class="form-input" type="number" min="1" max="1440" /></label><small>分钟</small></div></div></div>
        </section>
        <footer class="drawer-footer"><button class="btn" type="button" :disabled="saving" @click="settingsOpen = false">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存设置' }}</button></footer>
      </form>
    </div>
  </Teleport>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { ArrowUpRight, BellRing, Bot, CheckCircle2, RefreshCw, Settings2, VolumeX, X } from 'lucide-vue-next'
import { api } from '../api/index.js'
import { getRuntimes } from '../api/runtimes.js'
import { getAlertingAutomationEvents, getAlertingAutomationPolicy, getAlertingOverview, getAlertingSilences, getAlertingStatus } from '../api/alerting.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const props = defineProps({ nodes: { type: Array, default: () => [] }, monitoringReady: Boolean, metricsNodeName: { type: String, default: '' } })
const emit = defineEmits(['navigate'])

const status = ref(null)
const overview = ref({ active: [], resolved: [], firing: 0, silenced: 0 })
const loading = ref(false)
const installing = ref(false)
const testingChannel = ref('')
const saving = ref(false)
const silencing = ref(false)
const settingsOpen = ref(false)
const silencingAlert = ref(null)
const installForm = ref({ node_name: '', feishu_webhook_url: '' })
const settingsForm = ref({ feishu_webhook_url: '', email: blankEmail(), notification_policy: defaultNotificationPolicy(), rules: [] })
const silenceForm = ref({ duration_minutes: 240, comment: '' })
const automationSaving = ref(false)
const automationNotice = ref(null)
const automationPolicy = ref(blankAutomationPolicy())
const automationEvents = ref([])
const runtimes = ref([])
const reportMarkdown = new MarkdownIt({ breaks: true, html: false, linkify: true }).use(taskLists, { enabled: true })
const statusResource = useAsyncResource(({ signal }) => getAlertingStatus({ signal }), null)
const alertResource = useAsyncResource(async ({ signal }) => {
  const [overviewResult, policy, events, runtimeItems] = await Promise.all([
    getAlertingOverview({ signal }),
    getAlertingAutomationPolicy({ signal }),
    getAlertingAutomationEvents({ signal }),
    getRuntimes({ signal }),
  ])
  return { overviewResult, policy, events, runtimeItems }
}, null)

const readyNodes = computed(() => props.nodes.filter(node => node.ready))
const sortedAlerts = computed(() => [...overview.value.active].sort((left, right) => severityWeight(left) - severityWeight(right)))

onMounted(() => { refresh() })

async function refresh() {
  if (!props.monitoringReady) return
  loading.value = true
  try {
    status.value = await statusResource.refresh()
    if (!installForm.value.node_name) installForm.value.node_name = defaultNodeName()
    if (!status.value) return
    if (status.value.state === 'ready') {
      const result = await alertResource.refresh()
      if (!result) return
      overview.value = result.overviewResult || { active: [], resolved: [], firing: 0, silenced: 0 }
      automationPolicy.value = { ...blankAutomationPolicy(), ...result.policy }
      automationEvents.value = Array.isArray(result.events) ? result.events : []
      runtimes.value = Array.isArray(result.runtimeItems) ? result.runtimeItems : []
    }
  } finally { loading.value = false }
}

function defaultNodeName() {
  return readyNodes.value.find(node => node.name !== props.metricsNodeName)?.name || readyNodes.value[0]?.name || ''
}
function displayNode(node) { return node.display_name || node.name }
function severityWeight(alert) { return alert.labels?.severity === 'critical' ? 0 : 1 }
function severityClass(alert) { return alert.labels?.severity === 'critical' ? 'is-critical' : 'is-warning' }
function severityBadge(alert) { return alert.labels?.severity === 'critical' ? 'badge-danger' : 'badge-deploying' }
function severityLabel(alert) { return alert.labels?.severity === 'critical' ? '严重' : '警告' }
function ruleSupportsThreshold(id) { return ['node-cpu-high', 'node-memory-high', 'node-disk-high', 'pod-restarts'].includes(id) }
function alertDescription(alert) { return alert.annotations?.description || alert.labels?.node || alert.labels?.pod || '请查看关联资源状态' }
function alertCurrentValue(alert) { return alert.annotations?.current_value || '' }
function alertThreshold(alert) { return alert.annotations?.threshold || '' }
function alertTarget(alert) { return alert.labels?.node || (alert.labels?.namespace && alert.labels?.pod ? `${alert.labels.namespace}/${alert.labels.pod}` : '') }
function alertKey(alert) { return `${alert.labels?.alertname || ''}-${alertTarget(alert)}-${alert.startsAt || ''}` }
function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-' }
function renderReportMarkdown(content) {
  return DOMPurify.sanitize(reportMarkdown.render(String(content || '')), { USE_PROFILES: { html: true } })
}

async function install() {
  installing.value = true
  try { await api.post('/monitoring/alerts/install', installForm.value); await refresh() } finally { installing.value = false }
}
function navigate(alert) { emit('navigate', { tab: alert.labels?.node ? 'nodes' : 'workloads', node: alert.labels?.node || '' }) }
function openSilence(alert) { silencingAlert.value = alert; silenceForm.value = { duration_minutes: 240, comment: '' } }
function alertMatchers(alert) {
  const labels = alert.labels || {}
  return ['alertname', 'node', 'namespace', 'pod', 'deployment', 'statefulset'].filter(key => labels[key]).map(key => ({ name: key, value: labels[key], isRegex: false, isEqual: true }))
}
async function createSilence() {
  silencing.value = true
  try { await api.post('/monitoring/alerts/silences', { ...silenceForm.value, matchers: alertMatchers(silencingAlert.value) }); silencingAlert.value = null; await refresh() } finally { silencing.value = false }
}
function blankEmail() { return { enabled: false, smtp_host: '', smtp_port: 587, username: '', password: '', from: '', to: '', tls_mode: 'starttls' } }
function defaultNotificationPolicy() { return { group_wait_seconds: 30, group_interval_minutes: 5, repeat_interval_minutes: 240 } }
function blankAutomationPolicy() { return { runtime_id: 0, enabled: false, alert_name: '', minimum_severity: 'warning', mode: 'report_only', cooldown_minutes: 30 } }
const nanobotRuntimes = computed(() => (Array.isArray(runtimes.value) ? runtimes.value : []).filter(runtime => runtime.runtime_type === 'nanobot' && runtime.deployment_mode === 'managed'))
function automationEventState(event) { return { firing: '正在告警', analyzing: '正在分析', awaiting_approval: '等待审批', remediating: '正在执行', resolved: '已恢复', failed: '自动化失败' }[event.status] || event.status }
async function saveAutomationPolicy() {
  automationSaving.value = true
  automationNotice.value = null
  try {
    const result = await api.put('/monitoring/alerts/automation-policy', automationPolicy.value)
    if (result?.sync_warning) {
      automationNotice.value = { type: 'warning', text: result.sync_warning }
    } else if (automationPolicy.value.enabled) {
      const synced = Number(result?.synced || 0)
      automationNotice.value = { type: 'success', text: synced > 0 ? `已同步 ${synced} 条当前告警，已按策略开始分析。` : '当前没有匹配的 firing 告警。' }
    } else {
      automationNotice.value = { type: 'neutral', text: '自动处置策略已保存，自动分析已关闭。' }
    }
    await refresh()
  } finally { automationSaving.value = false }
}
async function openSettings() {
  settingsForm.value = { feishu_webhook_url: '', email: blankEmail(), notification_policy: { ...(status.value?.notification_policy || defaultNotificationPolicy()) }, rules: (status.value?.rules || []).map(rule => ({ ...rule })) }
  settingsOpen.value = true
  try { await getAlertingSilences() } catch { /* The active-alert view remains usable when only silence history is unavailable. */ }
}
async function testNotification(channel) {
  testingChannel.value = channel
  try { await api.post(`/monitoring/alerts/test-notification?channel=${channel}`, {}) } finally { testingChannel.value = '' }
}
async function saveSettings() {
  saving.value = true
  try { await api.put('/monitoring/alerts/config', settingsForm.value); settingsOpen.value = false; await refresh() } finally { saving.value = false }
}

defineExpose({ refresh })
</script>

<style scoped>
.status-copy,.form-hint,.metric small,.alert-section-heading p,.drawer-header p,.drawer-section-heading p{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.alerting-empty .empty-state{min-height:160px}.alerting-form{margin-top:var(--space-20)}.status-actions{justify-content:flex-start;margin-top:var(--space-16)}.alert-summary{grid-template-columns:repeat(4,minmax(0,1fr))}.metric{display:grid;min-width:0;gap:5px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric>span{color:var(--text-secondary);font-size:11px}.metric strong{min-width:0;font-size:18px}.metric small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.metric .is-danger{color:var(--danger)}.alert-section-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-16)}.alert-section-heading h2{margin:0;color:var(--text-primary);font-size:16px}.icon-actions,.alert-actions{display:flex;align-items:center;gap:6px}.alert-list{border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.alert-row{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:12px;align-items:center;padding:13px 14px;border-bottom:1px solid var(--border-muted)}.alert-row:last-child{border-bottom:0}.alert-row.is-critical{box-shadow:inset 3px 0 0 var(--danger)}.alert-row.is-warning{box-shadow:inset 3px 0 0 var(--warning)}.alert-severity{display:grid;justify-items:center;gap:5px;color:var(--text-secondary)}.alert-copy{display:grid;min-width:0;gap:3px}.alert-copy>strong,.alert-copy>small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.alert-copy small{color:var(--text-secondary);font-size:12px}.alert-reading{display:flex;align-items:center;gap:7px;min-width:0}.alert-reading strong{color:var(--danger);font-size:13px;font-variant-numeric:tabular-nums}.alert-reading span:last-child{padding-left:7px;border-left:1px solid var(--border-muted)}.alerting-healthy{display:flex;align-items:center;gap:10px;padding:18px;border:1px solid var(--success-border);border-radius:var(--radius-control);background:var(--surface-glass);color:var(--success)}.alerting-healthy div{display:grid;gap:3px}.alerting-healthy small{color:var(--text-secondary);font-size:12px}.alert-resolved{padding:0;border-radius:var(--radius-control)}.automation-panel,.event-list{border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-glass)}.automation-panel{padding:16px}.automation-toggle,.automation-footer,.event-row{display:flex;align-items:center;justify-content:space-between;gap:14px}.automation-toggle strong,.event-copy strong{display:block;color:var(--text-primary);font-size:13px}.automation-toggle small,.automation-footer small,.event-copy small,.event-copy p{display:block;margin-top:4px;color:var(--text-secondary);font-size:12px}.automation-switch{display:inline-flex;flex:0 0 auto;padding:3px;border:1px solid var(--border-muted);border-radius:999px;background:var(--surface-raised);box-shadow:inset 0 1px 0 var(--border);cursor:pointer}.automation-switch input{position:absolute;opacity:0}.automation-switch span{display:block;width:34px;height:20px;border-radius:10px;background:var(--surface-subtle);transition:background .16s ease}.automation-switch span::after{display:block;width:16px;height:16px;margin:2px;border-radius:50%;background:var(--text-primary);box-shadow:0 1px 2px var(--border-muted);content:'';transition:transform .16s ease,background .16s ease}.automation-switch input:checked+span{background:var(--action-primary)}.automation-switch input:checked+span::after{background:var(--action-contrast);transform:translateX(14px)}.automation-switch:has(input:focus-visible){outline:2px solid var(--focus);outline-offset:2px}.automation-fields{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-top:16px}.automation-fields .form-group{margin:0}.field-suffix{display:flex;align-items:center;gap:8px}.field-suffix .form-input{min-width:0}.field-suffix span{color:var(--text-secondary);font-size:12px;white-space:nowrap}.automation-notice{margin:14px 0 0;padding:9px 11px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-secondary);font-size:12px;line-height:1.5}.automation-notice.is-success{border-color:var(--success);color:var(--success)}.automation-notice.is-warning{border-color:var(--warning);background:var(--warning-surface);color:var(--warning)}.automation-footer{margin-top:16px;padding-top:14px;border-top:1px solid var(--border-muted)}.automation-events>.alert-section-heading{margin-bottom:var(--space-16)}.event-row{align-items:flex-start;padding:14px;border-bottom:1px solid var(--border-muted)}.event-row:last-child{border-bottom:0}.event-state{display:grid;place-items:center;flex:0 0 30px;width:30px;height:30px;border-radius:50%;background:var(--surface-subtle);color:var(--text-secondary)}.state-awaiting_approval{color:var(--warning)}.state-failed{color:var(--danger)}.state-resolved{color:var(--success)}.event-copy{min-width:0;flex:1}.event-copy p{margin-bottom:0;white-space:pre-wrap}.event-error{color:var(--danger)!important}.event-report{margin-top:var(--space-12)}.event-report summary{display:flex;align-items:center;gap:6px;color:var(--text-secondary);font-size:12px;list-style:none;cursor:pointer}.event-report summary::-webkit-details-marker{display:none}.event-report summary::before{color:var(--action-primary);content:'▸';font-size:13px;transition:transform .16s ease}.event-report[open] summary::before{transform:rotate(90deg)}.event-report summary:hover{color:var(--text-primary)}.event-report-markdown{max-height:300px;overflow:auto;margin-top:10px;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font-size:12px;line-height:1.6;overflow-wrap:anywhere}.event-report-markdown :deep(p){margin:0 0 8px}.event-report-markdown :deep(p:last-child){margin-bottom:0}.event-report-markdown :deep(h1),.event-report-markdown :deep(h2),.event-report-markdown :deep(h3){margin:12px 0 6px;color:var(--text-primary);font-size:1em}.event-report-markdown :deep(ul),.event-report-markdown :deep(ol){margin:6px 0;padding-left:20px}.event-report-markdown :deep(li+li){margin-top:2px}.event-report-markdown :deep(a){color:var(--action-primary);text-decoration:underline}.event-report-markdown :deep(code){padding:1px 4px;border-radius:3px;background:var(--surface-input);font-family:var(--font-mono);font-size:.9em}.event-report-markdown :deep(pre){overflow-x:auto;margin:8px 0;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-input)}.event-report-markdown :deep(pre code){padding:0;background:transparent;white-space:pre}.event-report-markdown :deep(blockquote){margin:8px 0;padding-left:10px;border-left:2px solid var(--border);color:var(--text-secondary)}.event-report-markdown :deep(table){width:100%;margin:8px 0;border-collapse:collapse;font-size:12px}.event-report-markdown :deep(th),.event-report-markdown :deep(td){padding:4px 6px;border:1px solid var(--border-muted);text-align:left}.drawer-header,.drawer-section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.drawer-header h2,.drawer-section-heading h3{margin:0;color:var(--text-primary);font-size:16px}.drawer-section{padding:var(--space-20) 0;border-bottom:1px solid var(--border-muted)}.drawer-section>.form-input{margin-top:var(--space-16)}.drawer-actions{margin-top:10px}.rule-list{display:grid;gap:0;margin-top:var(--space-12);border-top:1px solid var(--border-muted)}.rule-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:center;padding:11px 0;border-bottom:1px solid var(--border-muted)}.rule-title{display:flex;align-items:center;gap:8px;min-width:0;color:var(--text-primary);font-size:13px}.rule-title input{margin:0}.rule-fields{display:flex;align-items:flex-end;gap:6px}.rule-fields label{display:grid;gap:3px;color:var(--text-secondary);font-size:10px}.rule-fields .form-input{width:58px;min-height:30px;margin:0;padding:4px 6px;font-size:11px}.rule-fields small{padding-bottom:7px;color:var(--text-secondary);font-size:10px}.drawer-footer{display:flex;justify-content:flex-end;gap:8px;padding-top:var(--space-20)}.alert-silence-modal{width:min(420px,calc(100vw - 32px))}.alert-silence-modal .form-group{margin-top:var(--space-16)}@media(max-width:760px){.alert-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.alert-section-heading{align-items:flex-start;flex-direction:column}.alert-row{grid-template-columns:auto minmax(0,1fr)}.alert-actions{grid-column:2;justify-content:flex-start}.rule-row{grid-template-columns:1fr}.rule-fields{justify-content:flex-start}.automation-fields{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:440px){.alert-summary,.automation-fields{grid-template-columns:1fr}.automation-footer{align-items:flex-start;flex-direction:column}}
.alert-settings-overlay{z-index:2000;align-items:center;justify-content:center}.alert-settings-modal{width:min(680px,100%);max-height:calc(100dvh - 32px)}.check-row{display:flex;gap:8px;margin-top:var(--space-16);color:var(--text-secondary);font-size:13px}.email-fields{margin-top:var(--space-16)}.email-fields .form-row{align-items:start}.email-fields .form-input,.email-fields .form-select{margin-top:0}.alert-timing-fields{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--space-12);margin-top:var(--space-16)}.alert-timing-fields .form-group{margin:0}@media(max-width:600px){.alert-timing-fields{grid-template-columns:1fr}}
</style>
