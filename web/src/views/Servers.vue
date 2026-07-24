<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">服务器</h1>
      <button v-if="activeTab === 'servers'" class="btn btn-primary" @click="showAdd = true">+ 添加服务器</button>
    </div>

    <!-- Tab switcher -->
    <div class="card" style="margin-bottom:16px">
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab === 'servers' }]" @click="activeTab = 'servers'">服务器列表</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'nodes' }]" @click="activeTab = 'nodes'">集群节点</button>
      </div>
    </div>

    <!-- 服务器列表 Tab -->
    <div v-if="activeTab === 'servers'" class="card">
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>名称</th><th>主机</th><th>SSH 端口</th><th>集群角色</th><th>节点名</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id">
              <td style="font-weight:600">{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td>{{ srv.ssh_port }}</td>
              <td>
                <span class="badge" :class="srv.cluster_role ? 'badge-online' : 'badge-offline'">
                  {{ srv.cluster_role || '未加入' }}
                </span>
              </td>
              <td>{{ srv.k8s_node_name || '-' }}</td>
              <td>
                <div class="btn-group" style="justify-content:flex-end;">
                  <button v-if="!srv.cluster_role" class="btn btn-sm" @click="addToCluster(srv.id)">加入集群</button>
                  <button v-if="srv.cluster_role" class="btn btn-sm" @click="drainNode(srv.id)">驱逐</button>
                  <button v-if="srv.cluster_role" class="btn btn-sm btn-danger" @click="confirmRemoveNode(srv)">移出</button>
                  <button v-if="!srv.cluster_role" class="btn btn-sm btn-danger" @click="confirmDelete(srv)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 集群节点 Tab -->
    <div v-if="activeTab === 'nodes'" class="card">
      <div v-if="nodes.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 K8s 节点</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>节点名</th><th>状态</th><th>角色</th><th>K8s 版本</th><th>IP</th><th>OS</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodes" :key="node.name">
              <td style="font-weight:600">{{ node.name }}</td>
              <td>
                <span class="badge" :class="node.ready ? 'badge-online' : 'badge-offline'">
                  <span class="badge-dot"></span> {{ node.ready ? '就绪' : '未就绪' }}
                </span>
              </td>
              <td>{{ node.roles || '-' }}</td>
              <td>{{ node.version || '-' }}</td>
              <td>{{ node.internal_ip || '-' }}</td>
              <td style="font-size:12px;color:var(--text-secondary);">{{ node.os || '-' }}</td>
              <td>
                <div class="btn-group" style="justify-content:flex-end;">
                  <button class="btn btn-sm" @click="drainNode(node.name)">驱逐</button>
                  <button class="btn btn-sm btn-danger" @click="confirmRemoveNode({ name: node.name, k8s_node_name: node.name })">移出</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add Server modal -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">添加服务器</h2>
        <form @submit.prevent="addServer">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">名称</label>
              <input v-model="form.name" class="form-input" placeholder="我的服务器" required />
            </div>
            <div class="form-group">
              <label class="form-label">主机</label>
              <input v-model="form.host" class="form-input" placeholder="192.168.1.100" required />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">SSH 端口</label>
              <input v-model.number="form.ssh_port" class="form-input" type="number" placeholder="22" />
            </div>
            <div class="form-group">
              <label class="form-label">SSH 用户</label>
              <input v-model="form.ssh_user" class="form-input" placeholder="root" />
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">认证方式</label>
            <select v-model="form.ssh_auth_type" class="form-select">
              <option value="password">密码</option>
              <option value="key">密钥</option>
            </select>
          </div>
          <div class="form-group" v-if="form.ssh_auth_type === 'password'">
            <label class="form-label">SSH 密码</label>
            <input v-model="form.ssh_password" class="form-input" type="password" placeholder="输入密码" />
          </div>
          <div class="form-group" v-if="form.ssh_auth_type === 'key'">
            <label class="form-label">SSH 密钥</label>
            <textarea v-model="form.ssh_key" class="form-input" style="min-height:100px;font-family:monospace;" placeholder="粘贴私钥内容" />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn" @click="showAdd = false">取消</button>
            <button type="submit" class="btn btn-primary">确认添加</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Add to cluster confirm -->
    <div v-if="addTarget" class="overlay" @click.self="addTarget = null">
      <div class="modal">
        <h2 class="modal-title">加入集群</h2>
        <p style="color:var(--text-secondary);margin-bottom:16px;">
          确定将 <strong>{{ addTarget.name }}</strong> 加入 K3s 集群吗？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="addTarget = null">取消</button>
          <button class="btn btn-primary" @click="doAddToCluster">确认</button>
        </div>
      </div>
    </div>

    <!-- Drain confirm -->
    <div v-if="drainTarget" class="overlay" @click.self="drainTarget = null">
      <div class="modal">
        <h2 class="modal-title">驱逐节点</h2>
        <p style="color:var(--text-secondary);margin-bottom:16px;">
          确定驱逐 <strong>{{ drainTarget.name || drainTarget.k8s_node_name }}</strong> 吗？Pod 会迁移到其他节点。
        </p>
        <div class="modal-actions">
          <button class="btn" @click="drainTarget = null">取消</button>
          <button class="btn btn-danger" @click="doDrain">确认驱逐</button>
        </div>
      </div>
    </div>

    <!-- Remove node confirm -->
    <div v-if="removeTarget" class="overlay" @click.self="removeTarget = null">
      <div class="modal">
        <h2 class="modal-title">移出集群</h2>
        <p style="color:var(--text-secondary);margin-bottom:16px;">
          确定将 <strong>{{ removeTarget.name }}</strong> 从集群中移出吗？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="removeTarget = null">取消</button>
          <button class="btn btn-danger" @click="doRemoveNode">确认移出</button>
        </div>
      </div>
    </div>

    <!-- Delete server confirm -->
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal">
        <h2 class="modal-title">删除服务器</h2>
        <p style="color:var(--text-secondary);margin-bottom:16px;">
          确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="deleteTarget = null">取消</button>
          <button class="btn btn-danger" @click="deleteServer">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const activeTab = ref('servers')
const servers = ref([])
const nodes = ref([])
const showAdd = ref(false)
const deleteTarget = ref(null)
const addTarget = ref(null)
const drainTarget = ref(null)
const removeTarget = ref(null)
const form = ref({ name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' })

onMounted(() => {
  fetchServers()
  fetchNodes()
})

async function fetchServers() {
  try { const r = await api.get('/servers'); servers.value = await r.json() } catch (e) { console.error(e) }
}
async function fetchNodes() {
  try { const r = await api.get('/nodes'); nodes.value = await r.json() } catch (e) { console.error(e) }
}

async function addServer() {
  try { await api.post('/servers', form.value); showAdd.value = false; resetForm(); fetchServers() } catch (e) { console.error(e) }
}

function addToCluster(id) {
  addTarget.value = servers.value.find(s => s.id === id)
}
async function doAddToCluster() {
  if (!addTarget.value) return
  try {
    await api.post(`/nodes/${addTarget.value.id}/add`)
    addTarget.value = null
    fetchServers()
    fetchNodes()
  } catch (e) { console.error(e) }
}

function drainNode(idOrName) {
  drainTarget.value = servers.value.find(s => s.id === idOrName) || nodes.value.find(n => n.name === idOrName) || { name: idOrName }
}
async function doDrain() {
  if (!drainTarget.value) return
  const id = drainTarget.value.id || drainTarget.value.name
  try {
    await api.post(`/nodes/${id}/drain`)
    drainTarget.value = null
    fetchNodes()
  } catch (e) { console.error(e) }
}

function confirmRemoveNode(srv) { removeTarget.value = srv }
async function doRemoveNode() {
  if (!removeTarget.value) return
  const id = removeTarget.value.id
  try {
    await api.delete(`/nodes/${id}`)
    removeTarget.value = null
    fetchServers()
    fetchNodes()
  } catch (e) { console.error(e) }
}

function confirmDelete(srv) { deleteTarget.value = srv }
async function deleteServer() {
  try { await api.delete(`/servers/${deleteTarget.value.id}`); deleteTarget.value = null; fetchServers() } catch (e) { console.error(e) }
}

function resetForm() {
  form.value = { name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' }
}
</script>

<style scoped>
.table-tabs { display: flex; gap: 4px; }
.tab-btn { padding: 6px 16px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--bg-deep); color: var(--text-secondary); font-size: 13px; cursor: pointer; transition: all 0.15s; font-family: inherit; }
.tab-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.tab-active { background: var(--accent); border-color: var(--accent); color: var(--bg-deep); font-weight: 600; }
</style>
