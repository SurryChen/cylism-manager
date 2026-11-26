<template>
  <div class="task-board-layout">
    <div class="task-board-toolbar">
      <slot />
      <div class="task-board-actions">
        <div v-if="active && (canScrollLeft || canScrollRight)" class="btn-group" role="group" aria-label="浏览看板分组">
          <button type="button" class="icon-button" title="向左滚动" aria-label="向左滚动" :aria-controls="boardID" :disabled="!canScrollLeft" @click="scrollBoard(-1)"><ChevronLeft :size="16" /></button>
          <button type="button" class="icon-button" title="向右滚动" aria-label="向右滚动" :aria-controls="boardID" :disabled="!canScrollRight" @click="scrollBoard(1)"><ChevronRight :size="16" /></button>
        </div>
        <span v-if="!groupingAvailable" class="product-muted">分组功能需更新后端</span>
        <button type="button" class="btn btn-sm" :title="groupingAvailable ? '新建待办分组' : '更新后端后可新建待办分组'" :disabled="busy || saving || !groupingAvailable" @click="openGroup()"><Plus :size="15" />新建分组</button>
      </div>
    </div>
    <div
      v-if="active" :id="boardID" ref="viewport" class="task-board"
      :class="{ 'task-board--pannable': canScrollLeft || canScrollRight, 'task-board--dragging': dragging }"
      role="region" aria-label="待办分组看板" :tabindex="canScrollLeft || canScrollRight ? 0 : undefined"
      @scroll.passive="updateScroll" @wheel="onBoardWheel"
      @pointerdown.capture="startDrag" @pointermove="moveDrag" @pointerup="endDrag"
      @pointercancel="endDrag" @lostpointercapture="endDrag" @pointerleave="leaveBoard"
      @click.capture="preventDragClick"
    >
      <div ref="strip" class="task-board-columns">
        <ProductTaskColumn
          v-for="group in columns" :key="groupingAvailable ? group.id : 'all'"
          :product-id="productId" :group="group" :grouping-available="groupingAvailable" :show-empty="!groups?.length" :revision="revision" :busy="busy || saving"
          @add="emit('add', $event)" @edit="emit('edit', $event)" @toggle="forwardToggle" @manage="openGroup"
        />
      </div>
    </div>
  </div>

  <BaseModal teleport :open="Boolean(editing)" :title="editing?.id ? '管理分组' : '新建分组'" size="small" :show-close="!saving" :close-on-overlay="!saving" :close-on-escape="!saving" @close="closeGroup">
    <form :id="formID" @submit.prevent="saveGroup">
      <label class="form-group">
        <span class="form-label">分组名称</span>
        <input v-model.trim="name" class="form-input" aria-label="分组名称" placeholder="例如 发布验证" maxlength="64" :disabled="saving" required />
      </label>
    </form>
    <template #actions>
      <button v-if="editing?.id" type="button" class="btn btn-danger task-board-remove" :disabled="saving" @click="askRemove">删除分组</button>
      <button type="button" class="btn" :disabled="saving" @click="closeGroup">取消</button>
      <button type="submit" class="btn btn-primary" :form="formID" :disabled="saving || !name">保存分组</button>
    </template>
  </BaseModal>
  <ConfirmDialog teleport :open="Boolean(removing)" title="删除分组" :message="`删除「${removing?.name || ''}」后，该分组中的待办会移到「未分组」。`" confirm-text="删除分组" :busy="saving" @cancel="removing = null" @confirm="removeGroup" />
</template>

<script setup>
import { computed, ref, useId, watchEffect } from 'vue'
import { ChevronLeft, ChevronRight, Plus } from 'lucide-vue-next'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import { deleteProductTaskGroup, saveLifecycleRecord } from '../../api/product-lifecycle.js'
import ProductTaskColumn from './ProductTaskColumn.vue'

const props = defineProps({
  active: Boolean,
  productId: { type: Number, required: true },
  groups: { type: [Array, null], required: true },
  revision: { type: Number, required: true },
  busy: Boolean,
})
const emit = defineEmits(['add', 'edit', 'toggle', 'changed', 'error'])
const formID = `task-group-${useId()}`
const boardID = `task-board-${useId()}`
const groupingAvailable = computed(() => props.groups !== null)
const columns = computed(() => groupingAvailable.value
  ? [{ id: 0, name: '未分组' }, ...props.groups]
  : [{ id: 0, name: '全部待办' }])
const viewport = ref(null)
const strip = ref(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)
const dragging = ref(false)
let drag = null
let suppressClick = false
const editing = ref(null)
const removing = ref(null)
const name = ref('')
const saving = ref(false)

