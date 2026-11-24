<template>
  <nav class="cursor-pagination-controls" :aria-label="label" :aria-busy="loading">
    <span>第 {{ page }} 页 · 本页 {{ count }} 条</span>
    <div class="cursor-pagination-actions">
      <SelectMenu :model-value="pageSize" :options="options" aria-label="每页条数" :disabled="loading" @update:model-value="$emit('page-size-change', Number($event))" />
      <button class="icon-button" title="首页" aria-label="首页" :disabled="loading || page === 1" @click="$emit('page-change', 1)"><ChevronsLeft :size="15" /></button>
      <button class="icon-button" title="上一页" aria-label="上一页" :disabled="loading || page === 1" @click="$emit('page-change', page - 1)"><ChevronLeft :size="15" /></button>
      <button class="icon-button" title="下一页" aria-label="下一页" :disabled="loading || !hasNext" @click="$emit('page-change', page + 1)"><ChevronRight :size="15" /></button>
    </div>
  </nav>
</template>
<script setup>
import { ChevronLeft, ChevronRight, ChevronsLeft } from 'lucide-vue-next'
import SelectMenu from './SelectMenu.vue'
import { PAGE_SIZE_OPTIONS } from '../composables/usePagination.js'
defineProps({ page: { type: Number, default: 1 }, pageSize: { type: Number, default: 10 }, count: { type: Number, default: 0 }, hasNext: Boolean, loading: Boolean, label: { type: String, default: '列表分页' } })
defineEmits(['page-change', 'page-size-change'])
const options = PAGE_SIZE_OPTIONS.map(value => ({ value, label: value + ' 条 / 页' }))
</script>
<style scoped>
.cursor-pagination-controls { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:var(--space-12); padding:var(--space-16) var(--space-20); font-size:12px; color:var(--text-secondary); }
.cursor-pagination-actions { display:flex; gap:var(--space-4); align-items:center; }.cursor-pagination-actions :deep(.select-menu) { width:116px; }
@media(max-width:640px) { .cursor-pagination-controls { padding:14px; }.cursor-pagination-actions { margin-left:auto; } }
</style>
