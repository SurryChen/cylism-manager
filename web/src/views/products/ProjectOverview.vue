<template>
  <div v-if="hasDelivery || hasSidebar" class="project-overview-grid" :class="{ 'project-overview-grid--single': !hasDelivery || !hasSidebar }">
    <SurfaceCard v-if="hasDelivery" class="overview-delivery">
      <div class="overview-section-heading">
        <h2 class="product-section-title">版本与部署</h2>
      </div>

      <section v-if="version" class="overview-version" aria-label="项目版本">
        <div class="overview-section-heading">
          <div class="overview-version-heading">
            <span class="product-muted">{{ versionKind }}</span>
            <strong>{{ versionLabel(product.release_units || [], version) }}</strong>
          </div>
          <button type="button" class="btn btn-sm" @click="emit('select-tab', 'versions')">查看版本</button>
        </div>
        <ul v-if="version.github_sources?.length" class="overview-sources">
          <li v-for="source in version.github_sources" :key="source.binding_id">
            <span>{{ source.component }}</span>
            <code :title="source.tag || source.sha">{{ shortVersion(source.tag || source.sha) }}</code>
          </li>
        </ul>
      </section>
      <EmptyState v-else-if="versions.loading.value" class="overview-loading" variant="loading" message="正在读取版本…" />

      <section v-if="runtimeGroups.length" class="overview-runtime" aria-label="实际部署">
        <div class="overview-section-heading">
          <h3 class="overview-subtitle">实际部署</h3>
          <button type="button" class="btn btn-sm" @click="emit('select-tab', 'environments')">查看环境</button>
        </div>
        <div v-for="group in runtimeGroups" :key="group.id" class="overview-runtime-group">
          <div class="overview-environment">
            <Layers :size="14" aria-hidden="true" />
            <RouterLink :to="{ path: '/applications', query: { project_id: product.project_id, environment_id: group.id } }">{{ group.name }}</RouterLink>
            <span class="product-muted">{{ group.total }} 个组件</span>
          </div>
          <ul class="overview-runtime-list">
            <li v-for="app in group.items" :key="app.id">
              <RouterLink :to="'/applications/' + app.id" class="overview-app-link">{{ app.name }}</RouterLink>
              <code v-if="app.active_release?.version" :title="app.active_release.version">{{ shortVersion(app.active_release.version) }}</code>
              <span v-else class="product-muted">—</span>
              <span class="badge" :class="runtimeBadge(app.runtime?.status)">{{ runtimeLabel(app.runtime?.status) }}</span>
            </li>
          </ul>
        </div>
      </section>
      <EmptyState v-else-if="runtime.loading.value" class="overview-loading" variant="loading" message="正在读取运行状态…" />
    </SurfaceCard>

    <div v-if="hasSidebar" class="overview-sidebar">
      <SurfaceCard v-if="product.links?.length">
        <div class="overview-section-heading">
          <h2 class="product-section-title">常用链接</h2>
          <button type="button" class="btn btn-sm" :disabled="busy" @click="emit('manage-links')">管理链接</button>
        </div>
        <ul class="overview-links">
          <li v-for="(link, index) in product.links" :key="index">
            <a :href="link.url" target="_blank" rel="noopener noreferrer">
              <span class="overview-link-icon"><component :is="link.kind === 'repository' ? GitBranch : Link2" :size="15" aria-hidden="true" /></span>
              <span>{{ link.label || link.url }}</span>
              <ExternalLink :size="13" aria-hidden="true" />
            </a>
          </li>
        </ul>
      </SurfaceCard>

      <SurfaceCard v-if="openTasks.length">
        <div class="overview-section-heading">
          <h2 class="product-section-title">待办</h2>
          <button type="button" class="btn btn-sm" @click="emit('select-tab', 'tasks')">查看待办</button>
        </div>
        <ul class="overview-tasks">
          <li v-for="task in openTasks" :key="task.id">
            <span>{{ task.title }}</span>
            <span class="badge" :class="lifecycleClass(task.status)">{{ labelFor(taskStatuses, task.status) }}</span>
          </li>
        </ul>
      </SurfaceCard>
    </div>
  </div>
  <WeChatOverview :product="product" @error="emit('error', $event)" />
</template>

<script setup>
import { computed, watch } from 'vue'
import { ExternalLink, GitBranch, Layers, Link2 } from 'lucide-vue-next'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import WeChatOverview from './WeChatOverview.vue'
import { getWorkspacePage } from '../../api/list-pages.js'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { runtimeBadge, runtimeLabel } from '../applications/runtime-utils.js'
import { labelFor } from './product-utils.js'
import { lifecycleClass, taskStatuses, versionLabel } from './lifecycle-utils.js'

const props = defineProps({ product: { type: Object, required: true }, busy: Boolean })
const emit = defineEmits(['select-tab', 'manage-links', 'error'])

