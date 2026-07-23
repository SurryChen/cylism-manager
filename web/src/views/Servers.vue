<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">服务器</h1>
      <button class="btn btn-primary" @click="showAdd = true">+ 添加服务器</button>
    </div>

    <div class="card">
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>名称</th><th>主机</th><th>状态</th><th>端口</th><th>SSH</th><th>最近在线</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id" @click="selectServer(srv)" :class="{ 'row-selected': selectedServer?.id === srv.id }" style="cursor:pointer">
              <td style="font-weight:600">{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td><span class="badge" :class="statusBadge(srv.status)"><span class="badge-dot"></span> {{ statusLabel(srv.status) }}</span></td>
              <td>{{ srv.port }}</td>
              <td>
                <span v-if="sshStatus[srv.id] === 'testing'" style="font-size:12px;color:var(--text-secondary);">测试中...</span>
                <span v-else-if="sshStatus[srv.id] === 'ok'" class="badge badge-online">连通</span>
                <span v-else-if="sshStatus[srv.id] === 'fail'" class="badge badge-danger">失败</span>
                <span v-else style="font-size:12px;color:var(--text-muted);">未测试</span>
              </td>
              <td>{{ formatTime(srv.last_seen) }}</td>
              <td>
                <div class="btn-group" style="justify-content: flex-end;">
                  <button class="btn btn-sm" @click="testSSH(srv.id)" :disabled="sshStatus[srv.id]==='testing'">测试</button>
                  <button class="btn btn-sm" @click="syncStatus(srv.id)" :disabled="syncing[srv.id]">{{ syncing[srv.id] ? '同步中...' : '状态同步' }}</button>
                  <button class="btn btn-sm" @click="deployAgent(srv.id)" :disabled="srv.status==='deploying' || deployLoading[srv.id]">{{ deployLoading[srv.id] ? '探测中...' : '部署' }}</button>
                  <button class="btn btn-sm btn-danger" @click="confirmDelete(srv)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="sshError" style="margin-top:var(--space-12);color:var(--danger);font-size:13px;">{{ sshError }}</div>
    </div>

    <!-- 服务器详情面板 -->
    <div v-if="selectedServer" class="card" style="margin-top:16px">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
        <h3 style="margin:0;font-size:16px">{{ selectedServer.name }}</h3>
        <button class="btn btn-sm" @click="selectedServer=null;stopPolling()">关闭</button>
      </div>

      <!-- Agent 信息卡片 -->
      <div style="background:var(--bg-deep);border:1px solid var(--border);border-radius:var(--radius-md);padding:var(--space-16);margin-bottom:16px">
        <h4 style="margin:0 0 8px 0;font-size:14px;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em">Agent 信息</h4>
        <div style="display:grid;grid-template-columns:auto 1fr;gap:6px 12px;font-size:13px">
          <span style="color:var(--text-muted)">状态：</span>
          <span><span class="badge" :class="statusBadge(selectedServer.status)"><span class="badge-dot"></span> {{ statusLabel(selectedServer.status) }}</span></span>
          <span style="color:var(--text-muted)">Agent 版本：</span>
          <span>{{ selectedServer.agent_version || '未部署' }}</span>
          <span style="color:var(--text-muted)">部署路径：</span>
          <span>{{ selectedServer.agent_deploy_path || '未部署' }}</span>
          <span style="color:var(--text-muted)">最后部署：</span>
          <span>{{ formatTime(selectedServer.agent_deployed_at) }}</span>
          <span style="color:var(--text-muted)">最后在线：</span>
          <span>{{ formatTime(selectedServer.last_seen) }}</span>
        </div>
      </div>

      <h3 style="margin:0 0 12px 0;font-size:14px">操作日志</h3>
      <div v-if="logs.length === 0" class="empty-state"><span class="empty-text">暂无操作日志</span></div>
      <div v-else class="log-list">
        <div v-for="log in logs" :key="log.id" class="log-item">
          <span class="log-status" :class="'log-' + log.status">
            {{ log.status === 'running' ? '⏳' : log.status === 'success' ? '✅' : '❌' }}
          </span>
          <div class="log-content">
            <div class="log-step">{{ log.step }}</div>
            <div v-if="log.detail" class="log-detail">{{ log.detail }}</div>
            <div class="log-time">{{ formatTime(log.created_at) }}</div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">添加服务器</h2>
        <form @submit.prevent="addServer">
          <div class="form-row">
            <div class="form-group"><label class="form-label">名称</label><input v-model="form.name" class="form-input" placeholder="web-01" required /></div>
            <div class="form-group"><label class="form-label">主机</label><input v-model="form.host" class="form-input" placeholder="10.0.0.1" required /></div>
          </div>
          <div class="form-row">
            <div class="form-group"><label class="form-label">Agent 端口</label><input v-model.number="form.port" class="form-input" type="number" placeholder="9527" /></div>
            <div class="form-group"><label class="form-label">SSH 端口</label><input v-model.number="form.ssh_port" class="form-input" type="number" placeholder="22" /></div>
          </div>
          <div class="form-group"><label class="form-label">SSH 用户</label><input v-model="form.ssh_user" class="form-input" placeholder="root" /></div>
          <div class="form-group"><label class="form-label">认证方式</label><select v-model="form.ssh_auth_type" class="form-select"><option value="password">密码</option><option value="key">私钥</option></select></div>
          <div v-if="form.ssh_auth_type==='password'" class="form-group"><label class="form-label">SSH 密码</label><input v-model="form.ssh_password" class="form-input" type="password" /></div>
          <div v-if="form.ssh_auth_type==='key'" class="form-group"><label class="form-label">SSH 私钥</label><textarea v-model="form.ssh_key" class="form-input" rows="4" /></div>
          <div class="modal-actions"><button type="button" class="btn" @click="showAdd=false">取消</button><button type="submit" class="btn btn-primary">添加</button></div>
        </form>
      </div>
    </div>

    <!-- 部署确认弹窗 -->
    <div v-if="showDeployConfirm && deployTarget" class="overlay" @click.self="cancelDeploy">
      <div class="modal">
        <h2 class="modal-title">确认部署</h2>
        <div v-if="deployProbe[deployTarget.id]?.installed" style="margin-bottom:var(--space-16)">
          <p style="color:var(--warn);font-size:14px;margin-bottom:var(--space-12);">检测到远端已有 Agent：</p>
          <div style="background:var(--bg-deep);border:1px solid var(--border);border-radius:var(--radius-md);padding:var(--space-12);font-size:13px;">
            <div v-if="deployProbe[deployTarget.id].agent_version"><span style="color:var(--text-muted)">版本：</span>{{ deployProbe[deployTarget.id].agent_version }}</div>
            <div v-if="deployProbe[deployTarget.id].process_running"><span class="badge badge-online" style="margin-top:4px">进程运行中</span></div>
            <div v-if="deployProbe[deployTarget.id].systemd_active"><span class="badge badge-online" style="margin-top:4px">systemd 运行中</span></div>
          </div>
          <p style="color:var(--text-secondary);font-size:13px;margin-top:var(--space-12);">覆盖部署将先停止旧 Agent，再安装新版本。是否继续？</p>
        </div>
        <div v-else style="margin-bottom:var(--space-16)">
          <p style="color:var(--text-secondary);font-size:14px;">未检测到远端 Agent，将在服务器 <strong>{{ deployTarget.name }}</strong> ({{ deployTarget.host }}) 上全新部署。是否继续？</p>
        </div>
        <div class="modal-actions">
          <button class="btn" @click="cancelDeploy">取消</button>
          <button class="btn btn-primary" @click="confirmDeploy">确认部署</button>
        </div>
      </div>
    </div>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget=null">
      <div class="modal"><h2 class="modal-title">删除服务器</h2><p style="color:var(--text-secondary);margin-bottom:var(--space-16);">确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget=null">取消</button><button class="btn btn-danger" @click="deleteServer">确认删除</button></div></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { api } from '../api/index.js'

const servers = ref([])
const selectedServer = ref(null)
const logs = ref([])
let pollTimer = null
const showAdd = ref(false)
const deleteTarget = ref(null)
const sshStatus = reactive({})
const syncing = reactive({})
const deployLoading = reactive({})
const deployProbe = reactive({})
const showDeployConfirm = ref(false)
const deployTarget = ref(null)
const sshError = ref('')
const form = ref({ name:'',host:'',port:9527,ssh_port:22,ssh_user:'root',ssh_auth_type:'password',ssh_password:'',ssh_key:'' })

onMounted(fetchServers)
function selectServer(srv) {
  if (selectedServer.value?.id === srv.id) {
    selectedServer.value = null
    stopPolling()
    return
  }
  selectedServer.value = srv
  logs.value = []
  fetchLogs()
  startPolling()
}

async function fetchLogs() {
  if (!selectedServer.value) return
  try {
    const r = await api.get(`/operations?resource_type=server&resource_id=${selectedServer.value.id}`)
    const data = await r.json()
    logs.value = data.operations || []
    // Check if any log is still running
    const hasRunning = logs.value.some(l => l.status === 'running')
    if (!hasRunning) stopPolling()
  } catch (e) { console.error(e) }
}

function startPolling() { stopPolling(); pollTimer = setInterval(fetchLogs, 2000) }
function stopPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null } }

