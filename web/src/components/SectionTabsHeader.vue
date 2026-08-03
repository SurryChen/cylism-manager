<template>
  <header class="section-tabs-header">
    <h1 class="page-title">{{ title }}</h1>
    <nav class="section-tabs" :aria-label="title">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        :data-testid="testIdPrefix ? `${testIdPrefix}-${tab.id}` : undefined"
        class="section-tab"
        :class="{ 'is-active': activeTab === tab.id }"
        :aria-current="activeTab === tab.id ? 'page' : undefined"
        @click="$emit('select', tab.id)"
      >
        {{ tab.label }}
      </button>
    </nav>
    <div v-if="$slots.actions" class="section-tabs-actions"><slot name="actions" /></div>
  </header>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  tabs: { type: Array, required: true },
  activeTab: { type: String, required: true },
  testIdPrefix: { type: String, default: '' },
})

defineEmits(['select'])
</script>

<style scoped>
.section-tabs-header { display: flex; align-items: flex-end; gap: 34px; height: 52px; min-height: 0; border-bottom: 1px solid var(--border-muted); }
.section-tabs-header .page-title { margin: 0 0 11px; font-size: 24px; }
.section-tabs { display: flex; align-items: flex-end; gap: 26px; }
.section-tab { position: relative; padding: 0 0 12px; border: 0; color: var(--text-secondary); background: transparent; font: inherit; font-size: 14px; cursor: pointer; }
.section-tab:hover, .section-tab.is-active { color: var(--text-primary); }
.section-tab.is-active { font-weight: 650; }
.section-tab.is-active::after { position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--action-primary); content: ''; }
.section-tabs-actions { display: flex; margin-bottom: 8px; margin-left: auto; }
@media (max-width: 640px) {
  .section-tabs-header { align-items: flex-start; flex-wrap: wrap; gap: 0; height: auto; }
  .section-tabs-header .page-title { margin: 0 auto var(--space-12) 0; }
  .section-tabs-actions { margin: 0 0 var(--space-12); }
  .section-tabs { width: 100%; gap: 22px; overflow-x: auto; }
}
</style>
