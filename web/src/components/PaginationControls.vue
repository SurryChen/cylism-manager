<template>
  <nav class="pagination-controls" :aria-label="label" :aria-busy="loading">
    <span class="pagination-controls-summary" aria-live="polite">共 {{ total }} 条<span v-if="total">，第 {{ firstItem }}–{{ lastItem }} 条</span></span>
    <div class="pagination-controls-actions">
      <SelectMenu class="pagination-controls-size" :model-value="pageSize" :options="sizeOptions" aria-label="每页条数" :disabled="loading" @update:model-value="changeSize" />
      <button class="icon-button" type="button" title="首页" aria-label="首页" :disabled="loading || page <= 1" @click="changePage(1)"><ChevronsLeft :size="15" /></button>
      <button class="icon-button" type="button" title="上一页" aria-label="上一页" :disabled="loading || page <= 1" @click="changePage(page - 1)"><ChevronLeft :size="15" /></button>
      <span class="pagination-controls-status">{{ page }} / {{ totalPages }} 页</span>
      <button class="icon-button" type="button" title="下一页" aria-label="下一页" :disabled="loading || page >= totalPages" @click="changePage(page + 1)"><ChevronRight :size="15" /></button>
      <button class="icon-button" type="button" title="末页" aria-label="末页" :disabled="loading || page >= totalPages" @click="changePage(totalPages)"><ChevronsRight :size="15" /></button>
      <form class="pagination-controls-jump" novalidate @submit.prevent="jumpToPage">
        <span>前往</span>
        <input v-model="targetPage" class="form-input pagination-controls-page-input" type="number" inputmode="numeric" min="1" :max="totalPages" step="1" aria-label="跳转页码" :aria-invalid="!validTarget" :disabled="loading || totalPages <= 1" />
        <span>页</span>
        <button class="btn" type="submit" :disabled="loading || !validTarget || Number(targetPage) === page">跳转</button>
      </form>
    </div>
  </nav>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-vue-next'
import SelectMenu from './SelectMenu.vue'
import { PAGE_SIZE_OPTIONS } from '../composables/usePagination.js'

const props = defineProps({
  page: { type: Number, default: 1 },
  pageSize: { type: Number, default: 10 },
  total: { type: Number, default: 0 },
  loading: { type: Boolean, default: false },
  label: { type: String, default: '列表分页' },
})
const emit = defineEmits(['page-change', 'page-size-change'])
const sizeOptions = PAGE_SIZE_OPTIONS.map(value => ({ value, label: value + ' 条 / 页' }))
const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const firstItem = computed(() => (props.page - 1) * props.pageSize + 1)
const lastItem = computed(() => Math.min(props.page * props.pageSize, props.total))
const targetPage = ref(props.page)
const validTarget = computed(() => Number.isInteger(Number(targetPage.value)) && Number(targetPage.value) >= 1 && Number(targetPage.value) <= totalPages.value)
watch(() => [props.page, props.pageSize, props.total, props.loading], () => { targetPage.value = props.page })
function changeSize(value) {
  const size = Number(value)
  if (!props.loading && PAGE_SIZE_OPTIONS.includes(size) && size !== props.pageSize) emit('page-size-change', size)
}
function changePage(page) {
  if (!props.loading && Number.isInteger(page) && page >= 1 && page <= totalPages.value && page !== props.page) emit('page-change', page)
}
function jumpToPage() {
  if (validTarget.value) changePage(Number(targetPage.value))
}
</script>

<style scoped>
.pagination-controls { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-12); padding: var(--space-16) var(--space-20); color: var(--text-secondary); font-size: 12px; }
.pagination-controls-summary { font-variant-numeric: tabular-nums; }
.pagination-controls-actions { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-4); }
.pagination-controls-size { width: 116px; margin-right: var(--space-4); }
.pagination-controls-status { min-width: 68px; text-align: center; font-variant-numeric: tabular-nums; white-space: nowrap; }
.pagination-controls-jump { display: flex; align-items: center; gap: var(--space-4); margin-left: var(--space-4); white-space: nowrap; }
.pagination-controls-page-input { width: 64px; min-height: var(--button-height); padding: 7px 8px; text-align: center; font-size: 12px; font-variant-numeric: tabular-nums; }
@media (max-width: 640px) {
  .pagination-controls { padding: 14px; }
  .pagination-controls-actions { width: 100%; justify-content: flex-end; }
  .pagination-controls-size { margin-right: auto; }
}
@media (max-width: 420px) {
  .pagination-controls-actions { display: grid; grid-template-columns: 34px 34px minmax(60px, 1fr) 34px 34px; }
  .pagination-controls-size { grid-column: 1 / -1; justify-self: end; margin: 0 0 var(--space-4); }
  .pagination-controls-jump { grid-column: 1 / -1; justify-self: end; margin: var(--space-4) 0 0; }
}
</style>
