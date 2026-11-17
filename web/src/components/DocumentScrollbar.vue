<template>
  <Teleport to="body">
    <div
      v-if="maxScroll > 0 && !locked"
      ref="track"
      class="document-scrollbar"
      :class="{ 'is-dragging': dragging }"
      role="scrollbar"
      aria-label="页面滚动条"
      aria-controls="app"
      aria-orientation="vertical"
      :aria-valuemin="0"
      :aria-valuemax="Math.round(maxScroll)"
      :aria-valuenow="Math.round(scrollTop)"
      tabindex="0"
      @pointerdown.prevent="startDrag"
      @pointermove="moveDrag"
      @pointerup="endDrag"
      @pointercancel="endDrag"
      @lostpointercapture="dragging = false"
      @keydown="onKeydown"
    >
      <div ref="thumb" class="document-scrollbar-thumb" :style="{ height: `${thumbHeight}px`, transform: `translateY(${thumbTop}px)` }" />
    </div>
  </Teleport>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const track = ref(null)
const thumb = ref(null)
const scrollTop = ref(0)
const viewportHeight = ref(0)
const documentHeight = ref(0)
const locked = ref(false)
const dragging = ref(false)
const maxScroll = computed(() => Math.max(0, documentHeight.value - viewportHeight.value))
const trackHeight = computed(() => Math.max(0, viewportHeight.value - 8))
const thumbHeight = computed(() => Math.min(trackHeight.value, Math.max(28, trackHeight.value * viewportHeight.value / (documentHeight.value || 1))))
const thumbTravel = computed(() => trackHeight.value - thumbHeight.value)
const thumbTop = computed(() => maxScroll.value ? scrollTop.value / maxScroll.value * thumbTravel.value : 0)
let frame = null
let resizeObserver
let lockObserver
let dragOffset = 0

function measure() {
  frame = null
  const root = document.scrollingElement || document.documentElement
  viewportHeight.value = document.documentElement.clientHeight || window.innerHeight
  documentHeight.value = root.scrollHeight
  scrollTop.value = Math.max(0, Math.min(root.scrollTop, maxScroll.value))
  locked.value = [document.documentElement, document.body].some(element => {
    const style = getComputedStyle(element)
    return ['hidden', 'clip'].includes(element.style.overflowY || element.style.overflow || style.overflowY || style.overflow)
  })
}

function scheduleMeasure() {
  if (frame === null) frame = requestAnimationFrame(measure)
}

function scrollTo(top) {
  window.scrollTo({ top: Math.max(0, Math.min(top, maxScroll.value)), behavior: 'instant' })
  scheduleMeasure()
}

function moveDrag(event) {
  if (!dragging.value || !track.value || thumbTravel.value <= 0) return
  const position = event.clientY - track.value.getBoundingClientRect().top - dragOffset
  scrollTo(position / thumbTravel.value * maxScroll.value)
}

function startDrag(event) {
  if (event.button !== 0) return
  dragOffset = event.target === thumb.value ? event.clientY - thumb.value.getBoundingClientRect().top : thumbHeight.value / 2
  dragging.value = true
  track.value.setPointerCapture(event.pointerId)
  track.value.focus({ preventScroll: true })
  moveDrag(event)
}

function endDrag(event) {
  dragging.value = false
  if (track.value?.hasPointerCapture(event.pointerId)) track.value.releasePointerCapture(event.pointerId)
  // Pointer interaction must not leave an idle thumb visible via keyboard focus.
  track.value?.blur()
}

function onKeydown(event) {
  const targets = {
    ArrowDown: scrollTop.value + 40,
    ArrowUp: scrollTop.value - 40,
    PageDown: scrollTop.value + viewportHeight.value,
    PageUp: scrollTop.value - viewportHeight.value,
    Home: 0,
    End: maxScroll.value,
  }
  if (!(event.key in targets)) return
  event.preventDefault()
  scrollTo(targets[event.key])
}

onMounted(() => {
  measure()
  window.addEventListener('scroll', scheduleMeasure, { passive: true })
  window.addEventListener('resize', scheduleMeasure, { passive: true })
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(scheduleMeasure)
    resizeObserver.observe(document.documentElement)
    resizeObserver.observe(document.body)
  }
  lockObserver = new MutationObserver(scheduleMeasure)
  lockObserver.observe(document.body, { attributes: true, attributeFilter: ['style'] })
  lockObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['style'] })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', scheduleMeasure)
  window.removeEventListener('resize', scheduleMeasure)
  resizeObserver?.disconnect()
  lockObserver?.disconnect()
  if (frame !== null) cancelAnimationFrame(frame)
})
</script>

<style scoped>
.document-scrollbar {
  position: fixed;
  z-index: 100;
  top: var(--space-4);
  right: 0;
  bottom: var(--space-4);
  width: var(--space-12);
  opacity: 0;
  pointer-events: none;
  touch-action: none;
  user-select: none;
  transition: opacity 180ms ease;
}
html.is-scrolling .document-scrollbar,
.document-scrollbar.is-dragging,
.document-scrollbar:focus-visible {
  opacity: 1;
  pointer-events: auto;
}
.document-scrollbar-thumb {
  position: absolute;
  right: 2px;
  width: 7px;
  border-radius: var(--radius-control);
  background: var(--text-muted);
}
.document-scrollbar:focus-visible { outline: 2px solid var(--focus); outline-offset: -2px; border-radius: var(--radius-control); }
</style>
