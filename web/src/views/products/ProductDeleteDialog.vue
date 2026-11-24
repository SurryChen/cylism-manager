<template>
  <BaseModal teleport :open="open" :title="projectMode ? '删除项目' : all ? '清空全部产品' : '删除产品'" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('cancel')">
    <form :id="formID" @submit.prevent="confirm">
      <p class="product-muted">{{ all ? '将永久删除当前账号的全部产品，包含当前产品和已归档产品，不受搜索、筛选及分页影响。' : `将永久删除${projectMode ? '项目' : '产品'}“${product?.name || ''}”。` }}产品资料、事项、版本、发行渠道、发行记录及历史、资源关联将一并删除，此操作不可恢复。</p>
      <p v-if="projectMode" class="product-muted">将一并删除项目及需求、版本和发行资料。项目有环境或应用时无法删除，可选择归档保留记录。</p>
      <p v-else class="product-muted">关联的运维项目、环境、部署组件、部署记录、数据库及当前管理平台不会被删除或停止。</p>
      <label v-if="all" class="form-group"><span class="form-label">请输入“删除全部产品”确认</span><input v-model="confirmation" class="form-input" aria-label="清空产品确认文字" autocomplete="off" :disabled="busy" /></label>
    </form>
    <template #actions><button class="btn" data-testid="product-delete-cancel" :disabled="busy" @click="$emit('cancel')">取消</button><button class="btn btn-danger" data-testid="product-delete-confirm" :form="formID" type="submit" :disabled="busy || (all && confirmation !== confirmationText)">{{ busy ? '正在删除...' : projectMode ? '删除项目' : all ? '清空全部产品' : '删除产品' }}</button></template>
  </BaseModal>
</template>

<script setup>
import { ref, useId, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
const props = defineProps({ projectMode: Boolean, open: Boolean, all: Boolean, busy: Boolean, product: { type: Object, default: null } })
const emit = defineEmits(['confirm', 'cancel'])
const formID = `product-delete-${useId()}`
const confirmationText = '删除全部产品'
const confirmation = ref('')
watch(() => [props.open, props.all, props.product?.id], () => { confirmation.value = '' })
function confirm() {
  if (!props.busy && (!props.all || confirmation.value === confirmationText)) emit('confirm', confirmation.value)
}
</script>
