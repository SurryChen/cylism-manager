<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">路由</h1>
      <button class="btn btn-primary" @click="showAdd = true">+ 添加路由</button>
    </div>

    <!-- Server filter -->
    <div class="card" style="margin-bottom:16px">
      <div style="display:flex;align-items:center;gap:12px;">
        <label class="form-label" style="margin-bottom:0;white-space:nowrap">命名空间：</label>
        <select v-model="filterNs" class="form-select" style="width:auto;min-width:160px">
          <option value="">全部</option>
          <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
        </select>
      </div>
    </div>

    <div class="card">
      <div v-if="filteredRoutes.length === 0" class="empty-state">
        <span class="empty-icon">⊞</span>
        <span class="empty-text">暂无 IngressRoute</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>名称</th><th>命名空间</th><th>域名</th><th>服务</th><th>端口</th><th>TLS</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="route in filteredRoutes" :key="route.namespace + '/' + route.name">
              <td style="font-weight:600">{{ route.name }}</td>
              <td>{{ route.namespace }}</td>
              <td>{{ route.host || '-' }}</td>
              <td>{{ route.service_name }}</td>
              <td>{{ route.service_port }}</td>
              <td>
                <span class="badge" :class="route.tls_enabled ? 'badge-online' : 'badge-offline'">
                  {{ route.tls_enabled ? 'HTTPS' : 'HTTP' }}
                </span>
              </td>
              <td>
                <div class="btn-group" style="justify-content:flex-end;">
                  <button class="btn btn-sm btn-danger" @click="confirmDelete(route)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add Route modal -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">添加 IngressRoute</h2>
        <form @submit.prevent="createRoute">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">名称</label>
              <input v-model="form.name" class="form-input" placeholder="my-route" required />
            </div>
            <div class="form-group">
              <label class="form-label">命名空间</label>
              <input v-model="form.namespace" class="form-input" placeholder="default" required />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">域名</label>
              <input v-model="form.host" class="form-input" placeholder="example.com" required />
            </div>
            <div class="form-group">
              <label class="form-label">服务名称</label>
              <input v-model="form.service_name" class="form-input" placeholder="my-service" required />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">服务端口</label>
              <input v-model.number="form.service_port" class="form-input" type="number" placeholder="80" required />
            </div>
            <div class="form-group">
              <label class="form-label" style="display:flex;align-items:center;text-transform:none;letter-spacing:0;font-size:14px;color:var(--text-primary);">
                <input type="checkbox" v-model="form.tls_enabled" style="margin-right:8px;" />
                TLS
              </label>
            </div>
          </div>
          <div class="form-group" v-if="form.tls_enabled">
            <label class="form-label">TLS 密钥名</label>
            <input v-model="form.tls_secret" class="form-input" placeholder="my-tls-secret" />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn" @click="showAdd = false">取消</button>
            <button type="submit" class="btn btn-primary">确认添加</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Delete confirm modal -->
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal">
        <h2 class="modal-title">删除路由</h2>
        <p style="color:var(--text-secondary);margin-bottom:16px;">
          确定删除 <strong>{{ deleteTarget.name }}</strong>（{{ deleteTarget.namespace }}）？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="deleteTarget = null">取消</button>
          <button class="btn btn-danger" @click="removeRoute">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/index.js'

const routes = ref([])
const filterNs = ref('')
const showAdd = ref(false)
const deleteTarget = ref(null)
const form = ref({ name: '', namespace: 'default', host: '', service_name: '', service_port: 80, tls_enabled: false, tls_secret: '' })

const namespaces = computed(() => [...new Set(routes.value.map(r => r.namespace))].sort())
const filteredRoutes = computed(() =>
  filterNs.value ? routes.value.filter(r => r.namespace === filterNs.value) : routes.value
)

onMounted(fetchRoutes)

async function fetchRoutes() {
  try {
    const r = await api.get('/routes')
    routes.value = await r.json()
  } catch (e) { console.error(e) }
}

async function createRoute() {
  try {
    await api.post('/routes', form.value)
    showAdd.value = false
    form.value = { name: '', namespace: 'default', host: '', service_name: '', service_port: 80, tls_enabled: false, tls_secret: '' }
    fetchRoutes()
  } catch (e) { console.error(e) }
}

function confirmDelete(route) { deleteTarget.value = route }

async function removeRoute() {
  try {
    await api.delete(`/routes/${deleteTarget.value.namespace}/${deleteTarget.value.name}`)
    deleteTarget.value = null
    fetchRoutes()
  } catch (e) { console.error(e) }
}
</script>
