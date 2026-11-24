<template>
  <div class="empty-state" :class="`empty-state--${variant}`">
    <span v-if="icon" class="empty-state-icon" aria-hidden="true">{{ icon }}</span>
    <span class="empty-state-message">{{ message }}</span>
    <p v-if="description" class="empty-state-description">{{ description }}</p>
    <div v-if="$slots.action" class="empty-state-action">
      <slot name="action" />
    </div>
  </div>
</template>

<script setup>
defineProps({
  message: { type: String, required: true },
  variant: { type: String, default: 'empty', validator: value => ['loading', 'empty', 'actionable'].includes(value) },
  icon: { type: String, default: '' },
  description: { type: String, default: '' },
})
</script>

<style scoped>
.empty-state {
  display: flex;
  min-height: 190px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  flex-direction: column;
  color: var(--text-muted);
  text-align: center;
}

.empty-state--loading {
  min-height: 150px;
}

.empty-state-icon {
  color: var(--action-primary);
  font-size: 27px;
  opacity: .7;
}

.empty-state-message {
  font-size: 12px;
}
.empty-state-description { max-width: 440px; margin: 0; padding: 0 16px; font-size: 12px; line-height: 1.7; }

.empty-state-action {
  margin-top: var(--space-8);
}
</style>