const versions = useAsyncResource(({ signal }, id) => getLifecyclePage(id, 'versions', { limit: 1 }, { signal }))
const tasks = useAsyncResource(({ signal }, id) => getLifecyclePage(id, 'tasks', { limit: 3 }, { signal }))
const runtime = useAsyncResource(async ({ signal }, product) => {
  const environments = new Map()
  for (const app of product.related_applications || []) {
    if ((app.management_project_id || app.project_id) === product.project_id && app.environment?.cluster_id) {
      environments.set(app.environment.id, app.environment.name)
    }
  }
  for (const app of product.transferred_applications || []) {
    if (app.environment_id && !environments.has(app.environment_id)) environments.set(app.environment_id, app.environment)
  }

  const pages = await Promise.allSettled([...environments].map(async ([id, name]) => {
    const page = await getWorkspacePage({ project_id: product.project_id, environment_id: id, view: 'runtime', limit: 5 }, { signal })
    return { id, name, items: page.items || [], total: page.total }
  }))
  return {
    groups: pages.filter(page => page.status === 'fulfilled').map(page => page.value).filter(group => group.items.length),
    failed: pages.some(page => page.status === 'rejected'),
  }
})

const version = computed(() => versions.data.value?.items?.[0])
const versionKind = computed(() => /-rc(?:[.-][\w.-]+)?(?:\+[\w.-]+)?$/i.test(version.value?.name) ? '发版候选' : '最近登记版本')
const openTasks = computed(() => (tasks.data.value?.items || []).filter(task => !['done', 'cancelled'].includes(task.status)))
const runtimeGroups = computed(() => runtime.data.value?.groups || [])
const hasDelivery = computed(() => Boolean(version.value || runtimeGroups.value.length || versions.loading.value || runtime.loading.value))
const hasSidebar = computed(() => Boolean(props.product.links?.length || openTasks.value.length))
const shortVersion = value => /^[\da-f]{40,64}$/i.test(value) ? value.slice(0, 12) : value

watch(() => props.product, async product => {
  const [, , deployments] = await Promise.all([versions.refresh(product.id), tasks.refresh(product.id), runtime.refresh(product)])
  if (product !== props.product) return
  const errors = []
  if (versions.error.value) errors.push('版本')
  if (tasks.error.value) errors.push('待办')
  if (runtime.error.value || deployments?.failed) errors.push('部署状态')
  if (errors.length) emit('error', `项目总览读取失败：${errors.join('、')}`)
}, { immediate: true })
</script>

<style scoped>
.project-overview-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); align-items: start; gap: var(--space-16); }
.project-overview-grid--single { grid-template-columns: minmax(0, 1fr); }
.overview-sidebar { display: grid; gap: var(--space-16); min-width: 0; }
.overview-section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); }
.overview-section-heading .product-section-title, .overview-subtitle { margin: 0; }
.overview-subtitle { font-size: 13px; }
.overview-version { margin-top: var(--space-12); padding: var(--space-12) var(--space-16); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.overview-version-heading { display: grid; gap: var(--space-4); min-width: 0; }
.overview-version-heading strong { font: 700 18px/1.3 var(--font-mono); overflow-wrap: anywhere; }
.overview-sources, .overview-runtime-list, .overview-links, .overview-tasks { margin: 0; padding: 0; list-style: none; }
.overview-sources { display: flex; flex-wrap: wrap; gap: var(--space-8); margin-top: var(--space-12); }
.overview-sources li { display: flex; flex-wrap: wrap; gap: var(--space-4) var(--space-8); min-width: 0; padding: var(--space-4) var(--space-8); border-radius: var(--radius-control); background: var(--surface-raised); color: var(--text-secondary); font-size: 11px; overflow-wrap: anywhere; }
.overview-runtime { margin-top: var(--space-16); }
.overview-runtime-group + .overview-runtime-group { margin-top: var(--space-12); }
.overview-environment { display: flex; align-items: center; gap: var(--space-8); padding: var(--space-8) 0; color: var(--text-secondary); font-size: 12px; }
.overview-environment a { font-weight: 600; overflow-wrap: anywhere; }
.overview-environment svg { flex: none; color: var(--action-primary); }
.overview-runtime-list li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, .7fr) auto; align-items: center; gap: var(--space-12); border-top: 1px solid var(--border-muted); padding: var(--space-8) 0; font-size: 12px; }
.overview-runtime-list li:last-child { padding-bottom: var(--space-4); }
.overview-app-link { font-weight: 600; overflow-wrap: anywhere; }
code { color: var(--text-secondary); font-family: var(--font-mono); overflow-wrap: anywhere; }
.overview-links, .overview-tasks { margin-top: var(--space-8); }
.overview-links li + li, .overview-tasks li + li { border-top: 1px solid var(--border-muted); }
.overview-links a { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: var(--space-8); padding: var(--space-8) 0; font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
.overview-links a > svg { color: var(--text-secondary); }
.overview-link-icon { display: grid; place-items: center; padding: var(--space-8); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--action-primary); }
.overview-tasks li { display: flex; align-items: center; justify-content: space-between; gap: var(--space-8); padding: var(--space-8) 0; font-size: 13px; overflow-wrap: anywhere; }
.overview-loading { min-height: var(--space-40); }
@media (max-width: 760px) {
  .project-overview-grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
