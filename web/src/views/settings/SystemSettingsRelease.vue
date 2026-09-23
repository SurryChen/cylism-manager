<template>
  <section class="settings-section">
    <div class="platform-release-layout">
      <SurfaceCard class="platform-release-config-card">
        <div class="platform-release-config-table">
          <header class="platform-release-config-header">
            <span>配置名</span>
            <span>配置值</span>
            <span>操作</span>
          </header>
          <div class="platform-release-config-row">
            <span class="platform-release-config-name">当前镜像</span>
            <code class="platform-release-config-value">{{ platform.deployment?.image || '-' }}</code>
            <span class="platform-release-config-actions"><button class="btn btn-sm" type="button" data-testid="platform-manual-update-open" @click="openManualUpdate">手动更新</button></span>
          </div>
          <div class="platform-release-config-row">
            <span class="platform-release-config-name">就绪副本</span>
            <span class="platform-release-config-value">{{ platform.deployment?.ready_replicas || 0 }} / {{ platform.deployment?.desired_replicas || 1 }}</span>
            <span class="settings-table-empty">-</span>
          </div>
          <div class="platform-release-config-row">
            <span class="platform-release-config-name">允许的镜像前缀</span>
            <code class="platform-release-config-value platform-release-prefix-value">{{ platform.image_prefix || '-' }}</code>
            <span class="platform-release-config-actions"><button class="btn btn-sm" type="button" data-testid="platform-image-prefix-open" @click="openImagePrefixEdit">修改</button></span>
          </div>
          <div class="platform-release-config-row">
            <span class="platform-release-config-name">Webhook 密钥</span>
            <span class="platform-release-config-value"><span class="badge" :class="platform.webhook_configured ? 'badge-online' : 'badge-offline'">{{ platform.webhook_configured ? '已配置' : '未配置' }}</span></span>
            <span class="platform-release-config-actions"><button class="btn btn-sm btn-primary" type="button" data-testid="generate-platform-webhook-secret" @click="openWebhookSecret">生成或轮换</button></span>
          </div>
        </div>
        <p v-if="platformActionMessage" class="settings-copy platform-action-message">{{ platformActionMessage }}</p>
        <p v-if="platformReadError" class="settings-copy endpoint-error">{{ platformReadError }}</p>
      </SurfaceCard>

      <SurfaceCard class="platform-release-history-card">
        <section v-if="platform.releases?.length" class="platform-release-history-table" aria-label="发布记录">
          <header class="platform-release-history-header">
            <span>发布方式</span>
            <span>镜像 / Tag</span>
            <span>状态</span>
            <span>发布时间</span>
            <span>操作</span>
          </header>
          <div v-for="release in platform.releases.slice(0, 5)" :key="release.id" class="platform-release-history-row">
            <span>{{ release.source === 'github' ? 'GitHub' : release.source || '-' }}</span>
            <span class="platform-release-history-image"><code>{{ release.image || '-' }}</code><small v-if="release.commit_sha">提交 {{ release.commit_sha.slice(0, 12) }}</small></span>
            <span><span class="badge" :class="release.status === 'succeeded' ? 'badge-online' : release.status === 'failed' ? 'badge-danger' : 'badge-deploying'">{{ release.status || '-' }}</span></span>
            <time :datetime="release.created_at">{{ formatDateTime(release.created_at) }}</time>
            <span class="platform-release-history-actions"><button class="btn btn-sm" :disabled="rollingBack === release.id || !release.previous_image" @click="rollbackPlatformRelease(release)">
              {{ rollingBack === release.id ? '提交中...' : '回滚' }}
            </button></span>
          </div>
        </section>
        <p v-else class="settings-copy platform-release-history-empty">尚无发布记录。</p>
      </SurfaceCard>
    </div>

    <BaseModal :open="showManualUpdate" title="手动更新镜像" size="small" :show-close="!updatingPlatform" :close-on-overlay="!updatingPlatform" :close-on-escape="!updatingPlatform" @close="showManualUpdate = false">
      <form id="platform-manual-update-form" class="platform-release-form" @submit.prevent="manualPlatformUpdate">
        <div class="form-group">
          <label class="form-label" for="platform-manual-image">镜像 Tag</label>
          <input id="platform-manual-image" v-model.trim="manualImage" data-testid="platform-manual-image" class="form-input" :placeholder="`${platform.image_prefix || 'registry.example.com/cylism-manager'}:latest`" :disabled="updatingPlatform" required />
          <p class="settings-copy form-hint">仅支持允许仓库前缀下的镜像 Tag。</p>
        </div>
      </form>
      <template #actions>
        <button class="btn btn-sm" type="button" :disabled="updatingPlatform" @click="showManualUpdate = false">取消</button>
        <button class="btn btn-sm btn-primary" type="button" data-testid="platform-manual-update" :disabled="updatingPlatform || !manualImage" @click="manualPlatformUpdate">{{ updatingPlatform ? '提交中...' : '手动更新' }}</button>
      </template>
    </BaseModal>

    <BaseModal :open="showImagePrefixEdit" title="允许的镜像前缀" size="medium" :show-close="!savingPrefix" :close-on-overlay="!savingPrefix" :close-on-escape="!savingPrefix" @close="showImagePrefixEdit = false">
      <form id="platform-image-prefix-form" class="platform-release-form" @submit.prevent="savePlatformImagePrefix">
        <div class="form-group">
          <label class="form-label" for="platform-image-prefix">镜像前缀</label>
          <textarea id="platform-image-prefix" v-model.trim="platformImagePrefix" data-testid="platform-image-prefix" class="form-input platform-prefix-input" rows="5" :disabled="savingPrefix" placeholder="每行一个前缀，例如 registry.example.com/cylism-manager" @input="platformPrefixDirty = true" />
        </div>
      </form>
      <template #actions>
        <button class="btn btn-sm" type="button" :disabled="savingPrefix" @click="showImagePrefixEdit = false">取消</button>
        <button class="btn btn-sm btn-primary" type="button" data-testid="platform-image-prefix-save" :disabled="savingPrefix || !platformImagePrefix" @click="savePlatformImagePrefix">{{ savingPrefix ? '保存中...' : '保存' }}</button>
      </template>
    </BaseModal>

    <BaseModal :open="showWebhookSecret" :title="generatedSecret ? 'Webhook 密钥已生成' : '生成或轮换 Webhook 密钥'" size="small" :show-close="!generatingSecret" :close-on-overlay="!generatingSecret" :close-on-escape="!generatingSecret" @close="closeWebhookSecret">
      <div v-if="generatedSecret" class="secret-once">
        <strong>仅显示一次，请立即复制</strong>
        <code data-testid="platform-webhook-generated-secret">{{ generatedSecret }}</code>
      </div>
      <p v-else class="settings-copy">生成新密钥后，现有 Webhook 请求需要改用新密钥。</p>
      <template #actions>
        <template v-if="generatedSecret">
          <button class="btn btn-sm" type="button" @click="copyGeneratedSecret">复制</button>
          <button class="btn btn-sm btn-primary" type="button" @click="closeWebhookSecret">完成</button>
        </template>
        <template v-else>
          <button class="btn btn-sm" type="button" :disabled="generatingSecret" @click="closeWebhookSecret">取消</button>
          <button class="btn btn-sm btn-primary" type="button" data-testid="platform-webhook-generate-confirm" :disabled="generatingSecret" @click="generatePlatformWebhookSecret">{{ generatingSecret ? '生成中...' : '生成或轮换' }}</button>
        </template>
      </template>
    </BaseModal>

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
import { onMounted, ref } from 'vue'
import { createPlatformRelease, generatePlatformWebhookSecret as generatePlatformWebhookSecretRequest, getPlatformStatus, rollbackPlatformRelease as rollbackPlatformReleaseRequest, updatePlatformImagePrefix } from '../../api/settings.js'
import { useActionState } from '../../composables/useActionState.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { formatDateTime } from '../../utils/formatters.js'
import BaseModal from '../../components/BaseModal.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const platform = ref({ webhook_configured: false, image_prefix: '', deployment: null, releases: [] })
const platformImagePrefix = ref('')
const manualImage = ref('')
const platformActionMessage = ref('')
const platformActionError = ref('')
const platformReadError = ref('')
const generatedSecret = ref('')
const rollingBack = ref(0)
const platformPrefixDirty = ref(false)
const showManualUpdate = ref(false)
const showImagePrefixEdit = ref(false)
const showWebhookSecret = ref(false)
const platformResource = useAsyncResource(({ signal }) => getPlatformStatus({ signal }), null)
const generateSecretAction = useActionState()
const savePrefixAction = useActionState()
const manualUpdateAction = useActionState()
const generatingSecret = generateSecretAction.running
const savingPrefix = savePrefixAction.running
const updatingPlatform = manualUpdateAction.running

