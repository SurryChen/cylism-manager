<template>
  <BaseModal :open="open" :title="title" size="small" @close="emit('close')">
    <div class="error-notice" role="alert">
      <CircleAlert :size="18" aria-hidden="true" />
      <p>{{ message }}</p>
    </div>
    <template #actions>
      <button type="button" class="btn btn-primary" data-testid="dismiss-error-notice" @click="emit('close')">知道了</button>
    </template>
  </BaseModal>
</template>

<script setup>
import { CircleAlert } from 'lucide-vue-next'
import BaseModal from './BaseModal.vue'

defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '操作失败' },
  message: { type: String, required: true },
})

const emit = defineEmits(['close'])
</script>

<style scoped>
.error-notice { display: flex; align-items: flex-start; gap: var(--space-8); color: var(--danger); }
.error-notice p { margin: 0; color: var(--text-secondary); font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
.error-notice svg { flex: 0 0 auto; margin-top: 2px; }
</style>
