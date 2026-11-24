<template>
  <header class="section-tabs-header" :class="{ 'sticky-page-header': sticky, 'section-tabs-header--compact': compact }" data-header-variant="tabbed">
    <slot name="title"><h1 v-if="!compact" class="page-title">{{ title }}</h1></slot>
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
  sticky: { type: Boolean, default: true },
  compact: { type: Boolean, default: false },
})

defineEmits(['select'])
</script>

<style scoped>
.section-tabs-header { display: flex; align-items: flex-end; gap: 34px; height: var(--tabbed-page-header-height); min-height: 0; }
.section-tabs-header .page-title { margin: 0 0 23px; font-size: var(--tabbed-page-header-title-size); }
.section-tabs { display: flex; align-items: flex-end; gap: 26px; margin-bottom: 12px; }
.section-tab { position: relative; padding: 0 0 12px; border: 0; color: var(--text-secondary); background: transparent; font: inherit; font-size: 14px; cursor: pointer; transition: color var(--page-header-motion-duration) ease; }
.section-tab:hover, .section-tab.is-active { color: var(--text-primary); }
.section-tab.is-active { font-weight: 650; }
.section-tab::after { position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--action-primary); content: ''; opacity: 0; transform: scaleX(0); transform-origin: center; transition: opacity var(--page-header-motion-duration) ease, transform var(--page-header-motion-duration) ease; }
.section-tab.is-active::after { opacity: 1; transform: scaleX(1); }
.section-tabs-actions { display: flex; margin-bottom: 20px; margin-left: auto; }
.section-tabs-header--compact { position: relative; flex-shrink: 0; height: auto; border-bottom: 1px solid var(--border-muted); margin-bottom: var(--space-24); }
.section-tabs-header--compact::after { display: none; }
.section-tabs-header--compact .section-tabs { width: 100%; margin: 0; gap: 28px; overflow-x: auto; }
.section-tabs-header--compact .section-tab { flex-shrink: 0; padding: 14px 0; white-space: nowrap; }
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
