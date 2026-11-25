<template>
  <BaseModal teleport :open="open" title="GitHub 仓库" size="large" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
    <EmptyState v-if="loading" variant="loading" message="正在读取 GitHub 仓库绑定" />
    <template v-else>
      <div v-if="bindings.length" class="table-wrap">
        <DataTable aria-label="已绑定的 GitHub 仓库">
          <thead><tr><th>发布端</th><th>组件</th><th>仓库</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="binding in bindings" :key="binding.id">
              <td>{{ releaseUnitName(units, binding.release_unit_id) }}</td>
              <td>{{ binding.component }}</td>
              <td>{{ binding.repository }}</td>
              <TableActionCell>
                <button class="btn btn-sm" type="button" :disabled="busy" @click="removing = binding">解除绑定</button>
              </TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
      <EmptyState v-else message="项目尚未绑定 GitHub 仓库" />
      <p v-if="!connection"><RouterLink to="/settings/system?tab=github" @click="$emit('close')">前往系统设置连接 GitHub App</RouterLink></p>
      <form v-else-if="installations.length" id="product-github-binding" class="product-form-grid binding-form" @submit.prevent="save">
        <label class="form-group">
          <span class="form-label">发布端</span>
          <SelectMenu v-model="form.release_unit_id" aria-label="仓库发布端" :disabled="busy">
            <option :value="0">项目默认</option>
            <option v-for="unit in units" :key="unit.id" :value="unit.id">{{ unit.name }}</option>
          </SelectMenu>
        </label>
        <label class="form-group"><span class="form-label">组件名称</span><input v-model.trim="form.component" class="form-input" maxlength="128" placeholder="例如 API、管理台" required :disabled="busy" /></label>
        <label class="form-group">
          <span class="form-label">GitHub 账号</span>
          <SelectMenu v-model="form.installation_id" :options="installationOptions" aria-label="GitHub 安装账号" :disabled="busy" placeholder="选择安装账号" />
        </label>
        <label class="form-group">
          <span class="form-label">仓库</span>
          <SelectMenu v-model="form.repository_id" v-model:search.trim="keyword" searchable :options="repositoryOptions" aria-label="GitHub 仓库" search-label="搜索仓库" :disabled="!form.installation_id || busy" @load-more="loadMore">
            <template #feedback>
              <span v-if="repoLoading" class="cell-secondary">正在读取仓库</span>
              <span v-else-if="!repositoryOptions.length" class="cell-secondary">没有匹配的仓库</span>
              <button v-if="hasMore" class="btn btn-sm" type="button" :disabled="repoLoading" @click="loadMore">加载更多</button>
            </template>
          </SelectMenu>
        </label>
      </form>
      <p v-else><RouterLink to="/settings/system?tab=github" @click="$emit('close')">前往系统设置安装 GitHub App 并授权仓库</RouterLink></p>
    </template>
    <template #actions>
      <button class="btn" type="button" :disabled="busy" @click="$emit('close')">关闭</button>
      <button v-if="installations.length" class="btn btn-primary" type="submit" form="product-github-binding" :disabled="busy || loading || !form.component || !form.repository_id">绑定仓库</button>
    </template>
  </BaseModal>
  <ConfirmDialog teleport :open="Boolean(removing)" title="解除仓库绑定" :message="`解除 ${removing?.component || ''} 的仓库绑定？已录入的版本仍可查看其来源。`" confirm-text="解除绑定" :busy="busy" @cancel="removing = null" @confirm="remove" />
  <ErrorNoticeModal teleport :open="Boolean(error)" :message="error" @close="error = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import DataTable from '../../components/DataTable.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { bindGitHubRepository, getGitHubBindings, getGitHubConnection, getGitHubInstallations, getInstallationRepositories, unbindGitHubRepository } from '../../api/github.js'
import { releaseUnitName } from './lifecycle-utils.js'

const props = defineProps({ open: Boolean, productId: { type: Number, required: true }, units: { type: Array, required: true } })
defineEmits(['close'])
const bindings = ref([])
const connection = ref(null)
const installations = ref([])
const repositories = ref([])
const keyword = ref('')
const hasMore = ref(false)
const page = ref(0)
const form = ref({})
const loading = ref(false)
const repoLoading = ref(false)
const busy = ref(false)
const removing = ref(null)
const error = ref('')
const installationOptions = computed(() => installations.value.map(item => ({ value: item.id, label: item.account.login })))
const repositoryOptions = computed(() => repositories.value
  .filter(item => item.id === form.value.repository_id || item.full_name.toLowerCase().includes(keyword.value.toLowerCase()))
  .map(item => ({ value: item.id, label: item.full_name, description: item.private ? '私有仓库' : '公开仓库' })))

watch(() => [props.open, props.productId], async ([open, productId], _, onCleanup) => {
  if (!open) return
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  const options = { signal: controller.signal }
  form.value = { release_unit_id: 0, component: '', installation_id: '', repository_id: '' }
  bindings.value = []
  connection.value = null
  installations.value = []
  loading.value = true
  try {
    const [items, app] = await Promise.all([getGitHubBindings(productId, options), getGitHubConnection(options)])
    if (controller.signal.aborted) return
    bindings.value = items
    connection.value = app
    if (!app) return
    let nextPage = 1
    let result
    do {
      result = await getGitHubInstallations(nextPage++, options)
      if (controller.signal.aborted) return
      installations.value.push(...result.items.filter(item => !item.suspended_at))
    } while (result.has_more && nextPage <= 1000)
    if (installations.value.length === 1) form.value.installation_id = installations.value[0].id
  } catch (cause) {
    if (!controller.signal.aborted) error.value = cause.message
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}, { immediate: true })

watch(() => [props.open, form.value.installation_id], ([open, id], _, onCleanup) => {
  form.value.repository_id = ''
  keyword.value = ''
  repositories.value = []
  hasMore.value = false
  page.value = 0
  repoLoading.value = false
  if (open && id) {
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    void fetchRepositories(id, 1, controller.signal)
  }
})

let repoRequest = 0
async function fetchRepositories(id, nextPage, signal) {
  const request = ++repoRequest
  repoLoading.value = true
  try {
    const result = await getInstallationRepositories(id, nextPage, { signal })
    if (props.open && id === form.value.installation_id && request === repoRequest && !signal?.aborted) {
      repositories.value = [...repositories.value, ...result.items]
      page.value = nextPage
      hasMore.value = result.has_more
    }
  } catch (cause) {
    if (props.open && id === form.value.installation_id && request === repoRequest && !signal?.aborted) error.value = cause.message
  } finally {
    if (request === repoRequest) repoLoading.value = false
  }
}

function loadMore() {
  if (!repoLoading.value && hasMore.value) void fetchRepositories(form.value.installation_id, page.value + 1)
}

async function save() {
  if (busy.value || !form.value.repository_id || !form.value.component) return
  busy.value = true
  try {
    const item = await bindGitHubRepository(props.productId, form.value)
    bindings.value.push(item)
    form.value.component = ''
    form.value.repository_id = ''
    keyword.value = ''
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

async function remove() {
  const id = removing.value.id
  busy.value = true
  try {
    await unbindGitHubRepository(props.productId, id)
    bindings.value = bindings.value.filter(item => item.id !== id)
    removing.value = null
  } catch (cause) {
    removing.value = null
    error.value = cause.message
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.binding-form { margin-top: var(--space-16); }
</style>
