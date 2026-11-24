<template>
  <component :is="to ? 'RouterLink' : 'button'" v-bind="to ? { to } : { type: 'button' }" class="back-link">
    <ArrowLeft :size="16" aria-hidden="true" />
    <span><slot /></span>
  </component>
</template>

<script setup>
import { ArrowLeft } from 'lucide-vue-next'

defineProps({
  to: { type: [String, Object], default: null },
})
</script>

<style scoped>
.back-link {
  display: inline-flex;
  width: fit-content;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  margin: 0 0 var(--space-16);
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
}

.back-link svg { flex: none; }
.back-link span { overflow-wrap: anywhere; }
.back-link:hover { color: var(--action-primary); }
.back-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 3px; }
</style>
