<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <h2 class="section-title">发布与更新</h2>
        <p class="settings-copy">这里负责平台镜像、允许前缀、Webhook 和发布历史。</p>
      </div>
    </div>

    <section class="card platform-update-card">
      <div class="card-header">
        <div>
          <h2 class="card-title">平台自更新</h2>
          <p class="settings-copy">GitHub Action 推送镜像后通知平台滚动更新自身工作负载。</p>
        </div>
        <span class="badge" :class="platform.webhook_configured ? 'badge-online' : 'badge-offline'">{{ platform.webhook_configured ? 'Webhook 已配置' : '待配置' }}</span>
      </div>
      <div v-if="platform.deployment" class="detail-grid">
        <span class="detail-label">当前镜像</span><code>{{ platform.deployment.image || '-' }}</code>
        <span class="detail-label">就绪副本</span><span>{{ platform.deployment.ready_replicas || 0 }} / {{ platform.deployment.desired_replicas || 1 }}</span>
      </div>
      <div class="platform-auto-update">
        <span class="detail-label">最近一次自动更新</span>
        <div v-if="latestAutomaticRelease" class="platform-auto-release">
          <div class="platform-auto-release-meta">
            <span class="badge" :class="latestAutomaticRelease.status === 'succeeded' ? 'badge-online' : latestAutomaticRelease.status === 'failed' ? 'badge-danger' : 'badge-deploying'">{{ latestAutomaticRelease.status }}</span>
            <small>{{ formatDateTime(latestAutomaticRelease.created_at) }}</small>
            <small v-if="latestAutomaticRelease.commit_sha">提交 {{ latestAutomaticRelease.commit_sha.slice(0, 12) }}</small>
          </div>
          <code>{{ latestAutomaticRelease.image }}</code>
        </div>
        <span v-else class="settings-copy">尚无 GitHub Action 自动更新记录。</span>
      </div>
      <div class="form-group">
        <label class="form-label" for="platform-manual-image">手动更新镜像 Tag</label>
        <div class="settings-action-row">
          <input id="platform-manual-image" v-model.trim="manualImage" data-testid="platform-manual-image" class="form-input" :placeholder="`${platform.image_prefix || 'registry.example.com/cylism-manager'}:latest`" :disabled="updatingPlatform" />
          <button class="btn btn-sm btn-primary" data-testid="platform-manual-update" :disabled="updatingPlatform || !manualImage" @click="manualPlatformUpdate">{{ updatingPlatform ? '提交中...' : '手动更新' }}</button>
        </div>
        <p class="settings-copy form-hint">仅支持允许仓库前缀下的镜像 Tag。每次提交都会滚动重启平台，并按 imagePullPolicy 拉取镜像。</p>
      </div>
      <p v-if="platformActionMessage" class="settings-copy platform-action-message">{{ platformActionMessage }}</p>
      <p v-if="platformReadError" class="settings-copy endpoint-error">{{ platformReadError }}</p>
      <div class="form-group">
        <label class="form-label">允许的镜像前缀</label>
        <div class="settings-action-row">
          <textarea v-model.trim="platformImagePrefix" data-testid="platform-image-prefix" class="form-input platform-prefix-input" rows="3" :disabled="savingPrefix" placeholder="每行一个前缀，例如 registry.example.com/cylism-manager" @input="platformPrefixDirty = true" />
          <button class="btn btn-sm" data-testid="platform-image-prefix-save" :disabled="savingPrefix || !platformImagePrefix" @click="savePlatformImagePrefix">保存</button>
        </div>
      </div>
      <div class="settings-action-row">
        <button class="btn btn-sm btn-primary" data-testid="generate-platform-webhook-secret" :disabled="generatingSecret" @click="generatePlatformWebhookSecret">
          {{ generatingSecret ? '生成中...' : '生成或轮换密钥' }}
        </button>
      </div>
      <div v-if="generatedSecret" class="secret-once">
        <strong>仅显示一次</strong>
        <code>{{ generatedSecret }}</code>
      </div>
      <div v-if="platform.releases?.length" class="platform-release-list">
        <div v-for="release in platform.releases.slice(0, 5)" :key="release.id" class="platform-release-row">
          <div>
            <code>{{ release.image }}</code>
            <small>{{ release.source }} · {{ release.status }}</small>
          </div>
          <button class="btn btn-sm" :disabled="rollingBack === release.id || !release.previous_image" @click="rollbackPlatformRelease(release)">
            {{ rollingBack === release.id ? '提交中...' : '回滚' }}
          </button>
        </div>
      </div>
      <p v-else class="settings-copy">尚无平台发布记录。</p>
    </section>

    <Teleport to="body">
      <div v-if="platformActionError" class="overlay platform-action-notice-overlay" @click.self="platformActionError = ''">
        <section class="modal platform-action-notice-modal" role="alertdialog" aria-modal="true" aria-labelledby="platform-action-error-title">
          <h2 id="platform-action-error-title" class="modal-title">平台更新失败</h2>
          <p class="settings-copy platform-action-error-text">{{ platformActionError }}</p>
          <div class="modal-actions"><button class="btn btn-danger" @click="platformActionError = ''">知道了</button></div>
        </section>
      </div>
    </Teleport>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { createPlatformRelease, generatePlatformWebhookSecret as generatePlatformWebhookSecretRequest, getPlatformStatus, rollbackPlatformRelease as rollbackPlatformReleaseRequest, updatePlatformImagePrefix } from '../../api/settings.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'

