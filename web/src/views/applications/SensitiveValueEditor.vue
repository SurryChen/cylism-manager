<template>
  <div class="sensitive-value-editor">
    <textarea
      v-if="revealed"
      :value="displayValue"
      class="form-input configuration-value"
      rows="2"
      :aria-label="`敏感配置 ${item.key} 的值`"
      placeholder="留空保留原值"
      autocomplete="off"
      spellcheck="false"
      @input="change($event.target.value)"
    ></textarea>
    <input
      v-else
      type="password"
      :value="item.value || ''"
      class="form-input configuration-value"
      :placeholder="item.original_key ? '已保存；输入新值可替换' : '填写敏感值'"
      autocomplete="new-password"
      spellcheck="false"
      :aria-label="`敏感配置 ${item.key} 的值`"
      @input="change($event.target.value)"
    />
    <button type="button" class="icon-button" :disabled="loading" :title="loading ? '正在读取...' : revealed ? '隐藏值' : '查看值'" :aria-label="`${revealed ? '隐藏' : '查看'}敏感配置 ${item.key} 的值`" :aria-pressed="revealed" :aria-busy="loading" @click="toggle">
      <EyeOff v-if="revealed" :size="15" /><Eye v-else :size="15" />
    </button>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Eye, EyeOff } from 'lucide-vue-next'
import { inspectDeploymentTemplateSecret } from '../../api/applications.js'

const props = defineProps({ item: { type: Object, required: true }, applicationId: { type: [String, Number], required: true }, templateId: { type: [String, Number], required: true }, revision: { type: Number, required: true } })
const emit = defineEmits(['error'])
const revealed = ref(false)
const loading = ref(false)
// Viewing must never populate the form's replacement payload. Only user edits
// write item.value; fetched plaintext is discarded when hidden or unmounted.
const fetchedValue = ref('')
const displayValue = computed(() => props.item.value || fetchedValue.value)
let controller
let generation = 0
function reset() {
  generation++
  controller?.abort()
  loading.value = false
  fetchedValue.value = ''
  revealed.value = false
}
async function toggle() {
  if (revealed.value) { reset(); return }
  if (props.item.value || !props.item.original_key || !props.templateId) { revealed.value = true; return }
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  const current = ++generation
  const item = props.item
  const key = item.original_key
  const revision = props.revision
  loading.value = true
  try {
    const result = await inspectDeploymentTemplateSecret(props.applicationId, props.templateId, { key, revision }, { signal: requestController.signal, cache: 'no-store' })
    if (current !== generation || item !== props.item) return
    if (result?.revision !== revision || result?.key !== key || typeof result?.value !== 'string') throw new Error('配置已改变，请关闭编辑器后重新打开')
    fetchedValue.value = item.value ? '' : result.value
    revealed.value = true
  } catch (error) {
    if (current === generation && error?.name !== 'AbortError') emit('error', error.message || '读取敏感配置失败')
  } finally {
    if (current === generation) loading.value = false
  }
}
function change(value) {
  props.item.value = value
  fetchedValue.value = ''
}
watch(() => [props.applicationId, props.templateId, props.revision, props.item, props.item.original_key], reset, { flush: 'sync' })
onBeforeUnmount(reset)
</script>

<style scoped>
.sensitive-value-editor { display: flex; align-items: flex-start; gap: var(--space-8); }
.sensitive-value-editor .form-input { min-width: 0; flex: 1; }
.sensitive-value-editor textarea { resize: vertical; }
.sensitive-value-editor .icon-button { flex: 0 0 auto; background: transparent; border-color: transparent; }
</style>