function updateScroll() {
  if (!viewport.value) return
  const { scrollLeft, scrollWidth, clientWidth } = viewport.value
  canScrollLeft.value = scrollLeft > 1
  canScrollRight.value = scrollLeft + clientWidth < scrollWidth - 1
}

watchEffect(onCleanup => {
  const scroller = viewport.value
  const content = strip.value
  if (!scroller || !content) {
    canScrollLeft.value = false
    canScrollRight.value = false
    dragging.value = false
    drag = null
    suppressClick = false
    return
  }
  updateScroll()
  if (typeof ResizeObserver === 'undefined') return
  const observer = new ResizeObserver(updateScroll)
  observer.observe(scroller)
  observer.observe(content)
  onCleanup(() => observer.disconnect())
}, { flush: 'post' })

function scrollBoard(direction) {
  const scroller = viewport.value
  if (!scroller) return
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  scroller.scrollBy({ left: direction * scroller.clientWidth * .8, behavior: reducedMotion ? 'instant' : 'smooth' })
}

function onBoardWheel(event) {
  if (!event.shiftKey || event.ctrlKey || event.defaultPrevented) return
  const scroller = event.currentTarget
  const delta = event.deltaX || event.deltaY
  const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? scroller.clientWidth : 1
  const next = Math.max(0, Math.min(scroller.scrollWidth - scroller.clientWidth, scroller.scrollLeft + delta * unit))
  if (next === scroller.scrollLeft) return
  event.preventDefault()
  scroller.scrollLeft = next
}

function startDrag(event) {
  if (!event.isPrimary) return
  suppressClick = false
  const scroller = event.currentTarget
  if (event.pointerType !== 'mouse' || event.button !== 0 || scroller.scrollWidth <= scroller.clientWidth) return
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, scrollLeft: scroller.scrollLeft }
}

function moveDrag(event) {
  if (drag?.id !== event.pointerId) return
  if (!(event.buttons & 1)) {
    endDrag(event)
    return
  }
  const distance = event.clientX - drag.x
  const scroller = event.currentTarget
  if (!dragging.value) {
    if (Math.abs(distance) < 5 || Math.abs(distance) < Math.abs(event.clientY - drag.y)) return
    scroller.setPointerCapture(event.pointerId)
    dragging.value = true
    suppressClick = true
  }
  event.preventDefault()
  scroller.scrollLeft = drag.scrollLeft - distance
}

function endDrag(event) {
  if (drag?.id !== event.pointerId) return
  drag = null
  dragging.value = false
}

function leaveBoard(event) {
  if (!dragging.value) endDrag(event)
}

function preventDragClick(event) {
  if (!suppressClick || event.detail === 0) return
  suppressClick = false
  event.preventDefault()
  event.stopPropagation()
}

function forwardToggle(task, event) { emit('toggle', task, event) }
function openGroup(group = { id: 0, name: '' }) {
  if (props.busy || saving.value || !groupingAvailable.value) return
  editing.value = group
  name.value = group.name
}
function closeGroup() {
  if (!saving.value) editing.value = null
}
function askRemove() {
  removing.value = editing.value
  editing.value = null
}
async function saveGroup() {
  if (props.busy || saving.value || !editing.value || !name.value || !groupingAvailable.value) return
  saving.value = true
  try {
    await saveLifecycleRecord(props.productId, 'task-groups', { id: editing.value.id || undefined, name: name.value })
    editing.value = null
    emit('changed')
  } catch (error) {
    emit('error', error.message)
  } finally {
    saving.value = false
  }
}
async function removeGroup() {
  if (saving.value || !removing.value) return
  saving.value = true
  try {
    await deleteProductTaskGroup(props.productId, removing.value.id)
    removing.value = null
    emit('changed')
  } catch (error) {
    emit('error', error.message)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.task-board-layout { grid-column: 1 / -1; display: grid; gap: var(--space-12); min-width: 0; }
.task-board-toolbar, .task-board-actions { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-8); }
.task-board-toolbar { justify-content: space-between; }
.task-board {
  min-width: 0;
  overflow-x: auto;
  overscroll-behavior-x: contain;
}
.task-board:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.task-board-columns { display: flex; align-items: flex-start; gap: var(--space-16); width: max-content; min-width: 100%; }
.task-board--pannable .task-board-columns { cursor: grab; user-select: none; }
.task-board--dragging .task-board-columns, .task-board--dragging .task-board-columns :deep(*) { cursor: grabbing; }
.task-board-columns:not(:empty) { padding-bottom: var(--space-12); }
.task-board-columns > * { flex: none; }
.task-board-remove { margin-right: auto; }
</style>
