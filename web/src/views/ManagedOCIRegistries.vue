<template>
  <div class="page-shell registry-page">
    <header class="page-header">
      <div>
        <h1 class="page-title">自托管制品库</h1>
        <p class="page-subtitle">由 Cylism 在集群中部署和管理的 OCI 制品库，为平台应用提供私有镜像存储与分发能力。</p>
      </div>
      <button data-testid="deploy-registry" class="btn btn-primary" :disabled="!loaded || Boolean(registry)" @click="openCreate"><Plus :size="16" /> 部署制品库</button>
    </header>

    <p v-if="error" class="form-error" role="alert">{{ error }}</p>
    <div v-if="!loaded" class="empty-state">正在读取制品库配置...</div>
    <section v-else-if="!registry" class="registry-empty" data-testid="registry-empty">
      <Database :size="26" />
      <div><h2>尚未部署自托管制品库</h2><p>部署后，平台会创建受认证保护、使用持久化存储并通过集群入口提供服务的 OCI Registry。</p></div>
    </section>
    <template v-else>
      <section class="registry-summary" data-testid="registry-summary">
        <div><span class="metric-label">入口地址</span><strong>{{ endpointURL(registry) }}</strong><small>{{ registry.insecure_http ? 'HTTP，凭据和镜像层以明文传输' : 'HTTPS，使用已配置 TLS Secret' }}</small></div>
        <div><span class="metric-label">运行状态</span><strong :class="statusClass(registry.status)">{{ statusLabel(registry.status) }}</strong><small>{{ registry.last_error || 'Registry 工作负载状态已同步' }}</small></div>
        <div><span class="metric-label">数据位置</span><strong>{{ registry.data_node }}</strong><small>{{ registry.data_path }}</small></div>
        <div><span class="metric-label">节点下发</span><strong>{{ mirrorStatus(registry) }}</strong><small>选择节点后写入 K3s registries.yaml</small></div>
      </section>

      <section class="registry-details">
        <div class="section-heading"><div><h2>Registry 配置</h2><p>凭据只以加密形式保存，不会再次展示。</p></div><div class="section-actions"><button class="icon-button" title="刷新状态" aria-label="刷新状态" :disabled="refreshing" @click="load"><RefreshCw :size="17" :class="{ 'is-spinning': refreshing }" /></button><button class="btn btn-secondary" @click="openEdit">编辑</button><button class="btn btn-danger" @click="deleteOpen = true">删除</button></div></div>
        <dl class="detail-grid"><div><dt>Registry 镜像</dt><dd>{{ registry.registry_image }}</dd></div><div><dt>命名空间</dt><dd>{{ registry.namespace }}</dd></div><div><dt>拉取账号</dt><dd>{{ registry.pull_username }}</dd></div><div><dt>TLS Secret</dt><dd>{{ registry.insecure_http ? '不使用（HTTP）' : registry.tls_secret_name }}</dd></div></dl>
      </section>

      <section class="registry-details">
        <div class="section-heading"><div><h2>节点镜像配置</h2><p>只会写入所选集群节点，并重启对应的 K3s 服务。</p></div><button class="btn btn-secondary" :disabled="selectedNodes.length === 0 || submitting" @click="applyNodes"><Send :size="16" /> 下发至 {{ selectedNodes.length }} 个节点</button></div>
        <div v-if="clusterNodes.length" class="node-options"><label v-for="server in clusterNodes" :key="server.id" class="node-option"><input v-model="selectedNodes" :value="server.id" type="checkbox" /><span><strong>{{ server.k8s_node_name || server.name }}</strong><small>{{ server.host }} · {{ server.cluster_role }}</small></span></label></div>
        <p v-else class="form-hint">尚无已绑定的 K3s 节点。</p>
      </section>
    </template>

    <div v-if="showForm" class="overlay" @click.self="closeForm"><div class="modal registry-modal"><h2 class="modal-title">{{ registry ? '编辑制品库' : '部署制品库' }}</h2><form @submit.prevent="save">
      <div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required placeholder="平台制品库" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">命名空间</label><input v-model.trim="form.namespace" class="form-input" required placeholder="cylism-system" /></div><div class="form-group"><label class="form-label">Registry 镜像</label><input v-model.trim="form.registry_image" class="form-input" required placeholder="registry:2" /></div></div>
      <div class="form-group"><label class="form-label">访问域名</label><input v-model.trim="form.endpoint" class="form-input" required :disabled="Boolean(registry)" placeholder="registry.example.com" /><p class="form-hint">必须是可解析到集群入口的域名；创建后不可直接修改。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">数据节点</label><input v-model.trim="form.data_node" class="form-input" required placeholder="k3s-data-node" /></div><div class="form-group"><label class="form-label">数据目录</label><input v-model.trim="form.data_path" class="form-input" required placeholder="/data/cylism-registry" /></div></div>
      <div class="form-group"><label class="form-label">传输方式</label><div class="transport-options"><label><input v-model="form.insecure_http" :value="false" type="radio" /> HTTPS</label><label><input v-model="form.insecure_http" :value="true" type="radio" /> HTTP</label></div></div>
      <div v-if="!form.insecure_http" class="form-group"><label class="form-label">TLS Secret</label><input v-model.trim="form.tls_secret_name" class="form-input" required placeholder="registry-tls" /></div>
      <label v-else class="risk-confirm"><input v-model="form.confirm_insecure_http" type="checkbox" required /> 我确认 HTTP 会以明文传输镜像层与拉取凭据。</label>
      <div class="form-row"><div class="form-group"><label class="form-label">拉取账号</label><input v-model.trim="form.pull_username" class="form-input" required placeholder="cylism-pull" /></div><div class="form-group"><label class="form-label">{{ registry ? '新拉取密码（留空保持不变）' : '拉取密码' }}</label><input v-model="form.pull_password" class="form-input" type="password" :required="!registry" minlength="12" autocomplete="new-password" /></div></div>
      <div class="modal-actions"><button class="btn" type="button" @click="closeForm">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : registry ? '保存配置' : '开始部署' }}</button></div>
    </form></div></div>

    <div v-if="deleteOpen" class="overlay" @click.self="deleteOpen = false"><div class="modal"><h2 class="modal-title">删除制品库</h2><p class="confirm-copy">会删除 Deployment、Service、Ingress 和认证 Secret。节点 <code>{{ registry.data_path }}</code> 中的镜像数据会保留，不会自动删除。</p><div class="modal-actions"><button class="btn" @click="deleteOpen = false">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteRegistry">确认删除</button></div></div></div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Database, Plus, RefreshCw, Send } from 'lucide-vue-next'
