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
            <tr v-for="srv in servers" :key="srv.id">
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
                  <button class="btn btn-sm" @click="deployAgent(srv.id)" :disabled="srv.status==='deploying'">部署</button>
                  <button class="btn btn-sm btn-danger" @click="confirmDelete(srv)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="sshError" style="margin-top:var(--space-12);color:var(--danger);font-size:13px;">{{ sshError }}</div>
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

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget=null">
      <div class="modal"><h2 class="modal-title">删除服务器</h2><p style="color:var(--text-secondary);margin-bottom:var(--space-16);">确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget=null">取消</button><button class="btn btn-danger" @click="deleteServer">确认删除</button></div></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { api } from '../api/index.js'

const servers = ref([])
const showAdd = ref(false)
const deleteTarget = ref(null)
const sshStatus = reactive({})
const sshError = ref('')
const form = ref({ name:'',host:'',port:9527,ssh_port:22,ssh_user:'root',ssh_auth_type:'password',ssh_password:'',ssh_key:'' })

onMounted(fetchServers)
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

async function deployAgent(id) {
  try { await api.post(`/servers/${id}/deploy`); fetchServers() } catch(e){console.error(e)}
}
function confirmDelete(srv) { deleteTarget.value = srv }
async function deleteServer() { try { await api.delete(`/servers/${deleteTarget.value.id}`); deleteTarget.value=null; fetchServers() } catch(e){console.error(e)} }
function resetForm() { form.value = { name:'',host:'',port:9527,ssh_port:22,ssh_user:'root',ssh_auth_type:'password',ssh_password:'',ssh_key:'' } }
function statusBadge(s) { return s==='online'?'badge-online':s==='deploying'?'badge-deploying':'badge-offline' }
function statusLabel(s) { const m={online:'在线',offline:'离线',deploying:'部署中'}; return m[s]||s }
function formatTime(d) { if(!d)return'-'; return new Date(d).toLocaleString('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}) }
</script>
