<template>
  <div>
    <div class="page-header"><h1 class="page-title">站点</h1><button class="btn btn-primary" @click="showAdd=true">+ 添加站点</button></div>
    <div class="card">
      <div v-if="sites.length===0" class="empty-state"><span class="empty-icon">⊞</span><span class="empty-text">暂无站点</span></div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>域名</th><th>服务器</th><th>SSL</th><th>受管</th><th>证书到期</th><th></th></tr></thead>
          <tbody>
            <tr v-for="site in sites" :key="site.id">
              <td style="font-weight:600">{{ site.domain }}</td><td>#{{ site.server_id }}</td>
              <td><span class="badge" :class="site.ssl_enabled?'badge-online':'badge-offline'">{{ site.ssl_enabled?'HTTPS':'HTTP' }}</span></td>
              <td><span class="badge" :class="site.managed?'badge-online':'badge-offline'">{{ site.managed?'是':'否' }}</span></td>
              <td>{{ site.cert?formatDate(site.cert.valid_to):'-' }}</td>
              <td><div class="btn-group" style="justify-content:flex-end;"><button class="btn btn-sm" @click="viewSite(site)">管理</button><button class="btn btn-sm btn-danger" @click="confirmDelete(site)">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="showAdd" class="overlay" @click.self="showAdd=false">
      <div class="modal"><h2 class="modal-title">添加站点</h2>
        <form @submit.prevent="addSite">
          <div class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="form.domain" class="form-input" placeholder="example.com" required /></div><div class="form-group"><label class="form-label">服务器 ID</label><input v-model.number="form.server_id" class="form-input" type="number" required /></div></div>
          <div class="form-row"><div class="form-group"><label class="form-label">端口</label><input v-model.number="form.port" class="form-input" type="number" /></div><div class="form-group"><label class="form-label">根目录</label><input v-model="form.root_path" class="form-input" /></div></div>
          <div class="form-group"><label class="form-label" style="display:flex;align-items:center;text-transform:none;letter-spacing:0;font-size:14px;color:var(--text-primary);"><input type="checkbox" v-model="form.ssl_enabled" style="margin-right:8px;" />启用 SSL</label></div>
          <div class="modal-actions"><button type="button" class="btn" @click="showAdd=false">取消</button><button type="submit" class="btn btn-primary">添加</button></div>
        </form>
      </div>
    </div>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget=null">
      <div class="modal"><h2 class="modal-title">删除站点</h2><p style="color:var(--text-secondary);margin-bottom:var(--space-16);">确定删除 <strong>{{ deleteTarget.domain }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget=null">取消</button><button class="btn btn-danger" @click="deleteSite">确认删除</button></div></div>
    </div>

    <div v-if="selectedSite" class="overlay" @click.self="selectedSite=null">
      <div class="modal" style="width:600px;"><h2 class="modal-title">{{ selectedSite.domain }}</h2>
        <div style="margin-bottom:var(--space-16);"><label class="form-label" style="margin-bottom:var(--space-8);">NGINX</label><div class="btn-group"><button class="btn btn-sm" @click="generateNginx">生成配置</button><button class="btn btn-sm" @click="reloadNginx">重载</button></div></div>
        <div style="margin-bottom:var(--space-16);"><label class="form-label" style="margin-bottom:var(--space-8);">证书</label><div class="btn-group"><button class="btn btn-sm" @click="issueCert">签发</button><button class="btn btn-sm" @click="renewCert">续期</button><button class="btn btn-sm btn-danger" @click="revokeCert">吊销</button></div></div>
        <div v-if="selectedSite.cert" style="font-size:12px;color:var(--text-muted);margin-top:var(--space-16);"><div>状态: <strong>{{ certStatusLabel(selectedSite.cert.status) }}</strong></div><div>到期: {{ formatDate(selectedSite.cert.valid_to) }}</div></div>
        <div class="modal-actions"><button class="btn" @click="selectedSite=null">关闭</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const sites = ref([])
const showAdd = ref(false)
const deleteTarget = ref(null)
const selectedSite = ref(null)
const form = ref({ domain:'',server_id:1,port:80,root_path:'',ssl_enabled:false })

onMounted(fetchSites)
async function fetchSites() { try { const r = await api.get('/sites'); sites.value = await r.json() } catch(e){console.error(e)} }
async function addSite() { try { await api.post('/sites', form.value); showAdd.value=false; form.value={domain:'',server_id:1,port:80,root_path:'',ssl_enabled:false}; fetchSites() } catch(e){console.error(e)} }
function confirmDelete(site) { deleteTarget.value=site }
async function deleteSite() { try { await api.delete(`/sites/${deleteTarget.value.id}`); deleteTarget.value=null; fetchSites() } catch(e){console.error(e)} }
function viewSite(site) { selectedSite.value={...site} }
async function generateNginx() { await api.post(`/sites/${selectedSite.value.id}/nginx/generate`) }
async function reloadNginx() { await api.post(`/sites/${selectedSite.value.id}/nginx/reload`) }
async function issueCert() { await api.post(`/sites/${selectedSite.value.id}/issue`) }
async function renewCert() { await api.post(`/sites/${selectedSite.value.id}/renew`) }
async function revokeCert() { await api.post(`/sites/${selectedSite.value.id}/revoke`) }
function formatDate(d) { if(!d)return'-'; return new Date(d).toLocaleDateString('zh-CN',{month:'short',day:'numeric',year:'numeric'}) }
function certStatusLabel(s) { const m={issued:'已签发',renewing:'续期中',expired:'已过期',revoked:'已吊销'}; return m[s]||s }
</script>
