<template>
  <section class="settings-section">
    <SurfaceCard padding="none">
      <div class="github-toolbar">
        <div v-if="connection">
          <strong>{{ connection.slug }}</strong>
          <div class="cell-secondary">App ID {{ connection.app_id }}</div>
        </div>
        <div class="github-actions">
          <button v-if="connection" class="btn" type="button" :disabled="loading || busy" @click="refresh">刷新</button>
          <button v-if="!connection" class="btn btn-primary" type="button" :disabled="loading || busy" @click="openConnect">连接 GitHub</button>
          <button v-if="connection" class="btn" type="button" :disabled="loading || busy" @click="disconnectOpen = true">断开连接</button>
          <a v-if="connection" class="btn btn-primary" :href="`https://github.com/apps/${encodeURIComponent(connection.slug)}/installations/new`">安装 GitHub App</a>
        </div>
      </div>
      <EmptyState v-if="loading" variant="loading" message="正在读取 GitHub 安装" />
      <EmptyState v-else-if="!connection" message="尚未连接 GitHub App" />
      <EmptyState v-else-if="!installations.length" message="请在 GitHub 安装 App 并选择要授权的仓库" />
      <div v-else class="table-wrap">
        <DataTable aria-label="GitHub App 安装">
          <thead><tr><th>GitHub 账号</th><th>仓库范围</th><th>安装权限</th><th>状态</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="item in installations" :key="item.id">
              <td>{{ item.account.login }}<div class="cell-secondary">#{{ item.id }}</div></td>
              <td>{{ item.repository_selection === 'all' ? '全部仓库' : '选定仓库' }}</td>
              <td>{{ permissionText(item.permissions) || '—' }}</td>
              <td><span class="badge" :class="item.suspended_at ? 'badge-offline' : 'badge-online'">{{ item.suspended_at ? '已暂停' : '已授权' }}</span></td>
              <TableActionCell><a class="btn btn-sm" :href="installationURL(item)" target="_blank" rel="noopener noreferrer">管理授权</a></TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
      <div v-if="hasMore" class="github-toolbar">
        <button class="btn btn-sm" type="button" :disabled="loading" @click="loadMore">加载更多</button>
      </div>
    </SurfaceCard>
    <BaseModal teleport :open="connectOpen" title="连接 GitHub" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="connectOpen = false">
      <form id="github-connection-form" @submit.prevent="connect">
        <label class="form-group">
          <span class="form-label">GitHub App 所属账号</span>
          <SelectMenu v-model="accountType" aria-label="GitHub App 所属账号" :disabled="busy">
            <option value="personal">个人账号</option>
            <option value="organization">组织</option>
          </SelectMenu>
        </label>
        <label v-if="accountType === 'organization'" class="form-group">
          <span class="form-label">GitHub 组织名</span>
          <input v-model.trim="organization" class="form-input" placeholder="例如 cylism" maxlength="39" required :disabled="busy" />
        </label>
        <p class="form-hint">GitHub 将创建私有 App，安装时可选择全部仓库或指定仓库。App 会请求 Contents、Actions 和 Pull requests 的读写权限，供后续集成使用。</p>
      </form>
      <template #actions>
        <button class="btn" type="button" :disabled="busy" @click="connectOpen = false">取消</button>
        <button class="btn btn-primary" form="github-connection-form" type="submit" :disabled="busy">前往 GitHub</button>
      </template>
    </BaseModal>
    <form ref="manifestForm" method="post" :action="registration?.action" hidden>
      <input type="hidden" name="manifest" :value="registration ? JSON.stringify(registration.manifest) : ''" />
    </form>
    <ConfirmDialog teleport :open="disconnectOpen" title="断开 GitHub App" message="Cylism Manager 将移除当前凭据。仓库授权可在 GitHub 的 App 安装设置中管理。" confirm-text="断开连接" :busy="busy" @cancel="disconnectOpen = false" @confirm="disconnect" />
    <ErrorNoticeModal teleport :open="Boolean(error)" :message="error" @close="error = ''" />
  </section>
</template>

<script setup>
import { nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import DataTable from '../../components/DataTable.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { deleteGitHubConnection, getGitHubConnection, getGitHubInstallations, startGitHubRegistration } from '../../api/github.js'

const route = useRoute()
const router = useRouter()
const connection = ref(null)
const installations = ref([])
const page = ref(0)
const hasMore = ref(false)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const connectOpen = ref(false)
const disconnectOpen = ref(false)
const accountType = ref('personal')
const organization = ref('')
const registration = ref(null)
const manifestForm = ref(null)
const permissionText = permissions => Object.entries(permissions || {}).map(([key, value]) => `${key}: ${value}`).join(' · ')
function installationURL(item) {
  const account = item.account.type === 'Organization' ? `/organizations/${encodeURIComponent(item.account.login)}` : ''
  return `https://github.com${account}/settings/installations/${item.id}`
}

async function fetchPage(nextPage) {
  const result = await getGitHubInstallations(nextPage)
  installations.value = nextPage === 1 ? result.items : [...installations.value, ...result.items]
  page.value = nextPage
  hasMore.value = result.has_more
}

async function refresh() {
  if (loading.value) return
  loading.value = true
  try {
    connection.value = await getGitHubConnection()
    installations.value = []
    hasMore.value = false
    if (connection.value) await fetchPage(1)
  } catch (cause) {
    error.value = cause.message
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  loading.value = true
  try {
    await fetchPage(page.value + 1)
  } catch (cause) {
    error.value = cause.message
  } finally {
    loading.value = false
  }
}

function openConnect() {
  accountType.value = 'personal'
  organization.value = ''
  connectOpen.value = true
}

async function connect() {
  if (busy.value) return
  busy.value = true
  try {
    const payload = { organization: accountType.value === 'organization' ? organization.value : '' }
    registration.value = await startGitHubRegistration(payload)
    await nextTick()
    manifestForm.value?.submit()
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

async function disconnect() {
  busy.value = true
  try {
    await deleteGitHubConnection()
    disconnectOpen.value = false
    await refresh()
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

onMounted(refresh)
watch(() => route.query.github_setup, status => {
  if (!status) return
  const messages = {
    expired: 'GitHub 连接请求已过期。请在同一浏览器中重新发起连接。',
    owner: 'GitHub App 的所属账号与所选账号不一致，请重新连接。',
    failed: 'GitHub App 配置接收失败，请重新连接。',
  }
  error.value = typeof status === 'string' && Object.hasOwn(messages, status) ? messages[status] : messages.failed
  const query = { ...route.query }
  delete query.github_setup
  void router.replace({ query })
}, { immediate: true })
defineExpose({ refresh })
</script>

<style scoped>
.github-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-8); padding: var(--space-12) var(--space-20); }
.github-actions { display: flex; flex-wrap: wrap; gap: var(--space-8); margin-inline-start: auto; }
</style>
