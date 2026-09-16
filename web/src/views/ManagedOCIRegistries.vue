<template>
  <div class="page-shell registry-page">
    <SectionTabsHeader title="制品库" :tabs="workspaceTabs" :active-tab="activeWorkspace" test-id-prefix="managed-registry-workspace" @select="selectWorkspace">
      <template #actions><button v-if="activeWorkspace === 'registry' && !registry" data-testid="deploy-registry" class="btn btn-primary" :disabled="!loaded" @click="openCreate"><Plus :size="16" /> 部署制品库</button></template>
    </SectionTabsHeader>

    <template v-if="activeWorkspace === 'registry'">
      <p v-if="pageError" class="form-error" role="alert">{{ pageError }}</p>
      <div v-if="!loaded" class="empty-state">正在读取制品库配置...</div>
      <section v-else-if="!registry" class="registry-empty" data-testid="registry-empty">
        <Database :size="26" />
        <div><h2>尚未部署自托管制品库</h2><p>部署后，平台会创建受认证保护、使用持久化存储并通过集群入口提供服务的 OCI Registry。</p></div>
      </section>
      <template v-else>
        <section class="metric-grid registry-overview" data-testid="registry-summary">
          <article class="metric registry-metric"><span>访问地址</span><strong class="registry-endpoint">{{ endpointURL(registry) }}</strong><small>{{ registry.insecure_http ? 'HTTP，凭据和镜像层以明文传输' : `HTTPS · ${registry.certificate_name || '未关联证书'}` }}</small></article>
          <article class="metric registry-metric"><span>运行状态</span><strong><span class="badge" :class="statusBadgeClass(registry.status)"><i class="badge-dot"></i>{{ statusLabel(registry.status) }}</span></strong><small>{{ registry.last_error || 'Registry 工作负载状态已同步' }}</small></article>
          <article class="metric registry-metric"><span>持久化存储</span><strong>{{ registry.pvc_name }}</strong><small>{{ registry.storage_class_name }} · {{ registry.storage_size }} · {{ registry.pvc_phase || '等待状态同步' }}</small></article>
        </section>
        <div class="registry-status-actions" aria-label="制品库操作"><button v-if="needsRepair" data-testid="repair-registry" class="btn" :disabled="repairing" @click="repairRegistry"><RefreshCw :size="16" :class="{ 'is-spinning': repairing }" /> {{ repairing ? '修复中...' : '修复' }}</button><button class="icon-button" title="刷新状态" aria-label="刷新状态" :disabled="refreshing" @click="load"><RefreshCw :size="17" :class="{ 'is-spinning': refreshing }" /></button><button data-testid="edit-registry" class="btn" @click="openEdit">配置</button><button class="btn btn-danger" @click="deleteOpen = true">删除</button></div>

        <section class="registry-catalog" data-testid="registry-catalog">
        <div class="registry-catalog-toolbar"><label class="registry-search" aria-label="搜索仓库"><span class="registry-search-input"><Search :size="15" /><input v-model.trim="catalogQuery" class="form-input" placeholder="搜索仓库" /></span></label><span class="registry-catalog-count">{{ filteredRepositories.length }} 个仓库</span></div>
        <section v-if="catalogError" class="card registry-catalog-state" data-testid="registry-catalog-error"><FeedbackBanner tone="warning"><span>暂时无法读取镜像目录，请稍后重试。</span><button class="btn btn-sm" :disabled="catalogLoading" @click="loadCatalog()"><RefreshCw :size="14" :class="{ 'is-spinning': catalogLoading }" /> 重新尝试</button></FeedbackBanner><EmptyState message="镜像目录内容暂不可展示" /></section>
        <EmptyState v-else-if="catalogLoading && !catalogRepositories.length" variant="loading" message="正在读取镜像目录..." />
        <EmptyState v-else-if="catalogLoaded && !filteredRepositories.length" message="制品库中暂无镜像" />
        <div v-else class="registry-catalog-grid">
          <section class="card registry-repository-list"><div class="registry-list-heading"><h2>仓库</h2></div><div v-for="repository in filteredRepositories" :key="repository" class="registry-repository-row" :class="{ 'is-selected': selectedRepository === repository }"><button class="registry-repository-select" type="button" @click="selectRepository(repository)">{{ repository }}</button><button class="icon-button registry-row-icon" type="button" title="删除仓库" :aria-label="`删除仓库 ${repository}`" @click="prepareRepositoryDelete(repository)"><Trash2 :size="15" /></button></div><div v-if="catalogNext" class="registry-load-more"><button class="btn btn-sm" :disabled="catalogLoading" @click="loadCatalog(catalogNext)">加载更多</button></div></section>
          <section class="card registry-tags-panel"><template v-if="!selectedRepository"><EmptyState message="选择一个仓库以查看标签" /></template><template v-else><header class="registry-list-heading"><div><h2>{{ selectedRepository }}</h2><p>{{ tags.length }} 个标签</p></div><button class="icon-button" title="刷新标签" aria-label="刷新标签" :disabled="tagsLoading" @click="loadTags(selectedRepository)"><RefreshCw :size="16" :class="{ 'is-spinning': tagsLoading }" /></button></header><EmptyState v-if="tagsLoading && !tags.length" variant="loading" message="正在读取标签..." /><EmptyState v-else-if="tagsLoaded && !tags.length" message="该仓库暂无标签" /><div v-else class="registry-tags-table"><div v-for="tag in tags" :key="tag.name" class="registry-tag-row"><div><strong>{{ tag.name }}</strong><code>{{ shortDigest(tag.digest) }}</code><small v-if="tag.platforms?.length">{{ tag.platforms.join(' · ') }}</small></div><div class="registry-tag-actions"><button class="icon-button" :title="`复制 ${tag.pull_reference}`" :aria-label="`复制 ${tag.pull_reference}`" @click="copyPullReference(tag.pull_reference)"><Copy :size="15" /></button><button class="icon-button danger-icon" :title="`删除标签 ${tag.name}`" :aria-label="`删除标签 ${tag.name}`" @click="prepareTagDelete(tag)"><Trash2 :size="15" /></button></div></div></div><div v-if="tagsNext" class="registry-load-more"><button class="btn btn-sm" :disabled="tagsLoading" @click="loadTags(selectedRepository, tagsNext)">加载更多</button></div></template></section>
        </div>
        </section>
      </template>
    </template>
    <RegistryProxyWorkspace v-else />

    <Teleport to="body"><div v-if="showForm" class="overlay registry-overlay" @click.self="closeForm"><div class="modal registry-modal"><h2 class="modal-title">{{ registry ? '编辑制品库' : '部署制品库' }}</h2><form @submit.prevent="save">
      <div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required placeholder="平台制品库" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">命名空间</label><input :value="form.namespace" class="form-input" readonly /><p class="form-hint">平台受管制品库固定部署在 <code>cylism-system</code>。</p></div><div class="form-group"><label class="form-label">Registry 镜像</label><input v-model.trim="form.registry_image" class="form-input" required placeholder="registry:2" /></div></div>
      <div class="form-group"><label class="form-label">验证镜像</label><input v-model.trim="form.verification_image" class="form-input" required :placeholder="`${form.endpoint || 'registry.example.com'}/cylism-manager:1.0.0`" /><p class="form-hint">用于检测制品库入口和节点镜像源，必须属于当前 Registry 地址。</p></div>
      <div class="form-group"><div class="volume-heading"><label class="form-label">持久化存储卷</label><a class="btn btn-sm" :href="pvcCreateLink()">创建 PVC</a></div><SelectMenu v-model="form.pvc_name" class="form-select registry-pvc-select" required :disabled="Boolean(registry) || pvcOptionsLoading"><option value="" disabled>{{ pvcOptionsLoading ? '正在读取可用 PVC...' : '选择 cylism-system 中已有的 local-path PVC' }}</option><option v-for="pvc in pvcOptions" :key="pvc.name" :value="pvc.name">{{ pvc.name }} · {{ pvc.storage }} · {{ pvc.phase }}<template v-if="pvc.bound_node"> · {{ pvc.bound_node }}</template></option></SelectMenu><p class="form-hint">仅显示 <code>local-path</code>、<code>ReadWriteOnce</code> 且未被工作负载引用的 PVC。平台只挂载所选 PVC，不会创建、修改或删除它。</p><p v-if="!pvcOptionsLoading && !pvcOptions.length" class="form-hint">没有可选 PVC，请先在存储卷管理中创建。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">数据节点</label><SelectMenu v-model="form.data_node" class="form-select registry-data-node-select" required :disabled="Boolean(registry) || Boolean(selectedPVC?.bound_node) || registryDataNodes.length === 0"><option value="" disabled>选择承载数据的 Kubernetes 节点</option><option v-for="node in registryDataNodes" :key="node" :value="node">{{ node }}</option></SelectMenu><p class="form-hint">待绑定 PVC 会在此节点首次挂载时创建本地卷；已绑定 PVC 自动使用其绑定节点。</p></div><div class="form-group"><label class="form-label">所选存储</label><div class="form-input registry-storage-summary">{{ selectedPVC ? `${selectedPVC.storage_class_name} · ${selectedPVC.storage} · ${selectedPVC.phase}` : '选择 PVC 后显示' }}</div><p class="form-hint">StorageClass 和容量由已选 PVC 决定。</p></div></div>
      <section class="resource-config"><div class="section-heading"><div><h3>运行资源</h3><p>为 Registry Pod 预留并限制 CPU 与内存用量。</p></div></div><div class="resource-grid"><label class="form-group"><span class="form-label">CPU Request</span><input v-model.trim="form.cpu_request" class="form-input" required placeholder="100m" /></label><label class="form-group"><span class="form-label">CPU Limit</span><input v-model.trim="form.cpu_limit" class="form-input" required placeholder="500m" /></label><label class="form-group"><span class="form-label">内存 Request</span><input v-model.trim="form.memory_request" class="form-input" required placeholder="256Mi" /></label><label class="form-group"><span class="form-label">内存 Limit</span><input v-model.trim="form.memory_limit" class="form-input" required placeholder="1Gi" /></label></div></section>
      <p v-if="storagePreflight && !storagePreflight.ready" class="form-error">{{ storagePreflight.message }}</p>
      <div class="form-group"><label class="form-label">传输方式</label><div class="transport-options"><label><input v-model="form.insecure_http" :value="false" type="radio" /> HTTPS</label><label><input v-model="form.insecure_http" :value="true" type="radio" /> HTTP</label></div></div>
      <div v-if="!form.insecure_http" class="form-group"><div class="volume-heading"><label class="form-label">TLS 证书</label><a class="btn btn-sm" :href="certificateCreateLink()">创建证书</a></div><SelectMenu v-model="form.certificate_name" class="form-select registry-certificate-select" required :disabled="certificatesLoading" @change="selectCertificate"><option value="" disabled>{{ certificatesLoading ? '正在读取可用证书...' : '选择 cylism-system 中已就绪的证书' }}</option><option v-for="certificate in certificateOptions" :key="certificate.namespace + '/' + certificate.name" :value="certificate.name">{{ certificate.name }} · {{ (certificate.domains || []).join(', ') }}</option></SelectMenu><p class="form-hint">选择证书后会自动填入其第一个精确域名；证书续期会由 cert-manager 自动写入 Ingress 使用的 Secret。</p><p v-if="certificateError" class="form-error">{{ certificateError }}</p><p v-else-if="!certificatesLoading && !certificateOptions.length" class="form-hint">没有可用证书。请先在 <code>cylism-system</code> 创建已就绪的证书。</p></div>
      <label v-else class="risk-confirm"><input v-model="form.confirm_insecure_http" type="checkbox" required /> 我确认 HTTP 会以明文传输镜像层与拉取凭据。</label>
      <div class="form-group"><label class="form-label">访问地址</label><input v-model.trim="form.endpoint" class="form-input" required :disabled="Boolean(registry)" placeholder="registry.example.com:31813" /><p class="form-hint">平台会创建对应 Host 的 Ingress，将该地址转发到 Registry；后续节点用它推送和拉取镜像，创建后不可直接修改。</p><p v-if="selectedCertificate && !firstExactCertificateDomain(selectedCertificate)" class="form-hint">所选证书仅包含通配符域名，请填写其覆盖的具体子域名。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">拉取账号</label><input v-model.trim="form.pull_username" class="form-input" required placeholder="cylism-pull" /></div><div class="form-group"><label class="form-label">{{ registry ? '新拉取密码（留空保持不变）' : '拉取密码' }}</label><input v-model="form.pull_password" class="form-input" type="password" :required="!registry" minlength="12" autocomplete="new-password" /></div></div>
      <div class="modal-actions"><button class="btn" type="button" @click="closeForm">取消</button><button class="btn btn-primary" :disabled="submitting || !storagePreflight?.ready || (!form.insecure_http && (!form.certificate_name || certificatesLoading))">{{ submitting ? '保存中...' : registry ? '保存配置' : '开始部署' }}</button></div>
    </form></div></div></Teleport>

    <Teleport to="body"><div v-if="deleteOpen" class="overlay registry-overlay" @click.self="deleteOpen = false"><div class="modal"><h2 class="modal-title">删除制品库</h2><p class="confirm-copy">会删除 Deployment、Service、Ingress 和认证 Secret，但会保留 PVC <code>{{ registry.pvc_name }}</code> 中的镜像数据。</p><div class="modal-actions"><button class="btn" @click="deleteOpen = false">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteRegistry">确认删除</button></div></div></div></Teleport>
    <Teleport to="body"><div v-if="catalogDeleteTarget" class="overlay registry-overlay" @click.self="catalogDeleteTarget = null"><div class="modal registry-delete-modal"><h2 class="modal-title">{{ catalogDeleteTarget.tag ? '删除镜像标签' : '删除镜像仓库' }}</h2><p class="confirm-copy">{{ catalogDeleteTarget.tag ? `将删除 ${catalogDeleteTarget.repository} 中指向该 manifest 的标签。` : `将删除 ${catalogDeleteTarget.repository} 中所有未被引用的 manifest。` }}</p><div v-if="catalogDeleteTarget.affected_tags?.length" class="registry-impact"><strong>受影响标签</strong><code>{{ catalogDeleteTarget.affected_tags.join('、') }}</code></div><div v-if="catalogDeleteTarget.references?.length" class="registry-impact is-blocked"><strong>无法删除，仍被引用</strong><span v-for="reference in catalogDeleteTarget.references" :key="reference.kind + reference.name">{{ reference.kind === 'release' ? '发布' : '模板' }}：{{ reference.name }}</span></div><div class="modal-actions"><button class="btn" :disabled="catalogDeleting" @click="catalogDeleteTarget = null">取消</button><button class="btn btn-danger" :disabled="catalogDeleting || Boolean(catalogDeleteTarget.references?.length)" @click="confirmCatalogDelete">{{ catalogDeleting ? '删除中...' : '确认删除' }}</button></div></div></div></Teleport>
    <Teleport to="body"><div v-if="actionError" class="overlay registry-notice-overlay" @click.self="actionError = null"><section class="modal registry-notice-modal" role="alertdialog" aria-modal="true" :aria-label="actionError.title"><div class="registry-notice-icon"><AlertCircle :size="22" /></div><h2 class="modal-title">{{ actionError.title }}</h2><p class="registry-notice-text">{{ actionError.message }}</p><div class="modal-actions"><button class="btn btn-danger" @click="actionError = null">确定</button></div></section></div></Teleport>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { AlertCircle, Copy, Database, Plus, RefreshCw, Search, Trash2 } from 'lucide-vue-next'
