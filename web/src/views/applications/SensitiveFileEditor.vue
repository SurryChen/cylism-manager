<template>
  <div class="sensitive-file-editor">
    <div class="file-editor-toolbar">
      <div class="file-editor-heading"><strong>{{ fullVisible ? '文件内容' : fieldsLoaded ? `配置项 · ${fields.length}` : '文件内容' }}</strong><span v-if="hasFieldChanges || rawDirty" class="draft-indicator">未保存</span><span class="file-help" tabindex="0" aria-label="值默认隐藏，点击眼睛查看；只保存修改项，查看记录审计。" title="值默认隐藏，点击眼睛查看；只保存修改项，查看记录审计。"><Info :size="14" /></span></div>
      <input v-if="fieldsLoaded && fields.length > 8 && !fullVisible" v-model="search" type="search" class="form-input field-search" aria-label="搜索配置项" placeholder="搜索字段名" />
      <details ref="actionsMenu" class="file-editor-actions" @keydown.esc.stop="closeActions" @focusout="onActionsFocusOut">
        <summary class="icon-button" title="内容操作" aria-label="内容操作"><Ellipsis :size="16" /></summary>
        <div class="file-actions-menu" @click="closeActions">
        <button v-if="isEnv && !fullVisible && !rawDirty" type="button" class="btn btn-sm" :disabled="loading" @click="loadFields()">{{ fieldsLoaded ? '重新读取' : '逐项编辑' }}</button>
        <button v-if="!fullVisible" type="button" class="btn btn-sm" :disabled="loading || hasFieldChanges" @click="confirmReveal = true">查看并编辑全文</button>
        <button v-else type="button" class="btn btn-sm" @click="hideContent">隐藏全文</button>
        <button v-if="hasFieldChanges || rawDirty" type="button" class="btn btn-sm" @click="discardChanges">撤销修改</button>
        </div>
      </details>
    </div>
    <EmptyState v-if="loading && !fullVisible && !fieldsLoaded" variant="loading" message="正在读取配置..." />
    <template v-else-if="fullVisible">
      <textarea :value="item.value" class="form-input file-content" rows="7" aria-label="文件内容" placeholder="留空保留原值" spellcheck="false" @input="updateContent($event.target.value)"></textarea>
    </template>
    <template v-else-if="fieldsLoaded && !rawDirty">
      <div class="env-file-fields" role="group" aria-label="配置项">
        <div class="env-field-columns" aria-hidden="true"><span>字段名</span><span>值 <small>· 点击编辑</small></span><span></span></div>
        <div v-for="field in visibleFields" :key="field.line === -1 ? field.id : field.line" class="env-file-field" :class="{ 'field-removed': field.remove, 'field-changed': field.changed }">
          <div class="env-field-name">
            <input v-if="field.line === -1" v-model="field.name" class="form-input" aria-label="新增配置项名称" placeholder="例如 MYSQL_HOST" @input="syncChanges" />
            <code v-else>{{ field.name }}</code>
          </div>
          <input :type="field.revealed ? 'text' : 'password'" :value="field.value ?? ''" class="form-input env-field-value" :class="{ 'is-editing': field.editing }" :aria-label="`配置项 ${field.name || '新配置项'} 的值`" :placeholder="field.line !== -1 && !field.changed && !field.revealed ? (field.editing ? '输入新值，留空可清空' : '••••••••') : '值（可为空）'" :readonly="!field.editing" :disabled="field.remove || loading" autocomplete="off" spellcheck="false" @focus="field.editing = true" @blur="field.editing = false" @input="changeField(field, $event.target.value)" />
          <div class="env-field-actions">
            <button type="button" class="icon-button" :disabled="field.remove || loading" :title="field.revealed ? '隐藏值' : '查看值'" :aria-label="`${field.revealed ? '隐藏' : '查看'}配置项 ${field.name || '新配置项'} 的值`" :aria-pressed="!!field.revealed" @click="toggleField(field)"><EyeOff v-if="field.revealed" :size="15" /><Eye v-else :size="15" /></button>
            <button type="button" class="icon-button field-remove" :disabled="loading" :title="field.remove ? '恢复配置项' : '移除配置项'" :aria-label="`${field.remove ? '恢复' : '移除'}配置项 ${field.name || '新配置项'}`" @click="removeField(field)"><Undo2 v-if="field.remove" :size="15" /><Trash2 v-else :size="15" /></button>
          </div>
        </div>
      </div>
      <EmptyState v-if="!visibleFields.length" message="没有匹配的配置项" />
      <button type="button" class="btn btn-sm add-field" @click="addField"><Plus :size="13" />添加配置项</button>
    </template>
    <p v-else-if="unsupportedMessage" class="form-hint">{{ unsupportedMessage }}</p>
    <p v-else class="form-hint">{{ rawDirty ? '已修改的内容将在保存模板时提交。' : '敏感内容已保存。查看全文会记录审计，关闭编辑器后清除页面中的明文。' }}</p>
    <BaseModal :open="confirmReveal" title="查看敏感文件" size="small" teleport @close="confirmReveal = false">
      <p>将读取 {{ item.original_key }} 的完整内容，可能包含密码或私钥。本次查看会记录审计，请注意屏幕隐私。</p>
      <template #actions><button type="button" class="btn" @click="confirmReveal = false">取消</button><button type="button" class="btn btn-primary" @click="revealContent">查看并编辑</button></template>
    </BaseModal>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Ellipsis, Eye, EyeOff, Info, Plus, Trash2, Undo2 } from 'lucide-vue-next'
