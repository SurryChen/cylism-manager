<template>
  <section class="product-full-width version-task-picker" aria-label="纳入版本的事项">
    <div class="version-task-toolbar">
      <h3 class="product-section-title">纳入版本的事项</h3>
      <span class="product-muted">已选 {{ selected.length }} 项</span>
    </div>
    <input v-model.trim="keyword" class="form-input" aria-label="搜索可选事项" placeholder="搜索事项标题" />
    <EmptyState v-if="tasks.error.value" class="version-task-empty" message="事项读取失败">
      <template #action><button type="button" class="btn btn-sm" @click="load">重试</button></template>
    </EmptyState>
    <EmptyState v-else-if="tasks.loading.value && !tasks.items.value.length" class="version-task-empty" variant="loading" message="正在读取事项…" />
    <EmptyState v-else-if="!tasks.total.value" class="version-task-empty" :message="keyword ? '没有匹配的事项' : '暂无可选事项'" />
    <div v-else class="product-checklist">
      <label v-for="task in tasks.items.value" :key="task.id" class="version-task-option">
        <input type="checkbox" :value="task.id" v-model="selected" />
        <span>
          <span class="cell-primary version-task-title">{{ task.title }}</span>
          <span class="cell-secondary">{{ labelFor(taskKinds, task.kind) }} · {{ labelFor(taskStatuses, task.status) }}</span>
        </span>
      </label>
    </div>
    <ResourcePagination v-if="tasks.total.value" :resource="tasks" label="可选事项分页" :show-error="false" />
  </section>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import EmptyState from '../../components/EmptyState.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { taskKinds, taskStatuses } from './lifecycle-utils.js'
import { labelFor } from './product-utils.js'

const props = defineProps({ productId: { type: Number, required: true }, versionId: Number })
const selected = defineModel({ type: Array, required: true })
const keyword = ref('')
const tasks = usePaginatedResource((query, options) => getLifecyclePage(props.productId, 'tasks', query, options), 'items')

function load() {
  return tasks.load({ assignable: true, keyword: keyword.value, ...(props.versionId ? { version_id: props.versionId } : {}) })
}

watch(() => [props.productId, props.versionId, keyword.value], load, { immediate: true })
onBeforeUnmount(tasks.cancel)
</script>

<style scoped>
.version-task-picker { min-width: 0; }
.version-task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-8); margin-bottom: var(--space-8); }
.version-task-toolbar h3 { margin: 0; }
.version-task-empty { min-height: 0; padding: var(--space-12) 0; }
.version-task-option { padding-block: var(--space-4); }
.version-task-title { display: block; overflow-wrap: anywhere; }
</style>
