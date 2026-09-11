<template>
  <header class="section-heading">
    <div class="section-heading-main">
      <h2>{{ title }}</h2>
      <p v-if="description" class="section-heading-description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="section-heading-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
})
</script>

<style scoped>
.section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-12);
  margin-bottom: var(--space-12);
}

.section-heading-main {
  min-width: 0;
}

.section-heading h2 {
  margin: 0;
  color: var(--text-primary);
  font-size: 15px;
}

.section-heading-description {
  margin: var(--space-4) 0 0;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.5;
}

.section-heading-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

@media (max-width: 640px) {
  .section-heading {
    align-items: stretch;
    flex-direction: column;
  }

  .section-heading-actions {
    justify-content: flex-start;
  }
}
</style>
