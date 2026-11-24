<template>
  <SurfaceCard as="section" padding="none" class="tabbed-workspace-card">
    <div v-if="$slots.title || $slots.meta || $slots.actions" class="tabbed-workspace-toolbar">
      <div class="tabbed-workspace-context">
        <slot name="title" />
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="tabbed-workspace-actions"><slot name="actions" /></div>
    </div>
    <slot />
  </SurfaceCard>
</template>

<script setup>
import SurfaceCard from './SurfaceCard.vue'
</script>

<style scoped>
.tabbed-workspace-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: var(--space-12); padding: 12px var(--space-16); }
.tabbed-workspace-context, .tabbed-workspace-actions { display: flex; min-width: 0; align-items: center; gap: var(--space-8); }
.tabbed-workspace-context { color: var(--text-muted); font-size: 11px; }
.tabbed-workspace-context :deep(h2) { margin: 0; color: var(--text-primary); font-size: 15px; }
.tabbed-workspace-actions { margin-left: auto; justify-content: flex-end; }
.tabbed-workspace-actions :deep(.btn) { min-height: var(--button-height); padding: var(--button-padding); font-size: var(--button-font-size); }
.tabbed-workspace-card :deep(.table-wrap) { padding: 0 var(--space-16); }
.tabbed-workspace-card :deep(.empty-state) { padding: 0 var(--space-16); }
@media (max-width: 640px) { .tabbed-workspace-toolbar { align-items: stretch; flex-direction: column; }.tabbed-workspace-context { width: 100%; }.tabbed-workspace-actions { width: 100%; margin-left: 0; justify-content: flex-end; } }
</style>
