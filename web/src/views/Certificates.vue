<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">证书</h1>
      <button class="btn btn-primary" @click="showAdd = true">+ 添加证书</button>
    </div>

    <div class="card section-gap">
      <div class="filter-bar">
        <div class="filter-control"><label class="form-label">命名空间：</label>
        <select v-model="filterNs" class="form-select">
          <option value="">全部</option>
          <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
        </select></div>
      </div>
    </div>

    <div class="card">
      <div v-if="filteredCerts.length === 0" class="empty-state">
        <span class="empty-icon">🔒</span>
        <span class="empty-text">暂无证书</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>名称</th><th>命名空间</th><th>域名</th><th>签发者</th><th>到期</th><th>状态</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="cert in filteredCerts" :key="cert.namespace + '/' + cert.name">
              <td class="cell-primary">{{ cert.name }}</td>
              <td>{{ cert.namespace }}</td>
              <td>{{ cert.domains || '-' }}</td>
              <td>{{ cert.issuer }}</td>
              <td>{{ formatDate(cert.not_after) }}</td>
              <td>
                <span class="badge" :class="certStatusClass(cert.status)">
                  {{ certStatusLabel(cert.status) }}
                </span>
              </td>
              <td>
                <div class="btn-group action-cell">
                  <button class="btn btn-sm btn-danger" @click="confirmDelete(cert)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add Cert modal -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">添加证书</h2>
        <form @submit.prevent="createCert">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">名称</label>
              <input v-model="form.name" class="form-input" placeholder="my-cert" required />
            </div>
            <div class="form-group">
              <label class="form-label">命名空间</label>
              <input v-model="form.namespace" class="form-input" placeholder="default" required />
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">域名（多个用逗号分隔）</label>
            <input v-model="form.domains" class="form-input" placeholder="example.com,*.example.com" required />
          </div>
          <div class="form-group">
            <label class="form-label">签发者</label>
            <input v-model="form.issuer" class="form-input" placeholder="letsencrypt-prod" required />
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
        <h2 class="modal-title">删除证书</h2>
        <p class="modal-copy">
          确定删除 <strong>{{ deleteTarget.name }}</strong>（{{ deleteTarget.namespace }}）？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="deleteTarget = null">取消</button>
          <button class="btn btn-danger" @click="removeCert">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/index.js'

const certs = ref([])
const filterNs = ref('')
const showAdd = ref(false)
const deleteTarget = ref(null)
const form = ref({ name: '', namespace: 'default', domains: '', issuer: 'letsencrypt-prod' })

const namespaces = computed(() => [...new Set(certs.value.map(c => c.namespace))].sort())
const filteredCerts = computed(() =>
  filterNs.value ? certs.value.filter(c => c.namespace === filterNs.value) : certs.value
)

onMounted(fetchCerts)

async function fetchCerts() {
  try {
    certs.value = await api.get('/certs') || []
  } catch (e) { console.error(e) }
}

async function createCert() {
  try {
    const body = { ...form.value }
    body.domains = body.domains.split(',').map(d => d.trim()).filter(Boolean)
    await api.post('/certs', body)
    showAdd.value = false
    form.value = { name: '', namespace: 'default', domains: '', issuer: 'letsencrypt-prod' }
    fetchCerts()
  } catch (e) { console.error(e) }
}

function confirmDelete(cert) { deleteTarget.value = cert }

async function removeCert() {
  try {
    await api.delete(`/certs/${deleteTarget.value.namespace}/${deleteTarget.value.name}`)
    deleteTarget.value = null
    fetchCerts()
  } catch (e) { console.error(e) }
}

function formatDate(d) {
  if (!d) return '-'
  return new Date(d).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric', year: 'numeric' })
}

function certStatusLabel(s) {
  const m = { ready: '就绪', pending: '签发中', failed: '失败', expired: '已过期' }
  return m[s] || s || '-'
}

function certStatusClass(s) {
  const m = { ready: 'badge-online', pending: 'badge-deploying', failed: 'badge-danger', expired: 'badge-danger' }
  return m[s] || 'badge-offline'
}
</script>
