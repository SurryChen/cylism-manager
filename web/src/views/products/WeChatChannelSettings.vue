<template>
  <BaseModal teleport :open="open" title="微信渠道接入" size="medium" :show-close="!busy && !disconnectOpen && !error" :close-on-overlay="!busy && !disconnectOpen && !error" :close-on-escape="!busy && !disconnectOpen && !error" @close="$emit('close')">
    <EmptyState v-if="loading" variant="loading" message="正在读取微信配置…" />
    <template v-else>
      <form id="wechat-channel-settings" class="product-form-grid" @submit.prevent="save">
        <label class="form-group product-full-width">
          <span class="form-label">微信发行渠道</span>
          <SelectMenu v-model="channelID" aria-label="微信发行渠道" :disabled="busy">
            <option v-for="channel in channels" :key="channel.id" :value="channel.id">{{ channel.name || (channel.platform === 'mini_game' ? '小游戏' : '微信小程序') }}</option>
          </SelectMenu>
        </label>
        <label class="form-group">
          <span class="form-label">AppID</span>
          <input v-model.trim="appID" class="form-input" aria-label="AppID" :readonly="Boolean(connection)" :disabled="busy" required maxlength="18" />
        </label>
        <label class="form-group">
          <span class="form-label">AppSecret{{ connection?.analytics_connected ? '（留空保留）' : '（运营数据）' }}</span>
          <input v-model="secret" class="form-input" aria-label="AppSecret" type="password" autocomplete="new-password" :disabled="busy" maxlength="256" />
        </label>
      </form>

      <div class="wechat-subsection">
        <div class="wechat-heading">
          <strong>版本与审核同步</strong>
          <span class="badge" :class="connection?.release_connected ? 'badge-online' : 'badge-offline'">{{ connection?.release_connected ? '已授权' : '未授权' }}</span>
        </div>
        <p v-if="!settings?.component_available" class="cell-secondary">需先由平台管理员配置微信开放平台第三方应用。</p>
        <template v-else>
          <div class="wechat-actions">
            <button type="button" class="btn btn-sm" :disabled="busy || !connection" @click="authorize">{{ connection?.release_connected ? '重新授权' : '扫码授权' }}</button>
            <button v-if="connection?.release_connected" type="button" class="btn btn-sm" :disabled="busy" @click="sync">立即同步</button>
          </div>
          <dl v-if="connection?.release_checked_at" class="wechat-values">
            <div><dt>线上版本</dt><dd>{{ connection.online_version || '—' }}</dd></div>
            <div><dt>线上发布时间</dt><dd>{{ connection.online_at ? time(connection.online_at) : '—' }}</dd></div>
            <div><dt>体验版</dt><dd>{{ connection.trial_version || '—' }}</dd></div>
            <div><dt>最近审核</dt><dd>{{ auditLabel }}</dd></div>
            <div><dt>最近检查</dt><dd>{{ time(connection.release_checked_at) }}</dd></div>
          </dl>
          <p v-if="connection?.release_error" class="cell-secondary" role="status">{{ connection.release_error }}</p>
          <p v-if="connection?.audit_error" class="cell-secondary" role="status">{{ connection.audit_error }}</p>
          <p v-if="connection?.audit_reason" class="cell-secondary">{{ connection.audit_reason }}</p>
        </template>
      </div>

      <div v-if="connection?.analytics_connected" class="wechat-subsection">
        <strong>推广码</strong>
        <form class="wechat-actions" @submit.prevent="generate">
          <input v-model.trim="scene" class="form-input" aria-label="推广场景" placeholder="例如 poster-01" required maxlength="32" :disabled="busy" />
          <button type="submit" class="btn btn-sm" :disabled="busy || !scene">生成微信码</button>
        </form>
        <figure v-if="code" class="wechat-code">
          <img :src="code" alt="微信推广码" width="150" height="150" />
          <a :href="code" :download="codeFilename" class="btn btn-sm">下载微信码</a>
        </figure>
      </div>
    </template>
    <template #actions>
      <button v-if="connection" type="button" class="btn btn-danger" :disabled="busy" @click="disconnectOpen = true">断开连接</button>
      <button type="button" class="btn" :disabled="busy" @click="$emit('close')">关闭</button>
      <button type="submit" form="wechat-channel-settings" class="btn btn-primary" :disabled="busy || loading || !channelID">保存连接</button>
    </template>
  </BaseModal>
  <ConfirmDialog teleport :open="disconnectOpen" title="断开微信连接" message="断开后会停止同步并清理本地缓存的运营数据。项目版本和发行记录会保留。" :busy="busy" confirm-text="断开连接" @cancel="disconnectOpen = false" @confirm="disconnect" />
  <ErrorNoticeModal teleport :open="Boolean(error)" :message="error" @close="error = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { authorizeWeChatRelease, deleteWeChatConnection, generateWeChatCode, getWeChatSettings, saveWeChatConnection, syncWeChatRelease, weChatChannels } from '../../api/wechat.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const props = defineProps({ open: Boolean, product: { type: Object, required: true }, authorizationReturn: Object })
