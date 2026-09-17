<template>
  <header class="section-tabs-header" data-header-variant="tabbed">
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
.section-tabs-header { position: relative; display: flex; align-items: flex-end; gap: 34px; height: var(--tabbed-page-header-height); min-height: 0; }
.section-tabs-header .page-title { margin: 0 0 23px; font-size: var(--tabbed-page-header-title-size); }
.section-tabs { display: flex; align-items: flex-end; gap: 26px; margin-bottom: 12px; }
.section-tab { position: relative; padding: 0 0 12px; border: 0; color: var(--text-secondary); background: transparent; font: inherit; font-size: 14px; cursor: pointer; transition: color var(--page-header-motion-duration) ease; }
.section-tab:hover, .section-tab.is-active { color: var(--text-primary); }
.section-tab.is-active { font-weight: 650; }
.section-tab::after { position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--action-primary); content: ''; opacity: 0; transform: scaleX(0); transform-origin: center; transition: opacity var(--page-header-motion-duration) ease, transform var(--page-header-motion-duration) ease; }
.section-tab.is-active::after { opacity: 1; transform: scaleX(1); }
.section-tabs-actions { display: flex; margin-bottom: 20px; margin-left: auto; }
@media (min-width: 641px) {
  .section-tabs-header::after { position: absolute; right: 0; bottom: 11px; left: 0; height: 1px; background: var(--border-muted); content: ''; pointer-events: none; }
}
@media (max-width: 640px) {
  .section-tabs-header { align-items: flex-start; flex-wrap: wrap; gap: 0; height: auto; }
  .section-tabs-header .page-title { margin: 0 auto var(--space-12) 0; }
  .section-tabs { margin-bottom: 0; }
  .section-tabs-actions { margin: 0 0 var(--space-12); }
  .section-tabs { width: 100%; gap: 22px; overflow-x: auto; }
}
</style>
