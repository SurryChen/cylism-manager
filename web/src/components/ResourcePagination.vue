<template>
  <PaginationControls v-if="!hideSinglePage || resource.total.value > resource.pageSize.value" :page="resource.page.value" :page-size="resource.pageSize.value" :total="resource.total.value" :loading="resource.loading.value" :label="label" @page-change="resource.changePage" @page-size-change="resource.changePageSize" />
  <ErrorNoticeModal :open="showError && Boolean(resource.error.value)" :message="resource.error.value?.message || '读取列表失败'" teleport @close="resource.error.value = null" />
</template>
<script setup>
import PaginationControls from './PaginationControls.vue'
import ErrorNoticeModal from './ErrorNoticeModal.vue'
defineProps({ resource: { type: Object, required: true }, label: { type: String, default: '列表分页' }, showError: { type: Boolean, default: true }, hideSinglePage: Boolean })
</script>