async function fetchServers() { try { const r = await api.get('/servers'); servers.value = await r.json() } catch(e){console.error(e)} }
async function addServer() { try { await api.post('/servers', form.value); showAdd.value=false; resetForm(); fetchServers() } catch(e){console.error(e)} }

async function testSSH(id) {
  sshError.value = ''
  sshStatus[id] = 'testing'
  try {
    await api.post(`/servers/${id}/ssh-test`)
    sshStatus[id] = 'ok'
  } catch (e) {
    sshStatus[id] = 'fail'
    sshError.value = 'SSH 测试失败，请检查凭据和网络'
  }
}

async function syncStatus(id) {
  syncing[id] = true
  try {
    const r = await api.post(`/servers/${id}/deploy/probe`)
    if (r.ok) {
      await fetchServers()
      // 刷新详情面板
      if (selectedServer.value?.id === id) {
        const updated = servers.value.find(s => s.id === id)
        if (updated) selectedServer.value = updated
      }
    }
  } catch (e) { console.error(e) }
  syncing[id] = false
}

async function deployAgent(id) {
  deployLoading[id] = true
  try {
    const r = await api.post(`/servers/${id}/deploy/probe`)
    if (!r.ok) {
      const err = await r.json()
      alert('探测失败: ' + (err.error || '未知错误'))
      deployLoading[id] = false
      return
    }
    const probe = await r.json()
    deployProbe[id] = probe
    deployTarget.value = servers.value.find(s => s.id === id)
    showDeployConfirm.value = true
  } catch (e) {
    console.error(e)
    alert('探测失败: 网络异常')
  }
  deployLoading[id] = false
}