import { inspectDeploymentTemplateFile } from '../../api/applications.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'

const props = defineProps({ item: { type: Object, required: true }, applicationId: { type: [String, Number], required: true }, templateId: { type: [String, Number], required: true }, revision: { type: Number, required: true } })
const emit = defineEmits(['error'])
const fields = ref([])
const fieldsLoaded = ref(false)
const loading = ref(false)
const fullVisible = ref(false)
const rawDirty = ref(false)
const confirmReveal = ref(false)
const unsupportedMessage = ref('')
const search = ref('')
const actionsMenu = ref(null)
let nextFieldID = 0
const visibleFields = computed(() => fields.value.filter(field => field.line === -1 || field.name.toLowerCase().includes(search.value.trim().toLowerCase())))
const isEnv = computed(() => /^(?:env|\.env(?:\..+)?|.+\.env)$/i.test(props.item.original_key || ''))
const hasFieldChanges = computed(() => !!props.item.file_edit?.changes?.length)
function closeActions() { if (actionsMenu.value) actionsMenu.value.open = false }
function onActionsFocusOut(event) { if (!event.currentTarget.contains(event.relatedTarget)) closeActions() }
let controller
let generation = 0

async function readFile(mode, parameters = {}) {
  controller?.abort()
  controller = new AbortController()
  const current = ++generation
  const item = props.item
  loading.value = true
  try {
    const result = await inspectDeploymentTemplateFile(props.applicationId, props.templateId, { key: item.original_key, revision: props.revision, mode, ...parameters }, { signal: controller.signal, cache: 'no-store' })
    if (current !== generation || item !== props.item) return null
    if (result?.revision !== props.revision) throw new Error('模板版本已改变，请关闭编辑器后重新打开')
    return result
  } catch (error) {
    if (current === generation && error?.name !== 'AbortError') emit('error', error.message || '读取配置文件失败')
    return null
  } finally {
    if (current === generation) loading.value = false
  }
}
async function loadFields(preserveChanges = false) {
  if (!preserveChanges && (hasFieldChanges.value || rawDirty.value)) { emit('error', '请先保存或撤销修改，再重新读取'); return }
  const result = await readFile('fields')
  if (!result) return
  fields.value = (result.fields || []).map(({ line, name }) => ({ line, name, changed: false, remove: false, revealed: false }))
  if (preserveChanges) {
    for (const change of props.item.file_edit?.changes || []) {
      if (change.line === -1) fields.value.push({ ...change, id: ++nextFieldID, revealed: false, changed: true })
      else {
        const field = fields.value.find(field => field.line === change.line && field.name === change.name)
        if (field) Object.assign(field, change, { changed: Object.hasOwn(change, 'value') })
      }
    }
  }
  fieldsLoaded.value = result.supported
  unsupportedMessage.value = result.supported ? '' : result.message || '此文件需要使用全文编辑'
}
async function toggleField(field) {
  if (field.revealed) {
    field.revealed = false
    if (!field.changed) delete field.value
    return
  }
  if (field.changed || field.line === -1) { field.revealed = true; return }
  const result = await readFile('field', { line: field.line, name: field.name })
  if (!result) return
  if (result.line !== field.line || result.name !== field.name || typeof result.value !== 'string') {
    emit('error', '配置项已改变，请重新读取文件')
    return
  }
  field.value = result.value
  field.revealed = true
}
async function revealContent() {
  confirmReveal.value = false
  if (rawDirty.value) { fullVisible.value = true; return }
  const result = await readFile('content')
  if (!result) return
  fields.value = []
  fieldsLoaded.value = false
  delete props.item.file_edit
  props.item.value = result.content
  fullVisible.value = true
}
function updateContent(value) { props.item.value = value; rawDirty.value = true }
function hideContent() {
  fullVisible.value = false
  if (!rawDirty.value) props.item.value = ''
}
function syncChanges() {
  const changes = fields.value.filter(field => field.line === -1 || field.changed || field.remove).map(field => ({ line: field.line, name: field.name, ...(field.remove ? { remove: true } : { value: field.value ?? '' }) }))
  if (changes.length) props.item.file_edit = { key: props.item.original_key, changes }
  else delete props.item.file_edit
}
function changeField(field, value) { field.value = value; field.changed = true; syncChanges() }
function removeField(field) {
  if (field.line === -1) fields.value.splice(fields.value.indexOf(field), 1)
  else field.remove = !field.remove
  syncChanges()
}
function addField() { search.value = ''; fields.value.push({ line: -1, id: ++nextFieldID, name: '', value: '', revealed: false, editing: true, changed: true, remove: false }); syncChanges() }
function discardChanges() {
  generation++
  controller?.abort()
  loading.value = false
  props.item.value = ''
  delete props.item.file_edit
  fields.value = []
  fieldsLoaded.value = false
  rawDirty.value = false
  fullVisible.value = false
  if (isEnv.value) void loadFields()
}
watch(() => [props.applicationId, props.templateId, props.revision, props.item], () => {
  generation++
  controller?.abort()
  fields.value = []
  fieldsLoaded.value = false
  fullVisible.value = false
  rawDirty.value = !!props.item.value
  unsupportedMessage.value = ''
  search.value = ''
  if (isEnv.value && !rawDirty.value) void loadFields(true)
}, { immediate: true })
onBeforeUnmount(() => { generation++; controller?.abort(); fields.value = []; if (!rawDirty.value) props.item.value = '' })
</script>

