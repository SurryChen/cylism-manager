<template>
  <BaseModal
    :open="open"
    :title="title"
    :size="size"
    :show-close="!busy"
    :close-on-overlay="!busy"
    :close-on-escape="!busy"
    @close="handleClose"
  >
    <p class="confirm-dialog-message">{{ message }}</p>
    <template #actions>
      <button
        type="button"
        class="btn"
        data-testid="confirm-dialog-cancel"
        :disabled="busy"
        @click="emit('cancel')"
      >
        {{ cancelText }}
      </button>
      <button
        type="button"
        class="btn"
        :class="confirmTone === 'danger' ? 'btn-danger' : 'btn-primary'"
        data-testid="confirm-dialog-confirm"
        :disabled="busy"
        @click="emit('confirm')"
      >
        {{ busy ? busyText : confirmText }}
      </button>
    </template>
  </BaseModal>
</template>

<script setup>
import BaseModal from './BaseModal.vue'

defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, required: true },
  message: { type: String, required: true },
  confirmText: { type: String, default: '确认' },
  cancelText: { type: String, default: '取消' },
  busyText: { type: String, default: '处理中...' },
  busy: { type: Boolean, default: false },
  confirmTone: { type: String, default: 'danger', validator: value => ['danger', 'primary'].includes(value) },
  size: { type: String, default: 'small' },
})

const emit = defineEmits(['confirm', 'cancel', 'close'])

function handleClose() {
  emit('close')
  emit('cancel')
}
</script>

<style scoped>
.confirm-dialog-message {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
</style>
