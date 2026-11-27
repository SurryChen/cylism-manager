<template>
  <BaseModal teleport :open="open" title="微信运营与推广" size="medium" :show-close="!busy && !disconnectOpen && !error" :close-on-overlay="!busy && !disconnectOpen && !error" :close-on-escape="!busy && !disconnectOpen && !error" @close="$emit('close')">
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
          <span class="form-label">AppSecret{{ connection?.analytics_connected ? '（留空保留）' : '' }}</span>
          <input v-model="secret" class="form-input" aria-label="AppSecret" type="password" autocomplete="new-password" :required="!connection?.analytics_connected" :disabled="busy" maxlength="256" />
        </label>
        <p v-if="!connection?.analytics_connected" class="product-full-width cell-secondary wechat-help">
          在<a href="https://mp.weixin.qq.com/" target="_blank" rel="noopener noreferrer">微信公众平台</a>的「开发管理 → 开发设置」获取应用自己的 AppID 和 AppSecret。保存后在项目总览查看运营数据。
        </p>
      </form>

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
      <button type="submit" form="wechat-channel-settings" class="btn btn-primary" :disabled="busy || !settings || !channelID">保存连接</button>
    </template>
  </BaseModal>
  <ConfirmDialog teleport :open="disconnectOpen" title="断开微信连接" message="断开后会停止查询微信运营数据，并清理此渠道在 Manager 的凭据和数据缓存。" :busy="busy" confirm-text="断开连接" @cancel="disconnectOpen = false" @confirm="disconnect" />
  <ErrorNoticeModal teleport :open="Boolean(error)" :message="error" @close="error = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { deleteWeChatConnection, generateWeChatCode, getWeChatSettings, saveWeChatConnection, weChatChannels } from '../../api/wechat.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const props = defineProps({ open: Boolean, product: { type: Object, required: true } })
const emit = defineEmits(['close'])
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
const connection = computed(() => settings.value?.connections.find(item => item.channel_id === channelID.value))

watch([channelID, connection], () => {
  const channel = channels.value.find(item => item.id === channelID.value)
  appID.value = connection.value?.app_id || channel?.app_identifier || ''
  secret.value = ''
  scene.value = ''
  code.value = ''
})
watch(() => props.open, async (open, _, onCleanup) => {
  if (!open) { secret.value = ''; return }
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  settings.value = null
  channelID.value = channels.value[0]?.id || 0
  loading.value = true
  try {
    const result = await getWeChatSettings(props.product.id, { signal: controller.signal })
    if (!controller.signal.aborted) settings.value = result
  }
  catch (cause) { if (cause.name !== 'AbortError') error.value = cause.message }
  finally { if (!controller.signal.aborted) loading.value = false }
}, { immediate: true })

async function save() {
  if (busy.value || !settings.value || !channelID.value || (!secret.value.trim() && !connection.value?.analytics_connected)) return
  busy.value = true
  try {
    await saveWeChatConnection(props.product.id, channelID.value, { app_id: appID.value, app_secret: secret.value })
    secret.value = ''
    settings.value = await getWeChatSettings(props.product.id)
  } catch (cause) { error.value = cause.message }
  finally { busy.value = false }
}

async function disconnect() {
  busy.value = true
  try {
    await deleteWeChatConnection(props.product.id, channelID.value)
    disconnectOpen.value = false
    settings.value = await getWeChatSettings(props.product.id)
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
.wechat-help { margin: 0; }
.wechat-actions { display: flex; align-items: center; gap: var(--space-8); flex-wrap: wrap; }
.wechat-actions { margin-top: var(--space-8); }
.wechat-actions .form-input { width: min(100%, 220px); }
.wechat-code { display: flex; align-items: center; gap: var(--space-12); margin: var(--space-12) 0 0; }
</style>
