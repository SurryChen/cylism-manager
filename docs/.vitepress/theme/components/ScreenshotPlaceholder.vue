<template>
  <figure class="screenshot-placeholder" :aria-label="`${title}截图待补充`">
    <div class="screenshot-placeholder__frame" aria-hidden="true">
      <span>截图待补充</span>
    </div>
    <figcaption>
      <strong>{{ title }}</strong>
      <span>{{ description }}</span>
      <code>{{ filename }}</code>
    </figcaption>
  </figure>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  description: string
  filename: string
}>()
</script>

<style scoped>
.screenshot-placeholder {
  margin: 1.5rem 0 2rem;
  border: 1px dashed var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
  overflow: hidden;
}

.screenshot-placeholder__frame {
  display: grid;
  min-height: 11rem;
  place-items: center;
  background: var(--vp-c-bg-alt);
  color: var(--vp-c-text-3);
  font-size: 0.9rem;
}

figcaption {
  display: grid;
  gap: 0.25rem;
  padding: 0.9rem 1rem;
  color: var(--vp-c-text-2);
  font-size: 0.9rem;
}

figcaption strong { color: var(--vp-c-text-1); }
figcaption code { color: var(--vp-c-text-3); font-size: 0.78rem; }
</style>
