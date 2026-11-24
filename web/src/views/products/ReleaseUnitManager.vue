<template>
  <BaseModal teleport :open="open" title="管理发布端" :close-on-overlay="!busy" :close-on-escape="!busy" :show-close="!busy" @close="$emit('close')">
    <form class="release-unit-form" @submit.prevent="save">
      <input ref="input" v-model.trim="name" class="form-input" :aria-label="editing ? '修改发布端名称' : '新发布端名称'" placeholder="例如 安卓、桌面端、Web" maxlength="128" required :disabled="busy" />
      <button class="btn btn-primary" :disabled="busy">{{ editing ? '保存' : '新增' }}</button>
      <button v-if="editing" class="btn" type="button" :disabled="busy" @click="edit(null)">取消</button>
    </form>
    <EmptyState v-if="!units.length" message="当前使用项目默认发布端" class="release-unit-empty" />
    <DataTable v-else>
      <thead><tr><th>发布端</th><TableActionCell header /></tr></thead>
      <tbody>
        <tr v-for="unit in units" :key="unit.id">
          <td>{{ unit.name }}</td>
          <TableActionCell>
            <button class="btn btn-sm" type="button" :disabled="busy" @click="edit(unit)">重命名</button>
            <button class="btn btn-sm btn-danger" type="button" :disabled="busy" @click="removing = unit">删除</button>
          </TableActionCell>
        </tr>
      </tbody>
    </DataTable>
    <template #actions><button class="btn" :disabled="busy" @click="$emit('close')">关闭</button></template>
  </BaseModal>
  <ConfirmDialog teleport :open="Boolean(removing)" title="删除发布端" :message="`确认删除“${removing?.name || ''}”？`" confirm-text="删除" :busy="busy" @cancel="removing = null" @confirm="remove" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import DataTable from '../../components/DataTable.vue'
import EmptyState from '../../components/EmptyState.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { deleteReleaseUnit, saveLifecycleRecord } from '../../api/product-lifecycle.js'

const props = defineProps({ open: Boolean, product: { type: Object, required: true } })
const emit = defineEmits(['close', 'updated', 'error'])
const units = computed(() => props.product.release_units || [])
const input = ref(null)
const editing = ref(null)
const removing = ref(null)
const name = ref('')
const busy = ref(false)

function edit(unit) {
  editing.value = unit
  name.value = unit?.name || ''
  if (unit) input.value?.focus()
}

async function save() {
  if (busy.value || !name.value) return
  busy.value = true
  try {
    const item = await saveLifecycleRecord(props.product.id, 'release-units', { id: editing.value?.id, name: name.value })
    emit('updated', [...units.value.filter(unit => unit.id !== item.id), item])
    edit(null)
  } catch (cause) {
    emit('error', cause.message)
  } finally {
    busy.value = false
  }
}

async function remove() {
  if (busy.value || !removing.value) return
  const id = removing.value.id
  busy.value = true
  try {
    await deleteReleaseUnit(props.product.id, id)
    emit('updated', units.value.filter(unit => unit.id !== id))
    if (editing.value?.id === id) edit(null)
    removing.value = null
  } catch (cause) {
    removing.value = null
    emit('error', cause.message)
  } finally {
    busy.value = false
  }
}

watch(() => props.open, open => { if (open) edit(null) })
</script>

<style scoped>
.release-unit-form { display: flex; gap: var(--space-8); margin-bottom: var(--space-12); }
.release-unit-empty { min-height: 70px; }
</style>