import { createManagedRegistry, deleteManagedRegistry, deleteManagedRegistryRepository, deleteManagedRegistryTag, getManagedRegistryCatalog, getManagedRegistryCatalogTags, getManagedRegistryCertificates, getManagedRegistryResources, preflightManagedRegistryRepositoryDelete, preflightManagedRegistryTagDelete, repairManagedRegistry, updateManagedRegistry } from '../api/managed-oci-registries.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import EmptyState from '../components/EmptyState.vue'
import FeedbackBanner from '../components/FeedbackBanner.vue'
import RegistryProxyWorkspace from '../components/RegistryProxyWorkspace.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'

const registry = ref(null); const storagePreflight = ref(null); const pvcOptions = ref([]); const certificateOptions = ref([]); const certificateError = ref(''); const loaded = ref(false); const refreshing = ref(false); const repairing = ref(false); const pageError = ref(''); const actionError = ref(null); const showForm = ref(false); const deleteOpen = ref(false); const submitting = ref(false); const form = ref(newForm())
const workspaceTabs = [{ id: 'registry', label: '自托管制品库' }, { id: 'registry-proxy', label: 'Registry Proxy' }]
const activeWorkspace = ref(new URLSearchParams(window.location.hash.split('?')[1] || '').get('tab') === 'registry-proxy' ? 'registry-proxy' : 'registry')
const catalogRepositories = ref([]); const catalogNext = ref(''); const catalogLoading = ref(false); const catalogLoaded = ref(false); const catalogError = ref(''); const catalogQuery = ref(''); const selectedRepository = ref(''); const tags = ref([]); const tagsNext = ref(''); const tagsLoading = ref(false); const tagsLoaded = ref(false); const catalogDeleteTarget = ref(null); const catalogDeleting = ref(false)
const registryResource = useAsyncResource(({ signal }) => getManagedRegistryResources({ signal }), null)
const certificateResource = useAsyncResource(({ signal }) => getManagedRegistryCertificates('cylism-system', { signal }), null)
const pvcOptionsLoading = registryResource.loading
const certificatesLoading = certificateResource.loading
const registryDataNodes = computed(() => storagePreflight.value?.data_nodes || [])
const selectedPVC = computed(() => pvcOptions.value.find(item => item.name === form.value.pvc_name) || null)
const selectedCertificate = computed(() => certificateOptions.value.find(item => item.name === form.value.certificate_name) || null)
const needsRepair = computed(() => ['degraded', 'failed', 'pending'].includes(registry.value?.status))
const filteredRepositories = computed(() => { const query = catalogQuery.value.toLowerCase(); return catalogRepositories.value.filter(item => item.toLowerCase().includes(query)) })
function newForm() { return { name: '', namespace: 'cylism-system', endpoint: '', verification_image: '', registry_image: 'registry:2', data_node: '', pvc_name: '', cpu_request: '100m', cpu_limit: '500m', memory_request: '256Mi', memory_limit: '1Gi', insecure_http: false, confirm_insecure_http: false, certificate_name: '', pull_username: 'cylism-pull', pull_password: '', project_ids: [] } }
function endpointURL(item) { return `${item.insecure_http ? 'http' : 'https'}://${item.endpoint}` }
function statusLabel(status) { return ({ ready: '就绪', deploying: '部署中', failed: '失败', degraded: '异常', pending: '待部署', migration_required: '需迁移' })[status] || '未知' }
function statusBadgeClass(status) { return status === 'ready' ? 'badge-online' : status === 'failed' || status === 'degraded' || status === 'migration_required' ? 'badge-danger' : 'badge-deploying' }
function showActionError(title, err, fallback) { actionError.value = { title, message: err.message || fallback } }
async function load() { refreshing.value = true; pageError.value = ''; const result = await registryResource.refresh(); if (result !== undefined) { const [registries, preflight, pvcs] = result; registry.value = registries?.[0] || null; storagePreflight.value = preflight; pvcOptions.value = pvcs || []; if (registry.value && !catalogLoaded.value) await loadCatalog() } else if (registryResource.error.value) pageError.value = registryResource.error.value.message || '加载制品库失败'; loaded.value = true; refreshing.value = false }
function openCreate() { actionError.value = null; form.value = newForm(); showForm.value = true }
function openEdit() { actionError.value = null; const item = registry.value; form.value = { ...newForm(), name: item.name, endpoint: item.endpoint, verification_image: item.verification_image || '', registry_image: item.registry_image, data_node: item.data_node, pvc_name: item.pvc_name, cpu_request: item.cpu_request, cpu_limit: item.cpu_limit, memory_request: item.memory_request, memory_limit: item.memory_limit, insecure_http: item.insecure_http, confirm_insecure_http: item.insecure_http, certificate_name: item.certificate_name, pull_username: item.pull_username }; showForm.value = true }
function closeForm() { showForm.value = false; form.value = newForm(); certificateOptions.value = []; certificateError.value = '' }
function firstExactCertificateDomain(certificate) { return certificate?.domains?.find(domain => !domain.startsWith('*.')) || '' }
function selectCertificate() { if (registry.value) return; form.value.endpoint = firstExactCertificateDomain(selectedCertificate.value) }
async function loadCertificates() { certificateError.value = ''; if (!showForm.value || form.value.insecure_http) { certificateResource.cancel(); certificateOptions.value = []; return } const options = await certificateResource.refresh(); if (options !== undefined) { certificateOptions.value = options || []; if (!registry.value && !certificateOptions.value.some(item => item.name === form.value.certificate_name)) form.value.certificate_name = '' } else if (certificateResource.error.value) certificateError.value = certificateResource.error.value.message || '读取可用证书失败' }
function endpointHostname() { try { return new URL(`https://${form.value.endpoint}`).hostname } catch { return '' } }
function certificateCreateLink() { const params = new URLSearchParams({ create: '1', namespace: 'cylism-system' }); const hostname = endpointHostname(); if (hostname) params.set('domains', hostname); return `#/network?tab=certificates&${params}` }
function pvcCreateLink() { const params = new URLSearchParams({ create: '1', namespace: 'cylism-system', name: 'cylism-oci-registry-data', storage_class_name: 'local-path', storage: '100Gi' }); if (form.value.data_node) params.set('node_name', form.value.data_node); return `#/storage?${params}` }
async function save() { submitting.value = true; actionError.value = null; try { const payload = { ...form.value }; if (registry.value && !payload.pull_password) delete payload.pull_password; if (registry.value) await updateManagedRegistry(registry.value.id, payload); else await createManagedRegistry(payload); closeForm(); await load() } catch (err) { showActionError(registry.value ? '保存配置失败' : '部署失败', err, '保存制品库失败') } finally { submitting.value = false } }
async function repairRegistry() { if (!registry.value) return; repairing.value = true; actionError.value = null; try { await repairManagedRegistry(registry.value.id); await load() } catch (err) { showActionError('修复制品库失败', err, '重新同步制品库资源失败') } finally { repairing.value = false } }
async function deleteRegistry() { submitting.value = true; actionError.value = null; try { await deleteManagedRegistry(registry.value.id, { confirm: true }); deleteOpen.value = false; await load() } catch (err) { showActionError('删除制品库失败', err, '删除制品库失败') } finally { submitting.value = false } }
function selectWorkspace(workspace) { activeWorkspace.value = workspace }
async function loadCatalog(cursor = '') { if (!registry.value) return; catalogLoading.value = true; catalogError.value = ''; try { const page = await getManagedRegistryCatalog(registry.value.id, cursor); catalogRepositories.value = cursor ? [...catalogRepositories.value, ...(page.repositories || [])] : (page.repositories || []); catalogNext.value = page.next || ''; catalogLoaded.value = true } catch (err) { catalogError.value = err.message || '读取镜像目录失败' } finally { catalogLoading.value = false } }
async function selectRepository(repository) { selectedRepository.value = repository; tags.value = []; tagsNext.value = ''; tagsLoaded.value = false; await loadTags(repository) }
async function loadTags(repository, cursor = '') { if (!registry.value || !repository) return; tagsLoading.value = true; catalogError.value = ''; try { const page = await getManagedRegistryCatalogTags(registry.value.id, repository, cursor); tags.value = cursor ? [...tags.value, ...(page.tags || [])] : (page.tags || []); tagsNext.value = page.next || ''; tagsLoaded.value = true } catch (err) { catalogError.value = err.message || '读取镜像标签失败' } finally { tagsLoading.value = false } }
function shortDigest(value) { return value ? `${value.slice(0, 19)}...` : '-' }
async function copyPullReference(reference) { try { await navigator.clipboard?.writeText(`${registry.value.endpoint}/${reference}`) } catch { actionError.value = { title: '复制失败', message: '浏览器未授权访问剪贴板。' } } }
async function prepareTagDelete(tag) { try { catalogDeleteTarget.value = await preflightManagedRegistryTagDelete(registry.value.id, { repository: selectedRepository.value, tag: tag.name }) } catch (err) { showActionError('无法删除镜像标签', err, '读取删除影响失败') } }
async function prepareRepositoryDelete(repository) { try { catalogDeleteTarget.value = await preflightManagedRegistryRepositoryDelete(registry.value.id, { repository }) } catch (err) { showActionError('无法删除镜像仓库', err, '读取删除影响失败') } }
async function confirmCatalogDelete() { const target = catalogDeleteTarget.value; if (!target) return; catalogDeleting.value = true; try { const payload = { repository: target.repository, digest: target.digest, affected_tags: target.affected_tags, confirm: true }; if (target.tag) { payload.tag = target.tag; await deleteManagedRegistryTag(registry.value.id, payload) } else await deleteManagedRegistryRepository(registry.value.id, payload); catalogDeleteTarget.value = null; if (target.tag) await loadTags(target.repository); else { selectedRepository.value = ''; tags.value = []; await loadCatalog() } } catch (err) { showActionError('删除镜像失败', err, '删除镜像失败，请刷新后重试') } finally { catalogDeleting.value = false } }
onMounted(load)
watch([showForm, () => form.value.insecure_http], loadCertificates)
watch(() => selectedPVC.value?.bound_node, boundNode => { if (boundNode) form.value.data_node = boundNode })
</script>

