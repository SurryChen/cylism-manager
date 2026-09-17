<template>
  <component :is="as" class="surface-card" :class="surfaceClasses">
    <header v-if="$slots.header || $slots.actions" class="surface-card-header">
      <div v-if="$slots.header" class="surface-card-header-main"><slot name="header" /></div>
      <div v-if="$slots.actions" class="surface-card-actions"><slot name="actions" /></div>
    </header>
    <slot />
  </component>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  as: { type: String, default: 'section', validator: value => ['section', 'article', 'aside', 'div'].includes(value) },
  padding: { type: String, default: 'md', validator: value => ['none', 'sm', 'md'].includes(value) },
  interactive: { type: Boolean, default: false },
})

const surfaceClasses = computed(() => [
  `surface-card--padding-${props.padding}`,
  { 'surface-card--interactive': props.interactive },
])
</script>

<style scoped>
.surface-card { overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow-soft); backdrop-filter: blur(30px) saturate(145%); }
.surface-card--padding-md { padding: var(--space-20); }
.surface-card--padding-sm { padding: var(--space-12); }
.surface-card--padding-none { padding: 0; }
.surface-card-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-12); margin-bottom: var(--space-12); }
.surface-card-header-main { min-width: 0; }
.surface-card-actions { display: flex; flex: 0 0 auto; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; }
.surface-card--interactive { cursor: pointer; transition: border-color .18s ease, background .18s ease, box-shadow .18s ease, transform .18s ease; }
.surface-card--interactive:hover { border-color: var(--action-primary); background: var(--surface-hover); transform: translateY(-1px); }
.surface-card--interactive:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
@supports not (backdrop-filter: blur(1px)) { .surface-card { background: var(--surface-raised); } }
@media (prefers-reduced-motion: reduce) { .surface-card--interactive { transition: none; }.surface-card--interactive:hover { transform: none; } }
@media (max-width: 640px) { .surface-card--padding-md { padding: 14px; } }
</style>
