<template>
  <EmptyState v-if="loading" variant="loading" message="正在读取仓库绑定" />
  <EmptyState v-else-if="!rows.length" message="请先在版本页的「GitHub 仓库」中为该发布端绑定仓库" />
  <div v-else class="github-versions">
    <div v-for="row in rows" :key="row.id" class="github-version-row">
      <div>
        <strong>{{ row.component }}</strong>
        <div class="cell-secondary">{{ row.repository }}</div>
      </div>
      <div v-if="row.source" class="github-version-revision">
        <span v-if="row.source.tag">{{ row.source.tag }}</span>
        <code class="cell-secondary" :title="row.source.sha">{{ row.source.sha.slice(0, 12) }}</code>
      </div>
      <div v-else-if="row.bound" class="github-version-revision">
        <SelectMenu v-model="revisionKinds[row.id]" :aria-label="row.component + ' 来源类型'" :disabled="disabled">
          <option value="tags">Tag</option>
          <option value="commits">提交</option>
        </SelectMenu>
        <GitHubRevisionMenu :key="row.id + ':' + (revisionKinds[row.id] || 'tags')" :product-id="productId" :binding-id="row.id" :kind="revisionKinds[row.id] || 'tags'" :disabled="disabled" @select="selectRevision(row, $event)" @error="$emit('error', $event)" />
      </div>
      <button v-if="row.source" type="button" class="icon-button" :aria-label="'移除 ' + row.component + ' 的版本来源'" title="移除版本来源" :disabled="disabled" @click="removeRevision(row.id)"><X :size="15" /></button>
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
.github-versions { display: grid; gap: var(--space-8); }
.github-version-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-8); }
.github-version-row > div:first-child { flex: 1; min-width: 120px; overflow-wrap: anywhere; }
.github-version-revision { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-8); }
</style>