<style scoped>
.section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-16); }
.section-heading p, .form-hint, .registry-empty p, .confirm-copy { margin: 4px 0 0; color: var(--text-secondary); font-size: 13px; }
.registry-empty { display: flex; align-items: center; gap: 16px; margin-top: 28px; padding: 24px 0; border-top: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.registry-empty h2 { margin: 0; font-size: 16px; }

.registry-status-actions { display: flex; align-items: center; justify-content: flex-end; gap: var(--space-8); margin: 0 0 var(--space-20); }
.registry-catalog { padding-top: var(--space-4); border-top: 1px solid var(--border-muted); }
.registry-catalog-toolbar { display: flex; min-height: 48px; align-items: center; justify-content: space-between; gap: var(--space-16); margin-bottom: var(--space-16); padding-bottom: var(--space-12); border-bottom: 1px solid var(--border-muted); }
.registry-search { display: block; width: min(360px, 100%); }
.registry-search-input { display: flex; min-height: 36px; align-items: center; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); }
.registry-search-input:focus-within { border-color: var(--focus); outline: 2px solid var(--focus); outline-offset: 2px; }
.registry-search-input svg { flex: 0 0 auto; margin-left: 10px; color: var(--text-muted); }
.registry-search .form-input { min-height: 34px; padding: 8px 10px; border: 0; outline: 0; background: transparent; font-size: 12px; }
.registry-catalog-count { color: var(--text-secondary); font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.registry-catalog-state { min-height: 300px; padding: var(--space-16); }
.registry-catalog-state :deep(.feedback-banner) { align-items: center; margin-bottom: 0; }
.registry-catalog-state :deep(.feedback-banner-content) { display: flex; width: 100%; min-width: 0; align-items: center; justify-content: space-between; gap: var(--space-12); }
.registry-catalog-state :deep(.empty-state) { min-height: 206px; }
.registry-catalog-grid { display: grid; grid-template-columns: minmax(220px, .75fr) minmax(0, 1.7fr); gap: var(--space-16); align-items: start; }
.registry-repository-list, .registry-tags-panel { min-height: 300px; padding: 0; overflow: hidden; }
.registry-list-heading { display: flex; align-items: center; justify-content: space-between; min-height: 56px; padding: 12px 16px; border-bottom: 1px solid var(--border-muted); }
.registry-list-heading h2 { margin: 0; font-size: 14px; }
.registry-list-heading p { margin: 3px 0 0; color: var(--text-muted); font-size: 11px; }
.registry-repository-row { display: flex; align-items: stretch; min-width: 0; border-bottom: 1px solid var(--border-muted); }
.registry-repository-row.is-selected { background: var(--surface-subtle); box-shadow: inset 2px 0 0 var(--action-primary); }
.registry-repository-select { min-width: 0; flex: 1; padding: 11px 12px 11px 16px; border: 0; background: transparent; color: var(--text-primary); font: inherit; font-family: var(--font-mono); font-size: 12px; overflow-wrap: anywhere; text-align: left; cursor: pointer; }
.registry-repository-select:hover { color: var(--action-primary); }
.registry-row-icon { align-self: center; margin-right: 7px; }
.registry-load-more { display: flex; justify-content: center; padding: 12px; }
.registry-tags-table { display: grid; }
.registry-tag-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; padding: 12px 16px; border-bottom: 1px solid var(--border-muted); }
.registry-tag-row > div:first-child { display: grid; min-width: 0; gap: 4px; }
.registry-tag-row strong { font-size: 13px; }
.registry-tag-row code { color: var(--text-secondary); font-size: 11px; overflow-wrap: anywhere; }
.registry-tag-row small { color: var(--text-muted); font-size: 11px; }
.registry-tag-actions { display: flex; flex: 0 0 auto; gap: 4px; }
.danger-icon { color: var(--danger); }
.registry-delete-modal { width: min(480px, calc(100vw - 32px)); }
.registry-impact { display: grid; gap: 6px; max-height: 130px; margin-top: 16px; padding: 10px; overflow: auto; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; }
.registry-impact strong { color: var(--text-primary); font-size: 12px; }
.registry-impact code { overflow-wrap: anywhere; }
.registry-impact.is-blocked { border-color: var(--danger); background: var(--danger-surface); color: var(--danger); }