const platform = ref({ webhook_configured: false, image_prefix: '', deployment: null, releases: [] })
const platformImagePrefix = ref('')
const manualImage = ref('')
const platformActionMessage = ref('')
const platformActionError = ref('')
const platformReadError = ref('')
const generatedSecret = ref('')
const generatingSecret = ref(false)
const savingPrefix = ref(false)
const updatingPlatform = ref(false)
const rollingBack = ref(0)
const platformPrefixDirty = ref(false)
const platformResource = useAsyncResource(({ signal }) => getPlatformStatus({ signal }), null)

const latestAutomaticRelease = computed(() => (platform.value.releases || []).find(release => release.source === 'github') || null)

onMounted(() => {
  void refresh({ syncForm: true })
})

async function refresh({ syncForm = false } = {}) {
  platformReadError.value = ''
  const result = await platformResource.refresh()
  if (!result) {
    platformReadError.value = platformResource.error.value?.message || '读取平台发布状态失败'
    return
  }
  platform.value = result
  if (syncForm || !platformPrefixDirty.value) {
    platformImagePrefix.value = platform.value.image_prefix || ''
    platformPrefixDirty.value = false
  }
}

async function generatePlatformWebhookSecret() {
  generatingSecret.value = true
  platformActionError.value = ''
  try {
    generatedSecret.value = (await generatePlatformWebhookSecretRequest()).secret
    await refresh()
  } catch (e) {
    platformActionError.value = e.message || '生成平台 Webhook 密钥失败'
  } finally {
    generatingSecret.value = false
  }
}

async function savePlatformImagePrefix() {
  savingPrefix.value = true
  platformActionError.value = ''
  try {
    await updatePlatformImagePrefix({ image_prefix: platformImagePrefix.value })
    platformPrefixDirty.value = false
    await refresh({ syncForm: true })
  } catch (e) {
    platformActionError.value = e.message || '保存镜像前缀失败'
  } finally {
    savingPrefix.value = false
  }
}

async function manualPlatformUpdate() {
  updatingPlatform.value = true
  platformActionMessage.value = ''
  platformActionError.value = ''
  try {
    await createPlatformRelease({ image: manualImage.value })
    manualImage.value = ''
    platformActionMessage.value = '平台更新已提交'
    await refresh()
  } catch (e) {
    platformActionError.value = e.message || '平台更新失败'
  } finally {
    updatingPlatform.value = false
  }
}

async function rollbackPlatformRelease(release) {
  rollingBack.value = release.id
  platformActionError.value = ''
  try {
    await rollbackPlatformReleaseRequest(release.id)
    await refresh()
  } catch (e) {
    platformActionError.value = e.message || '回滚平台发布失败'
  } finally {
    rollingBack.value = 0
  }
}

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN', { hour12: false })
}

defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