onMounted(() => {
  void refresh({ syncForm: true })
})

async function refresh({ syncForm = false } = {}) {
  platformReadError.value = ''
  const result = await platformResource.refresh()
  if (!result) {
    if (platformResource.error.value) {
      platformReadError.value = platformResource.error.value.message || '读取平台发布状态失败'
    }
    return
  }
  platform.value = result
  if (syncForm || !platformPrefixDirty.value) {
    platformImagePrefix.value = platform.value.image_prefix || ''
    platformPrefixDirty.value = false
  }
}

async function generatePlatformWebhookSecret() {
  platformActionError.value = ''
  try {
    generatedSecret.value = (await generateSecretAction.run(() => generatePlatformWebhookSecretRequest())).secret
    await refresh()
  } catch (e) {
    platformActionError.value = e.message || '生成平台 Webhook 密钥失败'
  }
}

function openManualUpdate() {
  manualImage.value = ''
  showManualUpdate.value = true
}

function openImagePrefixEdit() {
  platformImagePrefix.value = platform.value.image_prefix || ''
  platformPrefixDirty.value = false
  showImagePrefixEdit.value = true
}

function openWebhookSecret() {
  generatedSecret.value = ''
  showWebhookSecret.value = true
}

function closeWebhookSecret() {
  if (generatingSecret.value) return
  showWebhookSecret.value = false
  generatedSecret.value = ''
}

async function savePlatformImagePrefix() {
  platformActionError.value = ''
  try {
    await savePrefixAction.run(() => updatePlatformImagePrefix({ image_prefix: platformImagePrefix.value }))
    platformPrefixDirty.value = false
    await refresh({ syncForm: true })
    showImagePrefixEdit.value = false
  } catch (e) {
    platformActionError.value = e.message || '保存镜像前缀失败'
  }
}

async function manualPlatformUpdate() {
  platformActionMessage.value = ''
  platformActionError.value = ''
  try {
    await manualUpdateAction.run(() => createPlatformRelease({ image: manualImage.value }))
    manualImage.value = ''
    platformActionMessage.value = '平台更新已提交'
    await refresh()
    showManualUpdate.value = false
  } catch (e) {
    platformActionError.value = e.message || '平台更新失败'
  }
}

async function copyGeneratedSecret() {
  if (generatedSecret.value) await navigator.clipboard?.writeText(generatedSecret.value)
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

defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