.registry-overview { margin-bottom: var(--space-16); }
.registry-metric { display: grid; min-width: 0; min-height: 126px; align-content: start; gap: 8px; }
.registry-metric > span { color: var(--text-muted); font-size: 11px; }
.registry-metric strong { min-width: 0; color: var(--text-primary); font-size: 15px; line-height: 1.35; overflow-wrap: anywhere; }
.registry-metric small { color: var(--text-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.registry-endpoint { color: var(--action-primary) !important; font-family: var(--font-mono); }

.volume-heading { display: flex; align-items: center; gap: 8px; }
.volume-heading { justify-content: space-between; margin-bottom: 8px; }

.transport-options { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.transport-options label { display: flex; align-items: center; gap: 8px; padding: 9px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); font-size: 13px; }
.resource-config { margin: 20px 0; padding: 16px 0; border-top: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.resource-config h3 { margin: 0; font-size: 14px; }
.resource-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 14px; }
.registry-modal { width: min(620px, calc(100vw - 32px)); }
.registry-overlay { z-index: 3000; }
.registry-notice-overlay { z-index: 3100; }
.registry-notice-modal { width: min(420px, calc(100vw - 32px)); }
.registry-notice-icon { display: grid; width: 46px; height: 46px; margin-bottom: 14px; place-items: center; border-radius: 50%; background: var(--danger-surface); color: var(--danger); }
.registry-notice-modal .modal-title { margin-bottom: 0; }
.registry-notice-text { margin: 10px 0 0; color: var(--text-secondary); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.risk-confirm { display: flex; gap: 8px; margin: 0 0 14px; color: var(--danger); font-size: 13px; }
.is-spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 760px) {
  .registry-catalog-grid { grid-template-columns: 1fr; }
  .resource-grid { grid-template-columns: 1fr; }
}
@media (max-width: 480px) {
  .registry-catalog-toolbar { align-items: stretch; flex-direction: column; }
  .registry-catalog-count { text-align: right; }
  .registry-catalog-state :deep(.feedback-banner-content) { align-items: stretch; flex-direction: column; }
  .registry-catalog-state :deep(.feedback-banner .btn) { width: 100%; }
  .registry-status-actions { justify-content: flex-start; flex-wrap: wrap; }
}
</style>