<style scoped>
.file-editor-toolbar{display:flex;align-items:center;gap:8px;margin-bottom:var(--space-8)}
.file-editor-heading{display:flex;align-items:center;gap:8px;margin-right:auto;font-size:12px}
.file-editor-heading strong{font-weight:600}
.file-help{display:inline-flex;color:var(--text-muted);cursor:help}
.file-help:focus-visible{outline:2px solid var(--focus)}
.draft-indicator{color:var(--warning);font-size:11px}
.field-search{max-width:200px}
.file-editor-actions{position:relative}
.file-editor-actions>summary{list-style:none;background:transparent;border-color:transparent}
.file-editor-actions>summary::-webkit-details-marker{display:none}
.file-actions-menu{position:absolute;right:0;top:100%;z-index:5;display:grid;min-width:152px;padding:6px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--canvas);box-shadow:var(--shadow-soft)}
.file-actions-menu>.btn{text-align:left;justify-content:flex-start;border-color:transparent;background:transparent;white-space:nowrap}
.file-actions-menu>.btn:hover{background:var(--surface-hover)}
.form-hint{margin:0;color:var(--text-secondary);font-size:12px;line-height:1.6}
.file-content{font-family:var(--font-mono);font-size:12px;resize:vertical}
.env-file-fields{max-height:360px;overflow:auto;scrollbar-gutter:stable;margin-bottom:var(--space-8)}
.env-file-field,.env-field-columns{display:grid;grid-template-columns:minmax(130px,.9fr) minmax(160px,1.4fr) 68px;align-items:center;gap:8px}
.env-field-columns{position:sticky;top:0;z-index:1;background:var(--canvas);padding:8px;color:var(--text-muted);font-size:10px}
.env-field-columns small{font-size:10px;font-weight:400}
.env-file-field{padding:4px 8px;border-bottom:1px solid var(--border-muted);min-height:42px}
.env-file-field:hover,.env-file-field:focus-within{background:var(--surface-subtle)}
.env-field-actions{display:flex;align-items:center;gap:0}
.env-field-actions>.icon-button{background:transparent;border-color:transparent}
.field-remove{opacity:0;pointer-events:none}
.env-file-field:hover .field-remove,.env-file-field:focus-within .field-remove,.field-removed .field-remove{opacity:1;pointer-events:auto}
.env-field-name{display:flex;align-items:center;gap:8px;min-width:0}
.env-field-name code{font-size:12px;overflow-wrap:anywhere}
.env-field-value{font-family:var(--font-mono);font-size:12px;background:transparent;border-color:transparent;box-shadow:none;cursor:text;min-width:0}
.env-field-value:focus,.env-field-value.is-editing{background:var(--surface-input);border-color:var(--focus)}
.field-changed .env-field-name{border-left:2px solid var(--warning);padding-left:6px}
.add-field{background:transparent;border-color:transparent;color:var(--action-primary);gap:6px}
.field-removed .env-field-name{text-decoration:line-through;opacity:.6}
@media(hover:none){.field-remove{opacity:1;pointer-events:auto}}
@media(max-width:640px){.env-file-field,.env-field-columns{grid-template-columns:minmax(100px,1fr) minmax(60px,1fr) 60px;gap:4px;padding-left:4px;padding-right:4px}.env-field-actions>.icon-button{width:30px;height:30px}.env-field-value{padding-left:6px;padding-right:6px}.env-field-columns small{display:none}.file-editor-toolbar{flex-wrap:wrap}.field-search{max-width:150px}}
</style>
