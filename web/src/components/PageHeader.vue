<template>
  <header class="page-header">
    <div class="page-header-main">
      <button
        v-if="backTo"
        type="button"
        class="page-header-back"
        @click="$emit('back', backTo)"
      >
        {{ backLabel }}
      </button>
      <h1 class="page-title">{{ title }}</h1>
      <p v-if="description" class="page-header-description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  backTo: { type: [String, Object], default: null },
  backLabel: { type: String, default: '返回' },
})

defineEmits(['back'])
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-16);
}

.page-header-main {
  min-width: 0;
}

.page-header-description {
  margin: var(--space-4) 0 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.page-header-back {
  display: block;
  margin: 0 0 var(--space-8);
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--action-primary);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.page-header-back:hover {
  color: var(--action-primary-hover);
  text-decoration: underline;
}

.page-header-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 640px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;
  }

  .page-header-actions {
    justify-content: flex-start;
  }
}
</style>
