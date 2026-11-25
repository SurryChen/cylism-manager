<template>
  <SelectMenu
    v-model:search.trim="keyword"
    searchable
    :options="options"
    :aria-label="kind === 'tags' ? 'GitHub Tag' : 'GitHub 提交'"
    :search-label="kind === 'tags' ? '搜索 Tag' : '搜索提交'"
    :placeholder="kind === 'tags' ? '选择 Tag' : '选择提交'"
    :disabled="disabled"
    @open="loadFirst"
    @load-more="loadMore"
    @change="select"
  >
    <template #feedback>
      <span v-if="loading" class="cell-secondary">正在读取 GitHub</span>
      <span v-else-if="!options.length" class="cell-secondary">没有匹配的记录</span>
      <button v-if="hasMore" class="btn btn-sm" type="button" :disabled="loading" @click="loadMore">加载更多</button>
    </template>
  </SelectMenu>
</template>

<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { getGitHubRevisions } from '../../api/github.js'

const props = defineProps({ productId: Number, bindingId: Number, kind: String, disabled: Boolean })
const emit = defineEmits(['select', 'error'])
const controller = new AbortController()
const items = ref([])
const keyword = ref('')
const loading = ref(false)
const hasMore = ref(false)
let page = 0
const keyOf = item => item.tag || item.sha
const options = computed(() => items.value
  .filter(item => [item.tag, item.sha, item.message].some(value => value?.toLowerCase().includes(keyword.value.toLowerCase())))
  .map(item => ({ value: keyOf(item), label: item.tag || item.sha.slice(0, 12), description: item.message || item.sha.slice(0, 12) })))

async function load(nextPage) {
  if (loading.value) return
  loading.value = true
  try {
    const result = await getGitHubRevisions(props.productId, props.bindingId, props.kind, nextPage, { signal: controller.signal })
    if (controller.signal.aborted) return
    const merged = new Map([...items.value, ...result.items].map(item => [keyOf(item), item]))
    items.value = [...merged.values()]
    hasMore.value = result.has_more
    page = nextPage
  } catch (cause) {
    if (!controller.signal.aborted) emit('error', cause.message)
  } finally {
    loading.value = false
  }
}

function loadFirst() {
  if (!page) void load(1)
}

function loadMore() {
  if (hasMore.value) void load(page + 1)
}

function select(key) {
  const item = items.value.find(revision => keyOf(revision) === key)
  if (item) emit('select', item)
}

onBeforeUnmount(() => controller.abort())
</script>
