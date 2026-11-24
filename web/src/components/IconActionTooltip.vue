<template>
  <span class="icon-action-tooltip" @mouseenter="show" @mousemove="move" @mouseleave="hide" @focusin="show" @focusout="hide">
    <slot />
  </span>
  <Teleport to="body">
    <div v-if="position" class="icon-action-tooltip-content" role="tooltip" :style="{ left: `${position.x}px`, top: `${position.y}px` }">{{ text }}</div>
  </Teleport>
</template>

<script setup>
import { onBeforeUnmount, ref } from 'vue'

defineProps({ text: { type: String, required: true } })
const position = ref(null)

function place(event) {
  const rect = event.currentTarget?.getBoundingClientRect()
  const x = Number.isFinite(event.clientX) && event.clientX > 0 ? event.clientX : rect?.right || 0
  const y = Number.isFinite(event.clientY) && event.clientY > 0 ? event.clientY : rect?.bottom || 0
  position.value = {
    x: Math.max(12, Math.min(x + 10, window.innerWidth - 232)),
    y: Math.max(12, Math.min(y + 10, window.innerHeight - 52)),
  }
}

function show(event) { place(event) }
function move(event) { if (position.value && event.type === 'mousemove') place(event) }
function hide() { position.value = null }
onBeforeUnmount(hide)
</script>

<style scoped>
.icon-action-tooltip { display: inline-flex; }
.icon-action-tooltip-content { position: fixed; z-index: 1500; max-width: min(220px, calc(100vw - 24px)); max-height: min(160px, calc(100vh - 24px)); padding: 7px 9px; overflow: auto; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--text-primary); font-size: 12px; line-height: 1.4; overflow-wrap: anywhere; pointer-events: none; }
</style>
