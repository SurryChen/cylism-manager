<template>
  <div class="template-configuration">
    <details class="editor-disclosure" :open="environmentItems.length > 0">
      <summary>环境变量 <small>{{ environmentItems.length ? `${environmentItems.length} 项` : '未配置' }}</small></summary>
      <div class="configuration-toolbar"><p class="form-hint">普通值和敏感值统一维护，敏感值加密保存。</p><button type="button" class="btn btn-sm" @click="addVariable">添加变量</button></div>
      <div v-if="environmentItems.length" class="environment-list">
        <div v-for="({ item, source }, index) in environmentItems" :key="index" class="environment-entry">
          <div class="environment-row">
            <input v-model.trim="item.key" class="form-input" required aria-label="环境变量名称" placeholder="例如 DATABASE_HOST" />
            <SelectMenu :model-value="source" aria-label="变量存储方式" @update:model-value="changeVariableSource(item, source, $event)"><option value="application_config">普通值</option><option value="application_secret">敏感值</option></SelectMenu>
            <button type="button" class="icon-button" title="移除环境变量" aria-label="移除环境变量" @click="removeItem(item, source)"><Trash2 :size="15" /></button>
          </div>
          <textarea v-model="item.value" class="form-input configuration-value" rows="2" aria-label="环境变量值" :placeholder="source === 'application_secret' && item.original_key ? '已保存敏感值；留空保留原值' : '变量值'"></textarea>
          <details class="entry-options"><summary>更多设置</summary><label class="check-row"><input v-model="item.enabled" type="checkbox" /> 启用</label><label class="check-row"><input v-model="item.externally_managed" type="checkbox" /> 允许外部修改</label></details>
        </div>
      </div>
      <EmptyState v-else class="configuration-empty" message="尚未配置环境变量" />
    </details>

    <details class="editor-disclosure" :open="form.file_mounts.length > 0">
      <summary>配置文件 <small>{{ form.file_mounts.length ? `${form.file_mounts.length} 个文件` : '未配置' }}</small></summary>
      <div class="configuration-toolbar"><p class="form-hint" title="请使用独立配置目录，挂载会覆盖该目录中的原有文件。">选择文件查看和编辑内容</p><button type="button" class="btn btn-sm" @click="addFile">添加文件</button></div>
      <div v-if="form.file_mounts.length" class="configuration-files">
        <details v-for="(file, index) in form.file_mounts" :key="file.ui_id || index" class="configuration-file file-mount-row" :open="file.expanded">
          <summary @click.prevent="toggleFile(file)"><span class="file-summary"><span class="file-title"><strong>{{ file.key || '新配置文件' }}</strong><span class="badge">{{ file.source_type.includes('secret') ? 'Secret' : 'ConfigMap' }}</span></span><small>{{ file.mount_path || '设置挂载路径' }}</small></span><button type="button" class="icon-button file-settings-button" title="文件设置" :aria-label="`文件设置 ${file.key || '新配置文件'}`" @click.stop.prevent="openFileSettings(file)"><Settings2 :size="15" /></button></summary>
          <div class="file-fields">
            <BaseModal :open="file.settings_open" title="文件设置" size="medium" @close="file.settings_open = false">
            <div class="file-source-row"><SelectMenu :model-value="file.source_type" aria-label="文件来源类型" @update:model-value="changeFileSource(file, $event)"><option value="application_config">编写普通文件</option><option value="application_secret">编写敏感文件</option><option value="configmap">引用已有 ConfigMap</option><option value="secret">引用已有 Secret</option></SelectMenu><button type="button" class="icon-button" title="移除配置文件" aria-label="移除配置文件" @click="removeFile(file)"><Trash2 :size="15" /></button></div>
            <template v-if="isApplicationSource(file.source_type)">
              <div class="file-location-row">
                <label class="file-field"><span class="form-label">文件名称</span><input class="form-input" :value="file.key" required aria-label="文件名称" placeholder="例如 config.yaml" @input="renameFile(file, $event.target.value)" /></label>
                <label class="file-field"><span class="form-label">容器内文件路径</span><input v-model.trim="file.mount_path" class="form-input" required aria-label="文件挂载路径" placeholder="/etc/app/config.yaml" /></label>
              </div>
            </template>
            <template v-else>
              <label class="form-label">已有资源</label>
              <SelectMenu v-model="file.source_name" aria-label="文件来源资源" required @change="file.key = ''"><option value="" disabled>选择资源</option><option v-for="resource in resourcesFor(file.source_type)" :key="resource.name" :value="resource.name">{{ resource.name }}</option></SelectMenu>
              <label class="form-label">资源 Key</label>
              <SelectMenu v-model="file.key" aria-label="文件来源键" required><option value="" disabled>选择 Key</option><option v-for="key in keysFor(file)" :key="key" :value="key">{{ key }}</option></SelectMenu>
            </template>
            <template v-if="!isApplicationSource(file.source_type)"><label class="form-label">容器内文件路径</label><input v-model.trim="file.mount_path" class="form-input" required aria-label="文件挂载路径" placeholder="/etc/app/config.yaml" /></template>
            <details v-if="entryFor(file)" class="entry-options"><summary>更多设置</summary><label class="check-row"><input :checked="!entryFor(file).file_only" type="checkbox" aria-label="同时注入环境变量" @change="entryFor(file).file_only = !$event.target.checked" /> 同时注入环境变量</label><label class="check-row"><input v-model="entryFor(file).externally_managed" type="checkbox" /> 允许外部修改</label></details>
            <p class="form-hint file-location-hint">请使用独立配置目录，挂载会覆盖该目录中的原有文件。</p>
            <template #actions><button type="button" class="btn" @click="file.settings_open = false">完成</button></template>
            </BaseModal>
            <template v-if="isApplicationSource(file.source_type)">
              <template v-if="file.source_type === 'application_secret' && entryFor(file)?.original_key && templateId"><SensitiveFileEditor v-if="file.expanded" :key="entryFor(file).original_key" :item="entryFor(file)" :application-id="applicationId" :template-id="templateId" :revision="revision" @error="emit('error', $event)" /></template>
              <template v-else><label class="form-label file-content-label">文件内容</label><textarea v-if="entryFor(file)" v-model="entryFor(file).value" class="form-input configuration-value file-content" rows="5" aria-label="文件内容" :placeholder="file.source_type === 'application_secret' && entryFor(file).original_key ? '已保存敏感内容；留空保留原值' : '填写文件内容'"></textarea><p v-else class="form-hint">当前文件键不存在，请在文件设置中填写名称。</p></template>
            </template>
            <p v-else class="form-hint">引用 {{ file.source_type === 'secret' ? 'Secret' : 'ConfigMap' }} · {{ file.source_name || '未选择资源' }} / {{ file.key || '未选择 Key' }}<br />资源与挂载路径可在文件设置中修改。</p>
          </div>
        </details>
      </div>
      <EmptyState v-else class="configuration-empty" message="尚未配置文件" />
    </details>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Settings2, Trash2 } from 'lucide-vue-next'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SensitiveFileEditor from './SensitiveFileEditor.vue'

