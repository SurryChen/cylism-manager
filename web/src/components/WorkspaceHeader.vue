<template>
  <header class="workspace-header" data-header-variant="workspace">
    <div class="workspace-header-main">
      <h2 class="workspace-header-title">{{ title }}</h2>
      <p v-if="description" class="workspace-header-description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="workspace-header-actions"><slot name="actions" /></div>
  </header>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
})
</script>

<style scoped>
.workspace-header { display:flex; min-height:64px; align-items:center; justify-content:space-between; gap:var(--space-16); padding:var(--space-12) 0; }
.workspace-header-main { min-width:0; }.workspace-header-title { margin:0; color:var(--text-primary); font-size:18px; }.workspace-header-description { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; line-height:1.5; }.workspace-header-actions { display:flex; flex:0 0 auto; flex-wrap:wrap; align-items:center; justify-content:flex-end; gap:8px; }
@media (max-width:640px) { .workspace-header { align-items:stretch; flex-direction:column; }.workspace-header-actions { justify-content:flex-start; } }
</style>
