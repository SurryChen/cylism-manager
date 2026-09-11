<template>
  <div
    class="feedback-banner"
    :class="`feedback-banner--${tone}`"
    :role="tone === 'info' ? 'status' : 'alert'"
  >
    <div class="feedback-banner-content">
      <slot>{{ message }}</slot>
    </div>
    <button
      v-if="dismissible"
      type="button"
      class="feedback-banner-dismiss"
      aria-label="关闭提示"
      @click="$emit('dismiss')"
    >
      ×
    </button>
  </div>
</template>

<script setup>
defineProps({
  tone: { type: String, default: 'info', validator: value => ['error', 'warning', 'success', 'info'].includes(value) },
  message: { type: String, default: '' },
  dismissible: { type: Boolean, default: false },
})

defineEmits(['dismiss'])
</script>

<style scoped>
.feedback-banner {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-12);
  margin-bottom: var(--space-16);
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  padding: 11px 13px;
  color: var(--text-primary);
  font-size: 12px;
  line-height: 1.5;
}

.feedback-banner--error {
  border-color: var(--danger);
  background: var(--danger-surface);
  color: var(--danger);
}

.feedback-banner--warning {
  border-color: var(--warning);
  background: var(--warning-surface);
  color: var(--warning);
}

.feedback-banner--success {
  border-color: var(--success);
  background: var(--success-surface);
  color: var(--success);
}

.feedback-banner--info {
  background: var(--surface-subtle);
  color: var(--text-secondary);
}

.feedback-banner-content {
  min-width: 0;
}

.feedback-banner-dismiss {
  flex: 0 0 auto;
  width: 24px;
  height: 24px;
  margin: -3px -5px 0 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}

.feedback-banner-dismiss:hover {
  background: color-mix(in srgb, currentColor 12%, transparent);
}
</style>
