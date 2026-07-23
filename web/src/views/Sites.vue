<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">站点</h1>
      <button class="btn btn-primary" @click="showAdd=true">+ 添加站点</button>
    </div>

    <div class="card" style="margin-bottom:16px">
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:12px">
        <label class="form-label" style="margin-bottom:0;white-space:nowrap">服务器：</label>
        <select v-model="filterServer" class="form-select" style="width:auto;min-width:160px" @change="onFilterChange">
          <option :value="0">全部</option>
          <option v-for="s in servers" :key="s.id" :value="s.id">{{ s.name }} ({{ s.host }})</option>
        </select>
      </div>
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab==='managed' }]" @click="activeTab='managed'">受管站点</button>
        <button :class="['tab-btn', { 'tab-active': activeTab==='import' }]" @click="activeTab='import'">导入站点</button>
      </div>
    </div>

    <!-- 受管站点 tab -->
    <div v-if="activeTab==='managed'" class="card">
      <div v-if="filteredSites.length===0" class="empty-state"><span class="empty-icon">⊞</span><span class="empty-text">暂无受管站点</span></div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>域名</th><th>SSL</th><th>证书到期</th><th></th></tr></thead>
          <tbody>
            <tr v-for="site in filteredSites" :key="site.id">
              <td style="font-weight:600">{{ site.domain }}</td>
              <td><span class="badge" :class="site.ssl_enabled?'badge-online':'badge-offline'">{{ site.ssl_enabled?'HTTPS':'HTTP' }}</span></td>
              <td>{{ site.cert?formatDate(site.cert.valid_to):'-' }}</td>
              <td><div class="btn-group" style="justify-content:flex-end;"><button class="btn btn-sm" @click="viewSite(site)">管理</button><button class="btn btn-sm btn-danger" @click="confirmDelete(site)">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 导入站点 tab -->
    <div v-if="activeTab==='import'" class="card">
      <div style="margin-bottom:12px">
        <button class="btn btn-primary btn-sm" @click="doImport" :disabled="importing || filterServer===0">
          {{ importing ? '扫描中...' : '扫描 NGINX 配置' }}
        </button>
        <span v-if="filterServer===0" style="font-size:12px;color:var(--text-muted);margin-left:8px">请先选择服务器</span>
      </div>
      <div v-if="importError" style="color:var(--danger);font-size:13px;margin-bottom:12px">{{ importError }}</div>
      <div v-if="importSites.length===0 && !importing" class="empty-state"><span class="empty-text">点击扫描按钮从服务器导入 NGINX 站点</span></div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>域名</th><th>端口</th><th>SSL</th><th>根目录</th><th>代理</th><th></th></tr></thead>
          <tbody>
            <tr v-for="(s, idx) in importSites" :key="idx">
              <td style="font-weight:600">{{ s.domain }}</td>
              <td>{{ s.port||80 }}</td>
              <td><span class="badge" :class="s.ssl_enabled?'badge-online':'badge-offline'">{{ s.ssl_enabled?'HTTPS':'HTTP' }}</span></td>
              <td style="font-size:12px">{{ s.root_path||'-' }}</td>
              <td style="font-size:12px">{{ s.proxy_pass||'-' }}</td>
              <td><button class="btn btn-sm" @click="takeover(s)" :disabled="s.taking">{{ s.taking?'接管中...':'接管' }}</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 添加站点弹窗 -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd=false">
      <div class="modal"><h2 class="modal-title">添加站点</h2>
        <form @submit.prevent="addSite">
          <div class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="form.domain" class="form-input" placeholder="example.com" required /></div><div class="form-group"><label class="form-label">服务器</label><select v-model.number="form.server_id" class="form-select"><option v-for="s in servers" :key="s.id" :value="s.id">{{ s.name }}</option></select></div></div>
          <div class="form-row"><div class="form-group"><label class="form-label">端口</label><input v-model.number="form.port" class="form-input" type="number" /></div><div class="form-group"><label class="form-label">根目录</label><input v-model="form.root_path" class="form-input" /></div></div>
          <div class="form-group"><label class="form-label" style="display:flex;align-items:center;text-transform:none;letter-spacing:0;font-size:14px;color:var(--text-primary);"><input type="checkbox" v-model="form.ssl_enabled" style="margin-right:8px;" />启用 SSL</label></div>
          <div class="modal-actions"><button type="button" class="btn" @click="showAdd=false">取消</button><button type="submit" class="btn btn-primary">添加</button></div>
        </form>
      </div>
    </div>

    <!-- 删除确认 -->
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget=null">
      <div class="modal"><h2 class="modal-title">删除站点</h2><p style="color:var(--text-secondary);margin-bottom:var(--space-16);">确定删除 <strong>{{ deleteTarget.domain }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget=null">取消</button><button class="btn btn-danger" @click="deleteSite">确认删除</button></div></div>
    </div>

    <!-- 站点管理弹窗 -->
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
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/index.js'

const sites = ref([])
const servers = ref([])
const filterServer = ref(0)
const activeTab = ref('managed')
const importSites = ref([])
const importing = ref(false)
const importError = ref('')
const showAdd = ref(false)
const deleteTarget = ref(null)
const selectedSite = ref(null)
const form = ref({ domain:'',server_id:1,port:80,root_path:'',ssl_enabled:false })

const filteredSites = computed(() => filterServer.value===0 ? sites.value : sites.value.filter(s=>s.server_id===filterServer.value))

onMounted(async () => {
  await fetchSites()
  await fetchServers()
})

async function fetchServers() { try { const r = await api.get('/servers'); servers.value = await r.json() } catch(e){console.error(e)} }
async function fetchSites() { try { const r = await api.get('/sites'); sites.value = await r.json() } catch(e){console.error(e)} }

async function addSite() {
  try { await api.post('/sites', form.value); showAdd.value=false; form.value={domain:'',server_id:filterServer.value||1,port:80,root_path:'',ssl_enabled:false}; fetchSites() } catch(e){console.error(e)}
}

function confirmDelete(site) { deleteTarget.value=site }
async function deleteSite() { try { await api.delete(`/sites/${deleteTarget.value.id}`); deleteTarget.value=null; fetchSites() } catch(e){console.error(e)} }
function viewSite(site) { selectedSite.value={...site} }

async function generateNginx() {
  try { await api.post(`/sites/${selectedSite.value.id}/nginx/generate`); alert('NGINX 配置已生成并重载') } catch(e){ alert('生成失败: '+(await e)) }
}
async function reloadNginx() {
  try { await api.post(`/sites/${selectedSite.value.id}/nginx/reload`); alert('NGINX 已重载') } catch(e){ alert('重载失败') }
}
async function issueCert() { await api.post(`/sites/${selectedSite.value.id}/issue`) }
async function renewCert() { await api.post(`/sites/${selectedSite.value.id}/renew`) }
async function revokeCert() { await api.post(`/sites/${selectedSite.value.id}/revoke`) }

async function doImport() {
  if (!filterServer.value) return
  importing.value = true; importError.value = ''; importSites.value = []
  try {
    const r = await api.post('/nginx/import', { server_id: filterServer.value })
    const data = await r.json()
    importSites.value = data.sites || []
  } catch(e) { importError.value = '扫描失败: ' + (e.message||'') }
  importing.value = false
}

async function takeover(site) {
  site.taking = true
  try {
    await api.post('/sites', {
      server_id: filterServer.value,
      domain: site.domain,
      port: site.port||80,
      root_path: site.root_path||'',
      ssl_enabled: site.ssl_enabled||false,
      managed: true
    })
    fetchSites()
    // Remove from import list
    importSites.value = importSites.value.filter(s => s.domain !== site.domain)
  } catch(e) { console.error(e) }
  site.taking = false
}

function onFilterChange() {
  importSites.value = []
  importError.value = ''
}

function formatDate(d) { if(!d)return'-'; return new Date(d).toLocaleDateString('zh-CN',{month:'short',day:'numeric',year:'numeric'}) }
function certStatusLabel(s) { const m={issued:'已签发',renewing:'续期中',expired:'已过期',revoked:'已吊销'}; return m[s]||s }
</script>

<style scoped>
.table-tabs { display: flex; gap: 4px; }
.tab-btn { padding: 6px 16px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--bg-deep); color: var(--text-secondary); font-size: 13px; cursor: pointer; transition: all 0.15s; font-family: inherit; }
.tab-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.tab-active { background: var(--accent); border-color: var(--accent); color: var(--bg-deep); font-weight: 600; }
</style>