const props = defineProps({ form: { type: Object, required: true }, configmaps: { type: Array, default: () => [] }, secrets: { type: Array, default: () => [] }, applicationId: { type: [String, Number], default: '' }, templateId: { type: [String, Number], default: '' }, revision: { type: Number, default: 0 } })
const emit = defineEmits(['error'])
let nextFileID = 0
const isApplicationSource = source => source === 'application_config' || source === 'application_secret'
const itemsFor = source => source === 'application_secret' ? props.form.secret_items : props.form.config_items
const entryFor = file => isApplicationSource(file.source_type) ? (itemsFor(file.source_type).find(item => item === file.entry) || itemsFor(file.source_type).find(item => item.key === file.key)) : null
const resourcesFor = source => source === 'secret' ? props.secrets : props.configmaps
const keysFor = file => resourcesFor(file.source_type).find(resource => resource.name === file.source_name)?.keys || []
const environmentItems = computed(() => ['application_config', 'application_secret'].flatMap(source => itemsFor(source).filter(item => !props.form.file_mounts.some(file => file.source_type === source && entryFor(file) === item)).map(item => ({ item, source }))))

function toggleFile(file) { const open = !file.expanded; for (const other of props.form.file_mounts) other.expanded = false; file.expanded = open }
function openFileSettings(file) { for (const other of props.form.file_mounts) { other.expanded = other === file; other.settings_open = other === file } }
function newItem(fileOnly = false) { return { key: '', value: '', enabled: true, externally_managed: false, file_only: fileOnly } }
function addVariable() { props.form.config_items.push(newItem()) }
function removeItem(item, source) { const items = itemsFor(source); const index = items.indexOf(item); if (index !== -1) items.splice(index, 1) }
function changeVariableSource(item, source, next) {
  if (source === next) return
  if (item.key && itemsFor(next).some(other => other.key === item.key)) { emit('error', '目标存储中已存在同名配置，请先修改名称'); return }
  if (source === 'application_secret' && item.original_key && !item.value) { emit('error', '敏感值不会读取为普通值，请重新填写内容后再切换存储方式'); return }
  removeItem(item, source)
  itemsFor(next).push({ ...item, original_key: undefined })
}
function addFile() {
  const entry = newItem(true)
  props.form.config_items.push(entry)
  // Keep the reference to distinguish a new empty file from other empty entries.
  const stored = props.form.config_items.at(-1)
  props.form.file_mounts.push({ source_type: 'application_config', source_name: '', key: '', mount_path: '', entry: stored, expanded: true, ui_id: `new-${++nextFileID}` })
  openFileSettings(props.form.file_mounts.at(-1))
}
function removeUnusedFileEntry(item, source) {
  if (item?.file_only && !props.form.file_mounts.some(file => file.source_type === source && entryFor(file) === item)) removeItem(item, source)
}
function removeFile(file) {
  const item = entryFor(file)
  props.form.file_mounts.splice(props.form.file_mounts.indexOf(file), 1)
  removeUnusedFileEntry(item, file.source_type)
}
function renameFile(file, key) {
  const item = entryFor(file)
  if (item) {
    for (const mount of props.form.file_mounts) { if (mount.source_type === file.source_type && entryFor(mount) === item) mount.key = key }
    item.key = key
    file.entry = item
  } else {
    const existing = itemsFor(file.source_type).find(candidate => candidate.key === key)
    const next = existing || { ...newItem(true), key }
    if (!existing) itemsFor(file.source_type).push(next)
    file.entry = itemsFor(file.source_type).find(candidate => candidate === next || candidate.key === key)
  }
  file.key = key
}
function changeFileSource(file, next) {
  if (file.source_type === next) return
  const source = file.source_type
  const previous = entryFor(file)
  if (isApplicationSource(next) && file.key && itemsFor(next).some(item => item.key === file.key)) { emit('error', '目标存储中已存在同名配置，请先修改文件名称'); return }
  if (previous?.original_key && source === 'application_secret' && next === 'application_config' && !previous.value) { emit('error', '敏感文件不会读取为普通文件，请重新填写内容后再切换存储方式'); return }
  file.source_type = next
  file.source_name = ''
  file.entry = null
  if (isApplicationSource(next)) {
    const item = { ...newItem(true), key: previous?.key || '', value: previous?.value || '' }
    itemsFor(next).push(item)
    file.entry = itemsFor(next).at(-1)
    file.key = item.key
  } else file.key = ''
  removeUnusedFileEntry(previous, source)
}
</script>

