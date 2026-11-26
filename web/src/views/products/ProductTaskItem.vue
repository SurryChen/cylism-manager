<template>
  <li class="task-row" :class="{ 'task-row--done': task.status === 'done', 'task-row--card': card }">
    <input type="checkbox" class="task-check" :checked="task.status === 'done'" :aria-label="task.status === 'done' ? `重新打开：${task.title}` : `完成：${task.title}`" :disabled="busy || completionDisabled || task.status === 'cancelled'" @change="emit('toggle', task, $event)" />
    <div class="task-copy">
      <button type="button" class="task-title" :disabled="busy || editDisabled" @click="emit('edit', task)">{{ task.title }}</button>
      <OverflowTooltip v-if="task.details && !card" :text="task.details" class="cell-secondary product-line task-description" />
      <div v-if="!card || task.status === 'in_progress' || task.status === 'blocked' || task.priority === 'high' || task.due_date" class="task-meta">
        <span v-if="task.status !== 'todo' && !(card && task.status === 'done')" class="badge" :class="lifecycleClass(task.status)">{{ labelFor(taskStatuses, task.status) }}</span>
        <span v-if="!card">{{ labelFor(taskKinds, task.kind) }}</span>
        <span v-if="task.priority !== 'normal' && (!card || task.priority === 'high')" :class="{ 'product-warning': task.priority === 'high' }">{{ labelFor(priorities, task.priority) }}优先级</span>
        <span v-if="groupName">{{ groupName }}</span>
        <span v-for="name in versionNames" :key="name">{{ name }}</span>
        <span v-if="task.due_date">{{ task.due_date }}</span>
        <span v-if="task.due_date && !['done', 'cancelled'].includes(task.status)" class="badge" :class="dueClass(dueState(task.due_date))">{{ dueLabel(dueState(task.due_date)) }}</span>
      </div>
    </div>
    <div v-if="!card" class="task-row-actions">
      <button type="button" class="icon-button" title="编辑事项" aria-label="编辑事项" :disabled="busy || editDisabled" @click="emit('edit', task)"><Pencil :size="15" /></button>
      <button type="button" class="icon-button" title="删除事项" aria-label="删除事项" :disabled="busy" @click="emit('remove', task)"><Trash2 :size="15" /></button>
    </div>
  </li>
</template>

<script setup>
import { Pencil, Trash2 } from 'lucide-vue-next'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import { labelFor, dueState, dueClass, dueLabel } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, lifecycleClass } from './lifecycle-utils.js'

defineProps({
  task: { type: Object, required: true },
  versionNames: { type: Array, default: () => [] },
  groupName: { type: String, default: '' },
  busy: Boolean,
  completionDisabled: Boolean,
  editDisabled: Boolean,
  card: Boolean,
})
const emit = defineEmits(['edit', 'toggle', 'remove'])
</script>

<style scoped>
.task-row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: var(--space-12); align-items: start; padding-block: var(--space-12); border-top: 1px solid var(--border-muted); }
.task-row--card { grid-template-columns: auto minmax(0, 1fr); padding: var(--space-12); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-raised); }
.task-row--card.task-row--done { background: var(--surface-subtle); }
.task-check { width: 17px; height: 17px; margin: var(--space-4) 0 0; accent-color: var(--action-primary); cursor: pointer; }
.task-check:disabled { cursor: default; }
.task-title { max-width: 100%; padding: 0; border: 0; background: transparent; color: var(--text-primary); font-size: 14px; font-weight: 600; text-align: left; overflow-wrap: anywhere; cursor: pointer; }
.task-title:enabled:hover { color: var(--action-primary-hover); }
.task-title:disabled { cursor: default; }
.task-row--done .task-title { color: var(--text-secondary); text-decoration: line-through; }
.task-description { contain: inline-size; }
.task-meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-4) var(--space-8); margin-top: var(--space-4); color: var(--text-secondary); font-size: 11px; }
.task-row-actions { display: flex; gap: var(--space-4); }
@media (max-width: 760px) {
  .task-row { gap: var(--space-8); }
}
</style>
