<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <h2 class="section-title">安全与访问</h2>
        <p class="settings-copy">临时登录秘钥用于短期共享访问。</p>
      </div>
    </div>

    <section class="card temporary-token-card">
      <div class="card-header temporary-token-header">
        <div>
          <h2 class="card-title">临时登录秘钥</h2>
          <p class="settings-copy">
            生成可分享给其他人的临时登录凭据。秘钥只在生成时显示一次，可随时撤销。
          </p>
        </div>
        <span class="badge temporary-token-badge">安全管理</span>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label class="form-label" for="temporary-token-label">备注</label>
          <input id="temporary-token-label" v-model.trim="temporaryTokenForm.label" class="form-input" placeholder="例如：供应商临时访问" />
        </div>
        <div class="form-group">
          <label class="form-label" for="temporary-token-ttl">有效期</label>
          <select id="temporary-token-ttl" v-model.number="temporaryTokenForm.ttl_seconds" class="form-select">
            <option :value="3600">1 小时</option>
            <option :value="21600">6 小时</option>
            <option :value="86400">1 天</option>
            <option :value="604800">7 天</option>
          </select>
        </div>
      </div>
      <div class="settings-action-row">
        <button class="btn btn-sm btn-primary" :disabled="creatingTemporaryToken" @click="createTemporaryToken">
          {{ creatingTemporaryToken ? '生成中...' : '生成临时秘钥' }}
        </button>
      </div>
      <div v-if="generatedTemporaryToken" class="secret-once temporary-token-secret">
        <strong>仅显示一次，请立即复制</strong>
        <code>{{ generatedTemporaryToken }}</code>
        <button class="btn btn-sm" @click="copyTemporaryToken">复制</button>
      </div>
      <div v-if="temporaryTokens.length" class="temporary-token-list">
        <div v-for="item in temporaryTokens" :key="item.id" class="temporary-token-row">
          <div>
            <strong>{{ item.label || '未命名秘钥' }}</strong>
            <small>创建于 {{ formatDateTime(item.created_at) }} · 到期 {{ formatDateTime(item.expires_at) }}</small>
            <span class="badge" :class="item.status === 'active' ? 'badge-online' : item.status === 'expired' ? 'badge-offline' : 'badge-danger'">
              {{ item.status === 'active' ? '生效中' : item.status === 'expired' ? '已过期' : '已撤销' }}
            </span>
          </div>
          <button v-if="item.status === 'active'" class="btn btn-sm btn-danger" :disabled="revokingTemporaryToken === item.id" @click="revokeTemporaryToken(item)">
            {{ revokingTemporaryToken === item.id ? '撤销中...' : '撤销' }}
          </button>
        </div>
      </div>
      <p v-if="temporaryTokensError" class="settings-copy endpoint-error">{{ temporaryTokensError }}</p>
      <p v-else class="settings-copy">尚未生成临时登录秘钥。</p>
    </section>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { createTemporaryToken as createTemporaryTokenRequest, deleteTemporaryToken, getTemporaryTokens } from '../api/settings.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const temporaryTokens = ref([])
const generatedTemporaryToken = ref('')
const temporaryTokensError = ref('')
const creatingTemporaryToken = ref(false)
const revokingTemporaryToken = ref(0)
const temporaryTokenForm = ref({ label: '', ttl_seconds: 3600 })
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
  temporaryTokensError.value = ''
  try {
    const result = await createTemporaryTokenRequest(temporaryTokenForm.value)
    generatedTemporaryToken.value = result.token
    temporaryTokenForm.value.label = ''
    await refresh()
  } catch (e) {
    temporaryTokensError.value = e.message || '生成临时登录秘钥失败'
  } finally {
    creatingTemporaryToken.value = false
  }
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

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString('zh-CN', { hour12: false })
}

defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
