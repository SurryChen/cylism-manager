<template>
  <div>
    <div class="page-header">
      <div><h1 class="page-title">模型配置</h1><p class="page-subtitle">配置 Responses API 模型提供商并管理智能助手 Runtime。</p></div>
    </div>
    <AssistantSettings />
  </div>
</template>

<script setup>
import AssistantSettings from '../components/AssistantSettings.vue'
</script>

<style scoped>
.page-header{margin-bottom:var(--space-20)}.page-title{margin:0}.page-subtitle{margin:var(--space-4) 0 0;color:var(--text-secondary);font-size:13px}
</style>
