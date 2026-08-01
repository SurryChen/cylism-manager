<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">系统设置</h1>
    </div>

    <div class="settings-grid">
      <section class="card">
        <div class="card-header">
          <h2 class="card-title">Tailscale</h2>
          <span class="badge" :class="tailscale.initialized ? 'badge-online' : 'badge-offline'">
            {{ tailscale.initialized ? '已配置' : '待配置' }}
          </span>
        </div>
        <div class="detail-grid">
          <span class="detail-label">控制面 IP</span><span>{{ tailscale.ip || '-' }}</span>
          <span class="detail-label">在线状态</span><span>{{ tailscale.online ? '在线' : '离线' }}</span>
        </div>
        <p class="settings-copy" style="margin-top:var(--space-12)">
          Tailscale Auth Key、服务器导入和组网相关配置统一收敛到系统设置与服务器页，不再单独占一个基础设施页面。
        </p>
      </section>

      <section class="card">
        <div class="card-header">
          <h2 class="card-title">集群引导</h2>
        </div>
        <p class="settings-copy">
          Worker 加入所需的 join token、默认 SSH 超时和更多平台级设置会在这里集中管理。
        </p>
        <span class="badge badge-offline">本轮先提供入口与状态展示</span>
      </section>

      <section class="card platform-update-card">
        <div class="card-header"><div><h2 class="card-title">平台自更新</h2><p class="settings-copy">GitHub Action 将不可变镜像 digest 提交到平台，平台仅更新自身工作负载。</p></div><span class="badge" :class="platform.webhook_configured ? 'badge-online' : 'badge-offline'">{{ platform.webhook_configured ? 'Webhook 已配置' : '待配置' }}</span></div>
        <div v-if="platform.deployment" class="detail-grid"><span class="detail-label">当前镜像</span><code>{{ platform.deployment.image || '-' }}</code><span class="detail-label">就绪副本</span><span>{{ platform.deployment.ready_replicas || 0 }} / {{ platform.deployment.desired_replicas || 1 }}</span></div>
        <div class="form-group"><label class="form-label">允许的镜像前缀</label><div class="settings-action-row"><input v-model.trim="platformImagePrefix" class="form-input" :disabled="savingPrefix" /><button class="btn btn-sm" :disabled="savingPrefix || !platformImagePrefix" @click="savePlatformImagePrefix">保存</button></div></div>
        <div class="settings-action-row"><button class="btn btn-sm btn-primary" data-testid="generate-platform-webhook-secret" :disabled="generatingSecret" @click="generatePlatformWebhookSecret">{{ generatingSecret ? '生成中...' : '生成或轮换密钥' }}</button></div>
        <div v-if="generatedSecret" class="secret-once"><strong>仅显示一次</strong><code>{{ generatedSecret }}</code></div>
        <div v-if="platform.releases?.length" class="platform-release-list"><div v-for="release in platform.releases.slice(0, 5)" :key="release.id" class="platform-release-row"><div><code>{{ release.image }}</code><small>{{ release.source }} · {{ release.status }}</small></div><button class="btn btn-sm" :disabled="rollingBack === release.id || !release.previous_image" @click="rollbackPlatformRelease(release)">{{ rollingBack === release.id ? '提交中...' : '回滚' }}</button></div></div>
        <p v-else class="settings-copy">尚无平台发布记录。</p>
      </section>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const tailscale = ref({ initialized: false, ip: '', online: false })
const platform = ref({ webhook_configured: false, image_prefix: '', deployment: null, releases: [] })
const platformImagePrefix = ref('')
const generatedSecret = ref('')
const generatingSecret = ref(false)
const savingPrefix = ref(false)
const rollingBack = ref(0)
let refreshTimer

onMounted(async () => {
  await refresh()
  refreshTimer = window.setInterval(refresh, 15000)
})
onBeforeUnmount(() => window.clearInterval(refreshTimer))

async function refresh() {
  const [tailscaleResult, platformResult] = await Promise.allSettled([api.get('/tailscale/status'), api.get('/platform/status')])
  tailscale.value = tailscaleResult.status === 'fulfilled' ? tailscaleResult.value : { initialized: false, ip: '', online: false }
  if (platformResult.status === 'fulfilled') {
    platform.value = platformResult.value
    platformImagePrefix.value = platform.value.image_prefix || ''
  }
}

async function generatePlatformWebhookSecret() {
  generatingSecret.value = true
  try { generatedSecret.value = (await api.post('/platform/webhook-secret')).secret; await refresh() } finally { generatingSecret.value = false }
}

async function savePlatformImagePrefix() {
  savingPrefix.value = true
  try { await api.put('/platform/image-prefix', { image_prefix: platformImagePrefix.value }); await refresh() } finally { savingPrefix.value = false }
}

async function rollbackPlatformRelease(release) {
  rollingBack.value = release.id
  try { await api.post(`/platform/releases/${release.id}/rollback`); await refresh() } finally { rollingBack.value = 0 }
}
</script>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-16);
}

.settings-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.platform-update-card{grid-column:1/-1}.platform-update-card .card-header{align-items:flex-start}.settings-action-row{display:flex;align-items:center;gap:8px}.settings-action-row .form-input{min-width:0;flex:1}.secret-once{display:grid;gap:6px;margin-top:12px;padding:10px;border:1px solid var(--warning);border-radius:var(--radius-control);background:var(--warning-surface);font-size:12px}.secret-once code,.platform-release-row code{overflow-wrap:anywhere}.platform-release-list{display:grid;gap:8px;margin-top:16px}.platform-release-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.platform-release-row div{min-width:0;display:grid;gap:4px}.platform-release-row small{color:var(--text-secondary);font-size:11px}@media(max-width:600px){.platform-release-row,.settings-action-row{align-items:stretch;flex-direction:column}}
</style>
