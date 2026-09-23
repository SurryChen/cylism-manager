<template>
  <section class="settings-section">
    <SurfaceCard class="temporary-token-card">
      <div class="temporary-token-toolbar">
        <button class="btn btn-primary" type="button" data-testid="temporary-token-open-create" @click="openTemporaryTokenCreate">
          生成临时秘钥
        </button>
      </div>
      <section v-if="temporaryTokens.length" class="temporary-token-table" aria-label="临时登录秘钥列表">
        <header class="temporary-token-table-header">
          <span>备注</span>
          <span>创建时间</span>
          <span>到期时间</span>
          <span>生效状态</span>
          <span>操作</span>
        </header>
        <div v-for="item in temporaryTokens" :key="item.id" class="temporary-token-row">
          <strong class="temporary-token-note">{{ item.label || '未命名秘钥' }}</strong>
          <time class="temporary-token-date" :datetime="item.created_at">{{ formatDateTime(item.created_at) }}</time>
          <time class="temporary-token-date" :datetime="item.expires_at">{{ formatDateTime(item.expires_at) }}</time>
          <span class="badge" :class="item.status === 'active' ? 'badge-online' : item.status === 'expired' ? 'badge-offline' : 'badge-danger'">
            {{ item.status === 'active' ? '生效中' : item.status === 'expired' ? '已过期' : '已撤销' }}
          </span>
          <span class="temporary-token-action">
            <button class="btn btn-sm btn-danger" :disabled="item.status !== 'active' || revokingTemporaryToken === item.id" @click="revokeTemporaryToken(item)">
              {{ revokingTemporaryToken === item.id ? '撤销中...' : '撤销' }}
            </button>
          </span>
        </div>
      </section>
      <p v-if="temporaryTokensError" class="settings-copy endpoint-error">{{ temporaryTokensError }}</p>
      <p v-else-if="temporaryTokens.length === 0" class="settings-copy temporary-token-empty">尚未生成临时登录秘钥。</p>
    </SurfaceCard>

    <BaseModal
      :open="showTemporaryTokenCreate"
      :title="generatedTemporaryToken ? '临时秘钥已生成' : '生成临时秘钥'"
      size="small"
      :show-close="!creatingTemporaryToken"
      :close-on-overlay="!creatingTemporaryToken"
      :close-on-escape="!creatingTemporaryToken"
      dialog-class="temporary-token-create-modal"
      data-testid="temporary-token-create-modal"
      @close="closeTemporaryTokenCreate"
    >
      <div v-if="generatedTemporaryToken" class="secret-once temporary-token-secret">
        <strong>仅显示一次，请立即复制</strong>
        <code data-testid="temporary-token-generated-secret">{{ generatedTemporaryToken }}</code>
      </div>
      <form v-else id="temporary-token-create-form" class="temporary-token-create-form" @submit.prevent="createTemporaryToken">
        <div class="form-group">
          <label class="form-label" for="temporary-token-label">备注</label>
          <input id="temporary-token-label" v-model.trim="temporaryTokenForm.label" class="form-input" placeholder="例如：供应商临时访问" />
        </div>
        <div class="form-group">
          <label class="form-label" for="temporary-token-ttl">有效期</label>
          <SelectMenu id="temporary-token-ttl" v-model.number="temporaryTokenForm.ttl_seconds" class="form-select">
            <option :value="3600">1 小时</option>
            <option :value="21600">6 小时</option>
            <option :value="86400">1 天</option>
            <option :value="604800">7 天</option>
          </SelectMenu>
        </div>
        <p v-if="temporaryTokenCreateError" class="settings-copy endpoint-error">{{ temporaryTokenCreateError }}</p>
      </form>
      <template #actions>
        <template v-if="generatedTemporaryToken">
          <button class="btn btn-sm" type="button" @click="copyTemporaryToken">复制</button>
          <button class="btn btn-sm btn-primary" type="button" data-testid="temporary-token-create-close" @click="closeTemporaryTokenCreate">完成</button>
        </template>
        <template v-else>
          <button class="btn btn-sm" type="button" :disabled="creatingTemporaryToken" @click="closeTemporaryTokenCreate">取消</button>
          <button class="btn btn-sm btn-primary" type="button" data-testid="temporary-token-create-submit" :disabled="creatingTemporaryToken" @click="createTemporaryToken">
            {{ creatingTemporaryToken ? '生成中...' : '生成临时秘钥' }}
          </button>
        </template>
      </template>
    </BaseModal>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { createTemporaryToken as createTemporaryTokenRequest, deleteTemporaryToken, getTemporaryTokens } from '../../api/settings.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { formatDateTime } from '../../utils/formatters.js'
import BaseModal from '../../components/BaseModal.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const temporaryTokens = ref([])
const generatedTemporaryToken = ref('')
const temporaryTokensError = ref('')
const temporaryTokenCreateError = ref('')
const creatingTemporaryToken = ref(false)
const revokingTemporaryToken = ref(0)
const temporaryTokenForm = ref({ label: '', ttl_seconds: 3600 })
const showTemporaryTokenCreate = ref(false)
const temporaryTokensResource = useAsyncResource(({ signal }) => getTemporaryTokens({ signal }), [])

onMounted(() => {
  void refresh()
})

async function refresh() {
  temporaryTokensError.value = ''
  const result = await temporaryTokensResource.refresh()
  if (result) temporaryTokens.value = result || []
  else if (temporaryTokensResource.error.value) temporaryTokensError.value = temporaryTokensResource.error.value.message || '读取临时登录秘钥失败'
}

async function createTemporaryToken() {
  creatingTemporaryToken.value = true
  temporaryTokenCreateError.value = ''
  try {
    const result = await createTemporaryTokenRequest({ ...temporaryTokenForm.value })
    generatedTemporaryToken.value = result.token
    temporaryTokenForm.value.label = ''
    await refresh()
  } catch (e) {
    temporaryTokenCreateError.value = e.message || '生成临时登录秘钥失败'
  } finally {
    creatingTemporaryToken.value = false
  }
}

function openTemporaryTokenCreate() {
  generatedTemporaryToken.value = ''
  temporaryTokenCreateError.value = ''
  temporaryTokenForm.value = { label: '', ttl_seconds: 3600 }
  showTemporaryTokenCreate.value = true
}

function closeTemporaryTokenCreate() {
  if (creatingTemporaryToken.value) return
  showTemporaryTokenCreate.value = false
  generatedTemporaryToken.value = ''
  temporaryTokenCreateError.value = ''
  temporaryTokenForm.value = { label: '', ttl_seconds: 3600 }
}

async function revokeTemporaryToken(item) {
  revokingTemporaryToken.value = item.id
  temporaryTokensError.value = ''
  try {
    await deleteTemporaryToken(item.id)
    await refresh()
  } catch (e) {
    temporaryTokensError.value = e.message || '撤销临时登录秘钥失败'
  } finally {
    revokingTemporaryToken.value = 0
  }
}

async function copyTemporaryToken() {
  if (generatedTemporaryToken.value) await navigator.clipboard?.writeText(generatedTemporaryToken.value)
}

defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
