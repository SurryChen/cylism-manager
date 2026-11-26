<template>
  <EmptyState v-if="loading" variant="loading" message="正在读取仓库绑定" />
  <EmptyState v-else-if="!rows.length" :message="releaseUnitId === '' ? '请先选择发布端' : '可在「版本设置 → GitHub 仓库」绑定仓库'" />
  <div v-else class="github-versions">
    <div v-for="row in rows" :key="row.id" class="github-version-row" role="group" :aria-label="row.component">
      <div class="github-version-component">
        <strong>{{ row.component }}</strong>
        <div class="cell-secondary">{{ row.repository }}</div>
      </div>
      <div v-if="row.source" class="github-version-selection">
        <div>
          <div v-if="row.source.tag">{{ row.source.tag }}</div>
          <code class="cell-secondary" :title="row.source.sha">{{ row.source.sha.slice(0, 12) }}</code>
        </div>
        <button type="button" class="icon-button" :aria-label="'移除 ' + row.component + ' 的版本来源'" title="移除版本来源" :disabled="disabled" @click="removeRevision(row.id)"><X :size="15" aria-hidden="true" /></button>
      </div>
      <div v-else-if="row.bound" class="github-version-revision">
        <SelectMenu v-model="revisionKinds[row.id]" :aria-label="row.component + ' 来源类型'" :disabled="disabled">
          <option value="tags">Tag</option>
          <option value="commits">提交</option>
        </SelectMenu>
        <GitHubRevisionMenu :key="row.id + ':' + (revisionKinds[row.id] || 'tags')" :product-id="productId" :binding-id="row.id" :kind="revisionKinds[row.id] || 'tags'" :disabled="disabled" @select="selectRevision(row, $event)" @error="$emit('error', $event)" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import GitHubRevisionMenu from './GitHubRevisionMenu.vue'
import { getGitHubBindings } from '../../api/github.js'

const props = defineProps({ modelValue: { type: Array, default: () => [] }, productId: Number, releaseUnitId: [Number, String], disabled: Boolean })
const emit = defineEmits(['update:modelValue', 'error'])
const bindings = ref([])
const loading = ref(false)
const revisionKinds = ref({})
const rows = computed(() => {
  const byID = new Map(bindings.value.map(binding => [binding.id, { ...binding, bound: true }]))
  for (const source of props.modelValue) {
    byID.set(source.binding_id, { id: source.binding_id, component: source.component, repository: source.repository, source })
  }
  return [...byID.values()]
})

watch(() => [props.productId, props.releaseUnitId], async ([productId, releaseUnitId], _, onCleanup) => {
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  bindings.value = []
  revisionKinds.value = {}
  if (releaseUnitId === '') {
    loading.value = false
    return
  }
  loading.value = true
  try {
    const items = await getGitHubBindings(productId, { signal: controller.signal })
    if (!controller.signal.aborted) {
      bindings.value = items.filter(item => item.release_unit_id === Number(releaseUnitId))
      revisionKinds.value = Object.fromEntries(bindings.value.map(binding => [binding.id, 'tags']))
    }
  } catch (cause) {
    if (!controller.signal.aborted) emit('error', cause.message)
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}, { immediate: true })

function selectRevision(row, revision) {
  emit('update:modelValue', [...props.modelValue, {
    binding_id: row.id, component: row.component, repository: row.repository, tag: revision.tag || '', sha: revision.sha,
  }])
}

function removeRevision(id) {
  emit('update:modelValue', props.modelValue.filter(source => source.binding_id !== id))
}
</script>

<style scoped>
.github-version-row { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr); align-items: center; gap: var(--space-12); padding: var(--space-8) 0; }
.github-version-row + .github-version-row { border-top: 1px solid var(--border-muted); }
.github-version-component, .github-version-selection { overflow-wrap: anywhere; font-size: var(--button-font-size); }
.github-version-revision, .github-version-selection { display: grid; align-items: center; gap: var(--space-8); }
.github-version-revision { grid-template-columns: 84px minmax(0, 1fr); }
.github-version-selection { grid-template-columns: minmax(0, 1fr) auto; }
@media (max-width: 500px) {
  .github-version-row { grid-template-columns: minmax(0, 1fr); gap: var(--space-8); }
}
</style>
