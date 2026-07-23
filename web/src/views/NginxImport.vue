<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">NGINX 导入</h1>
    </div>

    <!-- 触发导入 -->
    <div class="card" style="margin-bottom: var(--space-24);">
      <div class="card-header">
        <h2 class="card-title">从服务器导入配置</h2>
      </div>
      <div style="display:flex;align-items:flex-end;gap:var(--space-16);flex-wrap:wrap;">
        <div class="form-group" style="margin-bottom:0;min-width:200px;">
          <label class="form-label">服务器 ID</label>
          <input v-model.number="serverId" class="form-input" type="number" placeholder="1" />
        </div>
        <button class="btn btn-primary" @click="doImport" :disabled="importing">
          {{ importing ? '导入中...' : '开始导入' }}
        </button>
      </div>
      <div v-if="importError" style="margin-top:var(--space-12);color:var(--danger);font-size:13px;">{{ importError }}</div>
    </div>

    <!-- 导入结果 -->
    <div v-if="importResult" class="card" style="margin-bottom: var(--space-24);">
      <div class="card-header">
        <h2 class="card-title">导入结果</h2>
        <span style="font-size:13px;color:var(--text-secondary);">发现 {{ importResult.sites?.length || 0 }} 个站点</span>
      </div>
      <div v-if="!importResult.sites || importResult.sites.length === 0" class="empty-state">
        <span class="empty-icon">⊞</span>
        <span class="empty-text">未发现新站点，或所有站点已被管理</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>域名</th>
              <th>端口</th>
              <th>SSL</th>
              <th>根目录</th>
              <th>代理目标</th>
              <th>状态</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(site, idx) in importResult.sites" :key="idx">
              <td style="font-weight:600;">{{ site.domain }}</td>
              <td>{{ site.port || 80 }}</td>
              <td>
                <span class="badge" :class="site.ssl_enabled ? 'badge-online' : 'badge-offline'">
                  {{ site.ssl_enabled ? 'HTTPS' : 'HTTP' }}
                </span>
              </td>
              <td>{{ site.root_path || '-' }}</td>
              <td style="font-size:12px;">{{ site.proxy_pass || '-' }}</td>
              <td>
                <span class="badge" :class="site.taken_over ? 'badge-online' : 'badge-offline'">
                  {{ site.taken_over ? '已接管' : '未受管' }}
                </span>
              </td>
              <td>
                <button
                  v-if="!site.taken_over"
                  class="btn btn-sm"
                  @click="takeover(site)"
                  :disabled="site.taking"
                >{{ site.taking ? '接管中...' : '接管' }}</button>
                <span v-else style="font-size:12px;color:var(--text-muted);">已由平台管理</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 已有非受管站点 -->
    <div class="card">
      <div class="card-header">
        <h2 class="card-title">已有非受管站点</h2>
      </div>
      <div v-if="unmanagedSites.length === 0" class="empty-state">
        <span class="empty-icon">✓</span>
        <span class="empty-text">所有站点均在管理中</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>域名</th>
              <th>服务器</th>
              <th>SSL</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="site in unmanagedSites" :key="site.id">
              <td style="font-weight:600;">{{ site.domain }}</td>
              <td>#{{ site.server_id }}</td>
              <td>
                <span class="badge" :class="site.ssl_enabled ? 'badge-online' : 'badge-offline'">
                  {{ site.ssl_enabled ? 'HTTPS' : 'HTTP' }}
                </span>
              </td>
              <td>
                <button class="btn btn-sm" @click="takeoverExisting(site)">接管</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const serverId = ref(1)
const importing = ref(false)
const importError = ref('')
const importResult = ref(null)
const unmanagedSites = ref([])

onMounted(fetchUnmanaged)

async function fetchUnmanaged() {
  try {
    const r = await api.get('/sites')
    const all = await r.json()
    unmanagedSites.value = (Array.isArray(all) ? all : []).filter(s => !s.managed)
  } catch (e) { console.error(e) }
}

async function doImport() {
  importing.value = true
  importError.value = ''
  importResult.value = null
  try {
    const r = await api.post('/nginx/import', { server_id: serverId.value })
    importResult.value = await r.json()
    fetchUnmanaged()
  } catch (e) {
    importError.value = '导入失败: ' + (e.message || '未知错误')
  }
  importing.value = false
}

async function takeover(site) {
  site.taking = true
  try {
    // 接管：先创建站点，然后设为 managed
    const r = await api.post('/sites', {
      server_id: serverId.value,
      domain: site.domain,
      port: site.port || 80,
      root_path: site.root_path || '',
      ssl_enabled: site.ssl_enabled || false,
    })
    const created = await r.json()
    // 标记为受管
    await api.put(`/sites/${created.id}`, { managed: true })
    site.taken_over = true
    fetchUnmanaged()
  } catch (e) {
    console.error('接管失败', e)
  }
  site.taking = false
}

async function takeoverExisting(site) {
  try {
    await api.put(`/sites/${site.id}`, { managed: true })
    fetchUnmanaged()
  } catch (e) { console.error(e) }
}
</script>
