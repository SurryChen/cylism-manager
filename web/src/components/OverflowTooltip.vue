<template>
  <span v-bind="$attrs" ref="trigger" class="overflow-tooltip-trigger" :class="{ 'overflow-tooltip-trigger--multiline': multiline }" tabindex="0" :aria-label="text" @mouseenter="show" @mousemove="move" @mouseleave="leave" @focus="show" @blur="hide" @keydown.esc="hide">
    <slot>{{ text }}</slot>
  </span>
  <Teleport to="body">
    <div v-if="tooltip" class="overflow-tooltip-content" :class="{ 'overflow-tooltip-content--multiline': multiline }" role="tooltip" :style="tooltip" @mouseenter="cancelHide" @mouseleave="hide">{{ text }}</div>
  </Teleport>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  text: { type: String, default: '' },
  multiline: Boolean,
})

const trigger = ref(null)
const tooltip = ref(null)
let observer
let hideTimeout

function isOverflowing() {
  const element = trigger.value
  return Boolean(element && (element.scrollWidth > element.clientWidth || (props.multiline && element.scrollHeight > element.clientHeight)))
}

function position(event) {
  if (props.multiline) {
    const rect = trigger.value.getBoundingClientRect()
    const above = rect.top
    const below = window.innerHeight - rect.bottom
    const placeAbove = below < 252 && above > below
    tooltip.value = {
      left: `${Math.max(12, Math.min(rect.left, window.innerWidth - 432))}px`,
      maxHeight: `min(240px, ${Math.max(0, (placeAbove ? above : below) - 16)}px)`,
      ...(placeAbove ? { bottom: `${window.innerHeight - rect.top + 4}px` } : { top: `${rect.bottom + 4}px` }),
    }
    return
  }
  const x = Number.isFinite(event?.clientX) ? event.clientX : 12
  const y = Number.isFinite(event?.clientY) ? event.clientY : 12
  tooltip.value = {
    left: `${Math.min(x + 14, Math.max(12, window.innerWidth - 432))}px`,
    top: `${Math.min(y + 16, Math.max(12, window.innerHeight - 252))}px`,
  }
}

function show(event) {
  cancelHide()
  if (!isOverflowing()) return
  position(event)
}

function move(event) {
  if (tooltip.value && !props.multiline) position(event)
}

function cancelHide() {
  clearTimeout(hideTimeout)
}

function leave() {
  if (props.multiline && tooltip.value) hideTimeout = setTimeout(hide, 200)
  else hide()
}

function hide() {
  cancelHide()
  tooltip.value = null
}

onMounted(async () => {
  await nextTick()
  if (typeof ResizeObserver !== 'undefined' && trigger.value) observer = new ResizeObserver(() => { if (!isOverflowing()) hide() })
  observer?.observe(trigger.value)
})

onBeforeUnmount(() => {
  cancelHide()
  observer?.disconnect()
})
</script>

<style scoped>
.overflow-tooltip-trigger { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; cursor: default; }
.overflow-tooltip-trigger--multiline { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; white-space: pre-wrap; overflow-wrap: anywhere; }
.overflow-tooltip-trigger:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.overflow-tooltip-content { position: fixed; z-index: 1500; max-width: min(420px, calc(100vw - 24px)); max-height: min(240px, calc(100vh - 24px)); padding: 9px 11px; overflow: auto; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--text-primary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; pointer-events: none; }
.overflow-tooltip-content--multiline { white-space: pre-wrap; pointer-events: auto; overscroll-behavior: contain; }
</style>