async function confirmDeploy() {
  if (!deployTarget.value) return
  const id = deployTarget.value.id
  deployLoading[id] = true
  showDeployConfirm.value = false
  try {
    const force = deployProbe[id]?.installed ? '?force=true' : ''
    await api.post(`/servers/${id}/deploy${force}`)
    fetchServers()
  } catch (e) {
    console.error(e)
    alert('部署失败')
  }
  deployLoading[id] = false
  deployProbe[id] = null
  deployTarget.value = null
}

function cancelDeploy() {
  showDeployConfirm.value = false
  deployTarget.value = null
}
function confirmDelete(srv) { deleteTarget.value = srv }
async function deleteServer() { try { await api.delete(`/servers/${deleteTarget.value.id}`); deleteTarget.value=null; fetchServers() } catch(e){console.error(e)} }
function resetForm() { form.value = { name:'',host:'',port:9527,ssh_port:22,ssh_user:'root',ssh_auth_type:'password',ssh_password:'',ssh_key:'' } }
function statusBadge(s) { return s==='online'?'badge-online':s==='deploying'?'badge-deploying':'badge-offline' }
function statusLabel(s) { const m={online:'在线',offline:'离线',deploying:'部署中'}; return m[s]||s }
function formatTime(d) { if(!d)return'-'; return new Date(d).toLocaleString('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}) }
</script>
