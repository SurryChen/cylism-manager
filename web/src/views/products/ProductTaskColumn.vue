<template>
  <SurfaceCard v-if="showEmpty || !groupingAvailable || group.id || active.total.value || done.total.value || active.error.value || done.error.value" class="task-column" padding="sm">
    <header class="task-column-heading">
      <h2>{{ group.name }} <span class="product-muted">{{ active.total.value }}</span></h2>
      <button type="button" class="icon-button" :title="`添加待办到${group.name}`" :aria-label="`添加待办到${group.name}`" :disabled="busy" @click="emit('add', group.id)"><Plus :size="16" /></button>
      <button v-if="group.id" type="button" class="icon-button" :title="`管理分组：${group.name}`" :aria-label="`管理分组：${group.name}`" :disabled="busy" @click="emit('manage', group)"><Ellipsis :size="16" /></button>
    </header>
    <EmptyState v-if="active.error.value || done.error.value" class="task-column-empty" message="待办读取失败">
      <template #action><button type="button" class="btn btn-sm" @click="reload">重试</button></template>
    </EmptyState>
    <EmptyState v-else-if="active.loading.value && !active.items.value.length" class="task-column-empty" variant="loading" message="正在读取…" />
    <button v-else-if="!active.total.value && !done.total.value" type="button" class="btn task-column-add" :disabled="busy" @click="emit('add', group.id)"><Plus :size="14" />添加待办</button>
    <ul v-else-if="active.items.value.length" class="task-column-items">
      <ProductTaskItem v-for="task in active.items.value" :key="task.id" :task="task" card :busy="busy || active.loading.value" @edit="emit('edit', $event)" @toggle="forwardToggle" />
    </ul>
    <PaginationControls v-if="active.totalPages.value > 1" compact :label="`${group.name}待办分页`" :page="active.page.value" :page-size="active.pageSize.value" :total="active.total.value" :loading="active.loading.value" @page-change="active.changePage" />
    <details v-if="done.total.value" class="task-column-done">
      <summary>已完成 <span class="product-muted">{{ done.total.value }}</span></summary>
      <ul class="task-column-items">
        <ProductTaskItem v-for="task in done.items.value" :key="task.id" :task="task" card :busy="busy || done.loading.value" @edit="emit('edit', $event)" @toggle="forwardToggle" />
      </ul>
      <PaginationControls v-if="done.totalPages.value > 1" compact :label="`${group.name}已完成分页`" :page="done.page.value" :page-size="done.pageSize.value" :total="done.total.value" :loading="done.loading.value" @page-change="done.changePage" />
    </details>
  </SurfaceCard>
</template>

<script setup>
import { watch } from 'vue'
import { Ellipsis, Plus } from 'lucide-vue-next'
import EmptyState from '../../components/EmptyState.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import PaginationControls from '../../components/PaginationControls.vue'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import ProductTaskItem from './ProductTaskItem.vue'

const props = defineProps({
  productId: { type: Number, required: true },
  group: { type: Object, required: true },
  groupingAvailable: Boolean,
  showEmpty: Boolean,
  revision: { type: Number, required: true },
  busy: Boolean,
})
const emit = defineEmits(['add', 'manage', 'edit', 'toggle'])
const readTasks = (query, options) => getLifecyclePage(props.productId, 'tasks', props.groupingAvailable ? { ...query, group_id: props.group.id } : query, options)
const active = usePaginatedResource(readTasks, 'items')
const done = usePaginatedResource(readTasks, 'items')

function forwardToggle(task, event) { emit('toggle', task, event) }
function reload() {
  if (!props.groupingAvailable) return active.refresh()
  return Promise.all([
    active.filters.value.open ? active.refresh() : active.load({ open: true }),
    done.filters.value.status ? done.refresh() : done.load({ status: 'done' }),
  ])
}
watch(() => props.revision, reload, { immediate: true })
</script>

<style scoped>
.task-column { width: clamp(220px, 17vw, 272px); }
.task-column-heading { display: flex; align-items: center; gap: var(--space-4); margin-bottom: var(--space-8); }
.task-column-heading h2 { flex: 1; min-width: 0; margin: 0; font-size: 13px; overflow-wrap: anywhere; }
.task-column-heading h2 span, .task-column-done summary span { margin-left: var(--space-4); font-weight: 400; }
.task-column-heading .icon-button { flex: none; width: 30px; height: 30px; }
.task-column-items { display: grid; gap: var(--space-8); padding: 0; margin: 0; list-style: none; }
.task-column-add { justify-content: flex-start; width: 100%; border-style: dashed; background: transparent; color: var(--text-secondary); }
.task-column-empty { min-height: var(--space-40); }
.task-column-done { margin-top: var(--space-12); }
.task-column-done summary { margin-bottom: var(--space-8); color: var(--text-secondary); font-size: 12px; cursor: pointer; }
</style>