const emit = defineEmits(['close', 'changed'])
const channels = computed(() => weChatChannels(props.product))
const channelID = ref(0)
const appID = ref('')
const secret = ref('')
const settings = ref(null)
const error = ref('')
const loading = ref(false)
const busy = ref(false)
const disconnectOpen = ref(false)
const scene = ref('')
const code = ref('')
const codeFilename = ref('')
let handledAuthorization
const connection = computed(() => settings.value?.connections.find(item => item.channel_id === channelID.value))
const auditLabel = computed(() => ({ 0: '审核通过', 1: '审核未通过', 2: '审核中', 3: '已撤回', 4: '审核延后' })[connection.value?.audit_status] || '—')
const time = value => new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))

watch([channelID, connection], () => {
  const channel = channels.value.find(item => item.id === channelID.value)
  appID.value = connection.value?.app_id || channel?.app_identifier || ''
  secret.value = ''
  scene.value = ''
  code.value = ''
})
watch([() => props.open, () => props.authorizationReturn], async ([open, authorization], _, onCleanup) => {
  if (!open) { secret.value = ''; return }
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  settings.value = null
  const returned = authorization && authorization !== handledAuthorization ? authorization : null
  channelID.value = channels.value.find(item => item.id === returned?.channelID)?.id || channels.value[0]?.id || 0
  loading.value = true
  try {
    settings.value = await getWeChatSettings(props.product.id, { signal: controller.signal })
    if (controller.signal.aborted || !returned) return
    handledAuthorization = returned
    if (returned.status === 'missing-scope') error.value = '微信未授予版本管理权限（权限集 18），请检查开放平台权限后重新授权。'
    else if (returned.status === 'failed') error.value = '微信授权未完成，请重新扫码授权。'
    else if (connection.value?.release_connected) await sync()
  }
  catch (cause) { if (cause.name !== 'AbortError') error.value = cause.message }
  finally { if (!controller.signal.aborted) loading.value = false }
}, { immediate: true })

async function save() {
  busy.value = true
  try {
    await saveWeChatConnection(props.product.id, channelID.value, { app_id: appID.value, app_secret: secret.value })
    secret.value = ''
    settings.value = await getWeChatSettings(props.product.id)
    emit('changed')
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}

async function authorize() {
  busy.value = true
  try {
    const result = await authorizeWeChatRelease(props.product.id, channelID.value)
    const url = new URL(result.url)
    if (url.origin !== 'https://mp.weixin.qq.com' || url.pathname !== '/cgi-bin/componentloginpage') throw new Error('微信授权地址无效')
    window.location.assign(url.href)
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}

async function sync() {
  busy.value = true
  try {
    settings.value = await syncWeChatRelease(props.product.id, channelID.value)
    emit('changed')
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}

async function disconnect() {
  busy.value = true
  try {
    await deleteWeChatConnection(props.product.id, channelID.value)
    disconnectOpen.value = false
    settings.value = await getWeChatSettings(props.product.id)
    emit('changed')
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}

async function generate() {
  busy.value = true
  try {
    const result = await generateWeChatCode(props.product.id, channelID.value, scene.value)
    code.value = `data:${result.mime};base64,${result.image}`
    const label = scene.value.replace(/[^a-zA-Z0-9_-]/g, '-')
    codeFilename.value = `wechat-${label}.${result.mime === 'image/png' ? 'png' : 'jpg'}`
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}
</script>

<style scoped>
.wechat-subsection { margin-top: var(--space-16); padding-top: var(--space-12); border-top: 1px solid var(--border-muted); }
.wechat-heading, .wechat-actions { display: flex; align-items: center; gap: var(--space-8); flex-wrap: wrap; }
.wechat-actions { margin-top: var(--space-8); }
.wechat-actions .form-input { width: min(100%, 220px); }
.wechat-values { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-8); margin-bottom: 0; }
.wechat-values dd { margin: var(--space-4) 0 0; overflow-wrap: anywhere; }
.wechat-values dt { color: var(--text-muted); font-size: 12px; }
.wechat-code { display: flex; align-items: center; gap: var(--space-12); margin: var(--space-12) 0 0; }
</style>