import { api } from '../api/index.js'

const registry = ref(null); const servers = ref([]); const loaded = ref(false); const refreshing = ref(false); const error = ref(''); const showForm = ref(false); const deleteOpen = ref(false); const submitting = ref(false); const selectedNodes = ref([]); const form = ref(newForm())
const clusterNodes = computed(() => servers.value.filter(server => server.cluster_role))
function newForm() { return { name: '', namespace: 'cylism-system', endpoint: '', registry_image: 'registry:2', data_node: '', data_path: '/data/cylism-registry', insecure_http: false, confirm_insecure_http: false, tls_secret_name: '', pull_username: 'cylism-pull', pull_password: '', project_ids: [] } }
function endpointURL(item) { return `${item.insecure_http ? 'http' : 'https'}://${item.endpoint}` }
function statusLabel(status) { return ({ ready: '就绪', deploying: '部署中', failed: '失败', degraded: '异常', pending: '待部署' })[status] || '未知' }
function statusClass(status) { return status === 'ready' ? 'status-ready' : status === 'failed' || status === 'degraded' ? 'status-failed' : 'status-pending' }
function mirrorStatus(item) { return item.node_registry_mirror_id ? '可下发' : '未配置' }
async function load() { refreshing.value = true; error.value = ''; try { const [registries, items] = await Promise.all([api.get('/managed-oci-registries'), api.get('/servers')]); registry.value = registries[0] || null; servers.value = items || [] } catch (err) { error.value = err.message || '加载制品库失败' } finally { loaded.value = true; refreshing.value = false } }
function openCreate() { form.value = newForm(); showForm.value = true }
function openEdit() { const item = registry.value; form.value = { ...newForm(), name: item.name, namespace: item.namespace, endpoint: item.endpoint, registry_image: item.registry_image, data_node: item.data_node, data_path: item.data_path, insecure_http: item.insecure_http, confirm_insecure_http: item.insecure_http, tls_secret_name: item.tls_secret_name, pull_username: item.pull_username }; showForm.value = true }
function closeForm() { showForm.value = false; form.value = newForm() }
async function save() { submitting.value = true; error.value = ''; try { const payload = { ...form.value }; if (registry.value && !payload.pull_password) delete payload.pull_password; if (registry.value) await api.put(`/managed-oci-registries/${registry.value.id}`, payload); else await api.post('/managed-oci-registries', payload); closeForm(); await load() } catch (err) { error.value = err.message || '保存制品库失败' } finally { submitting.value = false } }
async function applyNodes() { submitting.value = true; error.value = ''; try { await api.post(`/managed-oci-registries/${registry.value.id}/apply-node-access`, { server_ids: selectedNodes.value }); await load() } catch (err) { error.value = err.message || '下发节点配置失败' } finally { submitting.value = false } }
async function deleteRegistry() { submitting.value = true; error.value = ''; try { await api.delete(`/managed-oci-registries/${registry.value.id}`, { confirm: true }); deleteOpen.value = false; selectedNodes.value = []; await load() } catch (err) { error.value = err.message || '删除制品库失败' } finally { submitting.value = false } }
onMounted(load)
</script>

