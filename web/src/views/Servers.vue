<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">服务器</h1>
      <button class="btn btn-primary" @click="showAdd = true">+ 添加服务器</button>
    </div>

    <div class="card section-gap">
      <p class="section-copy">
        这里维护服务器台账、SSH 凭据与连通性。集群节点已经拆分到“集群节点”页面统一查看与操作。
      </p>
    </div>

    <div class="card">
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>名称</th><th>主机</th><th>SSH 用户</th><th>认证方式</th><th>SSH 连通</th><th>TS IP</th><th>TS 状态</th><th>集群状态</th><th>节点名</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id">
              <td class="cell-primary">{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td>{{ srv.ssh_user || 'root' }}</td>
              <td>{{ srv.ssh_auth_type === 'key' ? '密钥' : '密码' }}</td>
              <td>
                <button class="btn btn-sm" @click="probeServer(srv.id)" :disabled="probingId === srv.id">
                  {{ probingId === srv.id ? '...' : '🔍' }}
                </button>
              </td>
              <td>{{ srv.tailscale_ip || '-' }}</td>
              <td>
                <span v-if="srv.tailscale_online" class="badge badge-online">🌐 在线</span>
                <span v-else>-</span>
              </td>
              <td>
                <span class="badge" :class="srv.cluster_role ? 'badge-online' : 'badge-offline'">
                  {{ srv.cluster_role ? '已在集群' : '未加入' }}
                </span>
              </td>
              <td>{{ srv.k8s_node_name || '-' }}</td>
              <td>
                <div class="btn-group action-cell">
                  <button v-if="!srv.cluster_role" class="btn btn-sm" @click="startJoin(srv.id)">加入集群</button>
                  <button v-if="!srv.cluster_role" class="btn btn-sm" @click="startEdit(srv)">编辑</button>
                  <button v-if="!srv.cluster_role" class="btn btn-sm btn-danger" @click="confirmDelete(srv)">删除</button>
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
        <h2 class="modal-title">{{ editingId ? '编辑服务器' : '添加服务器' }}</h2>
        <form @submit.prevent="addServer">
          <div class="form-row">
            <div class="form-group"><label class="form-label">名称</label><input v-model="form.name" class="form-input" placeholder="我的服务器" required /></div>
            <div class="form-group"><label class="form-label">主机</label><input v-model="form.host" class="form-input" placeholder="192.168.1.100" required /></div>
          </div>
          <div class="form-row">
            <div class="form-group"><label class="form-label">SSH 端口</label><input v-model.number="form.ssh_port" class="form-input" type="number" placeholder="22" /></div>
            <div class="form-group"><label class="form-label">SSH 用户</label><input v-model="form.ssh_user" class="form-input" placeholder="root" /></div>
          </div>
          <div class="form-group"><label class="form-label">认证方式</label><select v-model="form.ssh_auth_type" class="form-select"><option value="password">密码</option><option value="key">密钥</option></select></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'password'"><label class="form-label">SSH 密码</label><input v-model="form.ssh_password" class="form-input" type="password" placeholder="输入密码" /></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'key'"><label class="form-label">SSH 密钥</label><textarea v-model="form.ssh_key" class="form-input textarea-input" placeholder="粘贴私钥内容" /></div>
          <div class="modal-actions"><button type="button" class="btn" @click="closeForm">取消</button><button type="submit" class="btn btn-primary">{{ editingId ? '保存修改' : '确认添加' }}</button></div>
        </form>
      </div>
    </div>

    <!-- SSH probe result modal -->
    <div v-if="probeResult" class="overlay" @click.self="probeResult = null">
      <div class="modal">
        <h2 class="modal-title">SSH 连通性检测</h2>
        <div class="detail-grid">
          <span class="detail-label">主机：</span><span>{{ probeResult.host }}</span>
          <span class="detail-label">结果：</span>
          <span>
            <span v-if="probeResult.reachable" class="badge badge-online">✅ 在线 ({{ probeResult.latency_ms }}ms)</span>
            <span v-else class="badge badge-danger">✗ 不可达</span>
          </span>
          <span v-if="!probeResult.reachable" class="detail-label">错误：</span>
          <span v-if="!probeResult.reachable">{{ probeResult.error }}</span>
        </div>
        <div class="modal-actions"><button class="btn" @click="probeResult = null">关闭</button></div>
      </div>
    </div>

    <!-- Precheck + progress modal -->
    <div v-if="joinState" class="overlay">
      <div class="modal modal-wide">
        <h2 class="modal-title">加入集群 - {{ joinServer?.name }}</h2>
        <!-- Precheck phase -->
        <div v-if="joinState.phase === 'precheck'">
          <p v-if="!joinState.checks?.length" style="margin-bottom:12px;color:var(--text-secondary)">正在执行前置检测...</p>
          <template v-else>
            <p style="margin-bottom:12px;color:var(--text-secondary)">
              {{ joinState.allPass ? '前置检测全部通过' : '前置检测未通过' }}
            </p>
            <div v-for="c in joinState.checks" :key="c.name" style="margin-bottom:6px">
              <span v-if="c.pass" class="badge badge-online">✅</span>
              <span v-else class="badge badge-danger">✗</span>
              {{ c.label }}：{{ c.detail }}
            </div>
            <div class="modal-actions" style="margin-top:16px">
              <button class="btn" @click="joinState = null">取消</button>
              <button v-if="joinState.allPass" class="btn btn-primary" @click="startJoinProgress">开始加入</button>
            </div>
          </template>
        </div>
        <!-- Progress phase -->
        <div v-if="joinState.phase === 'progress' || joinState.phase === 'done'">
          <div style="max-height:300px;overflow-y:auto;margin-bottom:12px">
            <div v-for="(log, i) in joinState.logs" :key="i" style="margin-bottom:4px;font-family:var(--font-mono);font-size:11px">
              <span v-if="log.status === 'success'" style="color:var(--color-success)">✅</span>
              <span v-else-if="log.status === 'failed'" style="color:var(--color-danger)">✗</span>
              <span v-else style="color:var(--color-accent)">⏳</span>
              {{ log.label }}：{{ log.detail }}
            </div>
          </div>
          <div v-if="joinState.phase === 'progress'" style="color:var(--text-muted);font-size:11px">
            进度：{{ joinState.logs.filter(l => l.status !== 'running').length }}/{{ joinState.total || 12 }}
          </div>
          <div v-if="joinState.phase === 'done'" class="modal-actions">
            <button class="btn btn-primary" @click="finishJoin">关闭</button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal"><h2 class="modal-title">删除服务器</h2><p class="modal-copy">确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" @click="deleteServer">确认删除</button></div></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const servers = ref([])
const showAdd = ref(false)
const editingId = ref(null)
const deleteTarget = ref(null)
const probingId = ref(null)
const probeResult = ref(null)
const joinState = ref(null)
const joinServer = ref(null)
const form = ref({ name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' })

onMounted(() => { fetchServers() })

async function fetchServers() { try { servers.value = await api.get('/servers') || [] } catch (e) { console.error(e) } }

async function addServer() {
  try {
    if (editingId.value) {
      await api.put('/servers/' + editingId.value, form.value)
    } else {
      await api.post('/servers', form.value)
    }
    closeForm()
    fetchServers()
  } catch (e) { console.error(e) }
}

function startEdit(srv) {
  editingId.value = srv.id
  form.value = {
    name: srv.name,
    host: srv.host,
    ssh_port: srv.ssh_port || 22,
    ssh_user: srv.ssh_user || 'root',
    ssh_auth_type: srv.ssh_auth_type || 'password',
    ssh_password: '',
    ssh_key: '',
  }
  showAdd.value = true
}

function closeForm() {
  showAdd.value = false
  editingId.value = null
  resetForm()
}

async function probeServer(id) {
  probingId.value = id
  try {
    const srv = servers.value.find(s => s.id === id)
    const result = await api.post(`/servers/${id}/probe`)
    probeResult.value = { host: srv?.host || '', ...result }
  } catch (e) { probeResult.value = { host: '', reachable: false, error: e.message } }
  probingId.value = null
}

async function startJoin(id) {
  const srv = servers.value.find(s => s.id === id)
  if (!srv) return
  joinServer.value = srv
  joinState.value = { phase: 'precheck', checks: [], allPass: false }
  try {
    const result = await api.post(`/servers/${id}/precheck`)
    joinState.value.checks = result.checks || []
    joinState.value.allPass = result.all_pass
  } catch (e) {
    joinState.value.checks = [{ name: 'network', label: '网络', pass: false, detail: '前置检测请求失败: ' + (e.message || '未知错误') }]
    joinState.value.allPass = false
  }
}

function startJoinProgress() {
  joinState.value.phase = 'progress'
  joinState.value.logs = []
  joinState.value.total = 12

  const id = joinServer.value.id
  const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/nodes/${id}/join-progress`
  const token = localStorage.getItem('access_token')
  const ws = new WebSocket(wsUrl + '?token=' + encodeURIComponent(token || ''))

  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data)
      joinState.value.logs.push(msg)
      if (msg.step === 'complete' || (msg.status === 'failed' && msg.index >= joinState.value.logs.length)) {
        joinState.value.phase = 'done'
        ws.close()
      }
    } catch (_) {}
  }
  ws.onerror = () => {
    if (!joinState.value.logs.some(l => l.step === 'complete' || l.status === 'failed')) {
      joinState.value.logs.push({ step: 'error', label: '连接错误', status: 'failed', detail: 'WebSocket 连接失败', index: joinState.value.logs.length + 1, total: 12, ts: new Date().toISOString() })
    }
    joinState.value.phase = 'done'
  }
  ws.onclose = () => {
    // Only mark done if no completion/failure was received
    if (!joinState.value.logs.some(l => l.step === 'complete' || l.status === 'failed')) {
      joinState.value.logs.push({ step: 'disconnected', label: '连接断开', status: 'failed', detail: 'WebSocket 连接意外关闭', index: joinState.value.logs.length + 1, total: 12, ts: new Date().toISOString() })
    }
    joinState.value.phase = 'done'
  }
}

function finishJoin() {
  joinState.value = null
  joinServer.value = null
  fetchServers()
}

function confirmDelete(srv) { deleteTarget.value = srv }
async function deleteServer() { try { await api.delete(`/servers/${deleteTarget.value.id}`); deleteTarget.value = null; fetchServers() } catch (e) { console.error(e) } }
function resetForm() { form.value = { name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' } }
</script>

<style scoped>
.modal-wide { max-width: 640px; }
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
</style>