<style scoped>
.editor-disclosure{border-bottom:1px solid var(--border-muted);padding:var(--space-16) 0}
.editor-disclosure>summary{cursor:pointer;color:var(--text-primary);font-size:13px;font-weight:600}
.editor-disclosure>summary small{margin-left:8px;color:var(--text-muted);font-size:11px;font-weight:400}
.configuration-toolbar{display:flex;align-items:center;justify-content:space-between;gap:var(--space-12);margin:var(--space-12) 0}
.configuration-toolbar>.btn{flex:none}
.form-hint{margin:0;color:var(--text-secondary);font-size:12px;line-height:1.6}
.environment-list,.configuration-files{display:grid;gap:var(--space-12)}
.environment-entry{padding-bottom:var(--space-12);border-bottom:1px solid var(--border-muted)}
.environment-entry:last-child{border-bottom:0}
.environment-row{display:grid;grid-template-columns:minmax(0,1fr) 110px 34px;gap:8px;margin-bottom:8px;align-items:center}
.configuration-value{font-family:var(--font-mono);font-size:12px;resize:vertical}
.entry-options{margin-top:var(--space-8);font-size:12px;color:var(--text-secondary)}
.entry-options>summary{cursor:pointer}
.check-row{display:flex;align-items:center;gap:8px;margin:12px 0;font-size:12px}
.configuration-file{border-bottom:1px solid var(--border-muted);padding-bottom:var(--space-12)}
.configuration-file>summary{padding:4px 0}
.file-title{display:flex;align-items:center;gap:8px}
.file-title strong{font-size:13px;font-weight:600}
.file-title .badge{font-size:10px;font-weight:400}
.file-settings-button{background:transparent;border-color:transparent}
.file-location-hint{margin-top:var(--space-16)}
.file-content-label{display:block;color:var(--text-secondary);font-size:11px;margin-bottom:8px}
.configuration-file>summary{display:flex;align-items:center;gap:12px;cursor:pointer;list-style:none}
.configuration-file>summary::before{content:'▸';color:var(--text-muted)}
.configuration-file[open]>summary::before{content:'▾'}
.file-summary{display:grid;gap:4px;min-width:0;flex:1;font-size:12px}
.file-summary strong,.file-summary small{overflow-wrap:anywhere}
.file-summary small{color:var(--text-secondary);font-weight:400;font-size:11px}
.file-fields{padding-top:var(--space-12)}
.file-fields>.form-label{display:block;margin:12px 0 6px}
.file-location-row{display:grid;grid-template-columns:1fr 1fr;gap:var(--space-12);margin-top:var(--space-12)}
.file-field{display:grid;gap:6px;min-width:0}
.file-source-row{display:flex;align-items:center;gap:8px}
.file-source-row>.select-menu{flex:1;min-width:0}
.configuration-empty{min-height:64px}
@media(max-width:640px){.file-location-row{grid-template-columns:1fr}.configuration-toolbar{align-items:flex-start}.environment-row{grid-template-columns:minmax(0,1fr) 100px 34px}}
</style>