<style scoped>
.page-header,.section-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }.page-subtitle,.section-heading p,.form-hint,.registry-empty p,.confirm-copy { margin:4px 0 0; color:var(--text-secondary); font-size:13px; }.registry-empty { display:flex; align-items:center; gap:16px; margin-top:28px; padding:24px 0; border-top:1px solid var(--border-muted); border-bottom:1px solid var(--border-muted); }.registry-empty h2,.section-heading h2 { margin:0; font-size:16px; }.registry-summary { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:1px; margin:26px 0; background:var(--border-muted); border:1px solid var(--border-muted); }.registry-summary>div { min-width:0; padding:16px; background:var(--surface); }.metric-label { display:block; margin-bottom:6px; color:var(--text-muted); font-size:11px; }.registry-summary strong,.registry-summary small { display:block; overflow-wrap:anywhere; }.registry-summary strong { font-size:14px; }.registry-summary small { margin-top:6px; color:var(--text-secondary); font-size:12px; }.status-ready { color:var(--success); }.status-failed { color:var(--danger); }.status-pending { color:var(--warning); }.registry-details { padding:20px 0; border-top:1px solid var(--border-muted); }.section-actions { display:flex; align-items:center; gap:8px; }.detail-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; margin:18px 0 0; }.detail-grid dt { color:var(--text-muted); font-size:12px; }.detail-grid dd { margin:4px 0 0; overflow-wrap:anywhere; font-size:13px; }.node-options,.transport-options { display:flex; flex-wrap:wrap; gap:8px; margin-top:18px; }.node-option,.transport-options label { display:flex; align-items:center; gap:8px; padding:9px 10px; border:1px solid var(--border-muted); border-radius:var(--radius-control); font-size:13px; }.node-option span { display:grid; gap:2px; }.node-option small { color:var(--text-muted); font-size:11px; }.registry-modal { width:min(620px,calc(100vw - 32px)); }.risk-confirm { display:flex; gap:8px; margin:0 0 14px; color:var(--danger); font-size:13px; }.is-spinning { animation:spin .8s linear infinite; }@keyframes spin { to { transform:rotate(360deg); } }@media(max-width:760px){.page-header,.section-heading{flex-direction:column;align-items:stretch}.registry-summary{grid-template-columns:1fr 1fr}.registry-empty{align-items:flex-start}.detail-grid{grid-template-columns:1fr}}@media(max-width:480px){.registry-summary{grid-template-columns:1fr}}
</style>
