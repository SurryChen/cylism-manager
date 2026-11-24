<template>
  <div class="form-group product-full-width">
    <label class="form-label" :for="id">纳入版本的事项</label>
    <SelectMenu
      :id="id"
      v-model="selected"
      v-model:search.trim="keyword"
      multiple
      searchable
      :disabled="disabled"
      :options="options"
      :placeholder="selected.length ? `已选 ${selected.length} 项` : '选择事项'"
      aria-label="纳入版本的事项"
      search-label="搜索可选事项"
      @open="loadFirstBatch"
      @load-more="loadMore"
    >
      <template #feedback>
        <button v-if="tasks.error.value" type="button" class="btn btn-sm" @click="load(tasks.data.value.offset)">事项读取失败，重试</button>
        <div v-else-if="tasks.loading.value" class="task-picker-feedback" role="status">正在读取事项…</div>
        <div v-else-if="!options.length" class="task-picker-feedback" role="status">{{ keyword ? '没有匹配的事项' : '暂无可选事项' }}</div>
      </template>
    </SelectMenu>
  </div>
</template>

<script setup>
import { computed, ref, useId, watch } from 'vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { taskKinds, taskStatuses } from './lifecycle-utils.js'
import { labelFor } from './product-utils.js'

const props = defineProps({ productId: { type: Number, required: true }, versionId: Number, disabled: Boolean })
const selected = defineModel({ type: Array, required: true })
const id = useId()
const keyword = ref('')
const batchSize = 50
const empty = { items: [], offset: 0, total: 0 }
const tasks = useAsyncResource(async ({ signal }, offset) => {
  const query = { assignable: true, keyword: keyword.value, limit: batchSize, offset }
  if (props.versionId) query.version_id = props.versionId
  const result = await getLifecyclePage(props.productId, 'tasks', query, { signal })
  signal.throwIfAborted()
  const items = result.items || []
  const previous = offset ? tasks.data.value.items : []
  return {
    items: [...new Map([...previous, ...items].map(task => [task.id, task])).values()],
    offset: offset + items.length,
    total: items.length ? result.total : offset,
  }
}, empty)

const options = computed(() => tasks.data.value.items.map(task => ({
  value: task.id,
  label: task.title,
  description: `${labelFor(taskKinds, task.kind)} · ${labelFor(taskStatuses, task.status)}`,
})))

function load(offset) {
  return tasks.refresh(offset)
}

function loadFirstBatch() {
  tasks.data.value = empty
  return load(0)
}

function loadMore() {
  if (!tasks.loading.value && !tasks.error.value && tasks.data.value.offset < tasks.data.value.total) {
    return load(tasks.data.value.offset)
  }
}

watch(() => [props.productId, props.versionId, keyword.value], loadFirstBatch)
</script>

<style scoped>
.task-picker-feedback { padding: var(--space-8); color: var(--text-muted); font-size: 12px; }
</style>
