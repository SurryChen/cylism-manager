<template>
  <span ref="trigger" class="overflow-tooltip-trigger" tabindex="0" :aria-label="text" @mouseenter="show" @mousemove="move" @mouseleave="hide" @focus="show" @blur="hide">
    <slot>{{ text }}</slot>
  </span>
  <Teleport to="body">
    <div v-if="tooltip" class="overflow-tooltip-content" role="tooltip" :style="{ left: `${tooltip.x}px`, top: `${tooltip.y}px` }">{{ text }}</div>
  </Teleport>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

defineProps({
  text: { type: String, default: '' },
})

const trigger = ref(null)
const tooltip = ref(null)
let observer

function isOverflowing() {
  return Boolean(trigger.value && trigger.value.scrollWidth > trigger.value.clientWidth)
}

function position(event) {
  if (!tooltip.value || typeof window === 'undefined') return
  const x = Number.isFinite(event?.clientX) ? event.clientX : 12
  const y = Number.isFinite(event?.clientY) ? event.clientY : 12
  tooltip.value = {
    x: Math.min(x + 14, Math.max(12, window.innerWidth - 432)),
    y: Math.min(y + 16, Math.max(12, window.innerHeight - 252)),
  }
}

function show(event) {
  if (!isOverflowing()) return
  tooltip.value = { x: 12, y: 12 }
  position(event)
}

function move(event) {
  position(event)
}

function hide() {
  tooltip.value = null
}

onMounted(async () => {
  await nextTick()
  if (typeof ResizeObserver !== 'undefined' && trigger.value) observer = new ResizeObserver(() => { if (!isOverflowing()) hide() })
  observer?.observe(trigger.value)
})

onBeforeUnmount(() => observer?.disconnect())
</script>

<style scoped>
.overflow-tooltip-trigger { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; cursor: default; }
.overflow-tooltip-trigger:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.overflow-tooltip-content { position: fixed; z-index: 1500; max-width: min(420px, calc(100vw - 24px)); max-height: min(240px, calc(100vh - 24px)); padding: 9px 11px; overflow: auto; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--text-primary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; pointer-events: none; }
</style>
