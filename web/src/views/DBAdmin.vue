<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">数据管理</h1>
      <button class="btn btn-primary" @click="showAdd = true" :disabled="!currentTable">+ 新增记录</button>
    </div>

    <div class="card" style="margin-bottom:16px">
      <div class="table-tabs">
        <button
          v-for="t in tables"
          :key="t"
          :class="['tab-btn', { 'tab-active': currentTable === t }]"
          @click="selectTable(t)"
        >{{ tableLabel(t) }}</button>
      </div>
    </div>

    <div class="card" v-if="currentTable">
      <div v-if="loading" class="empty-state"><span class="empty-text">加载中...</span></div>
      <div v-else-if="rows.length === 0" class="empty-state"><span class="empty-text">暂无数据</span></div>
      <div v-else>
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th v-for="col in columns" :key="col" style="cursor:pointer" @click="setSort(col)">
                  {{ col }} {{ sort === col ? (order === 'asc' ? '↑' : '↓') : '' }}
                </th>
                <th style="width:120px">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in rows" :key="row.id || Math.random()">
                <td v-for="col in columns" :key="col">{{ formatCell(row[col]) }}</td>
                <td>
                  <div class="btn-group">
                    <button class="btn btn-sm" @click="editRow(row)">编辑</button>
                    <button class="btn btn-sm btn-danger" @click="confirmDel(row)">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div style="display:flex;justify-content:center;align-items:center;gap:12px;margin-top:16px;">
          <button class="btn btn-sm" :disabled="page <= 1" @click="page--; fetchData()">上一页</button>
          <span style="font-size:13px;color:var(--text-secondary)">{{ page }} / {{ totalPages }}</span>
          <button class="btn btn-sm" :disabled="page >= totalPages" @click="page++; fetchData()">下一页</button>
        </div>
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <div v-if="showAdd || editTarget" class="overlay" @click.self="closeForm">
      <div class="modal">
        <h2 class="modal-title">{{ editTarget ? '编辑记录' : '新增记录' }}</h2>
        <form @submit.prevent="submitForm">
          <div v-for="col in editableColumns" :key="col" class="form-group">
            <label class="form-label">{{ col }}</label>
            <input v-model="formData[col]" class="form-input" :placeholder="col" />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn" @click="closeForm">取消</button>
            <button type="submit" class="btn btn-primary">{{ editTarget ? '保存' : '创建' }}</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 删除确认弹窗 -->
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget=null">
      <div class="modal">
        <h2 class="modal-title">确认删除</h2>
        <p style="color:var(--text-secondary);margin-bottom:var(--space-16);">
          确定删除 <strong>{{ currentTable }}</strong> 表中 ID 为 <strong>{{ deleteTarget.id }}</strong> 的记录吗？此操作不可撤销。
        </p>
        <div class="modal-actions">
          <button class="btn" @click="deleteTarget=null">取消</button>
          <button class="btn btn-danger" @click="doDelete">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, reactive } from 'vue'
import { api } from '../api/index.js'

const tables = ref([])
const currentTable = ref('')
const columns = ref([])
const rows = ref([])
const loading = ref(false)
const page = ref(1)
const size = 20
const total = ref(0)
const sort = ref('id')
const order = ref('desc')
const showAdd = ref(false)
const editTarget = ref(null)
const deleteTarget = ref(null)
const formData = reactive({})

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / size)))
const editableColumns = computed(() => columns.value.filter(c => !['id','created_at','updated_at','deleted_at'].includes(c)))

const tableLabels = { servers:'服务器', sites:'站点', certs:'证书', audit_logs:'审计日志', operation_logs:'操作日志', users:'用户' }
function tableLabel(t) { return tableLabels[t] || t }

onMounted(async () => {
  try {
    const r = await api.get('/admin/tables')
    const data = await r.json()
    tables.value = data.tables || []
  } catch (e) { console.error(e) }
})

async function selectTable(name) {
  currentTable.value = name
  page.value = 1
  sort.value = 'id'
  order.value = 'desc'
  await fetchData()
}

async function fetchData() {
  if (!currentTable.value) return
  loading.value = true
  try {
    const r = await api.get(`/admin/tables/${currentTable.value}?page=${page.value}&size=${size}&sort=${sort.value}&order=${order.value}`)
    const data = await r.json()
    rows.value = data.rows || []
    columns.value = data.columns || []
    total.value = data.total || 0
  } catch (e) { console.error(e) }
  loading.value = false
}

function setSort(col) {
  if (sort.value === col) {
    order.value = order.value === 'asc' ? 'desc' : 'asc'
  } else {
    sort.value = col
    order.value = 'asc'
  }
  fetchData()
}

function formatCell(val) {
  if (val === null || val === undefined) return '-'
  if (typeof val === 'boolean') return val ? '是' : '否'
  if (val === '') return '-'
  return String(val).substring(0, 200)
}

function editRow(row) {
  editTarget.value = row
  for (const col of columns.value) {
    formData[col] = row[col] !== null && row[col] !== undefined ? row[col] : ''
  }
}

function closeForm() {
  showAdd.value = false
  editTarget.value = null
  Object.keys(formData).forEach(k => delete formData[k])
}

async function submitForm() {
  try {
    if (editTarget.value) {
      await api.put(`/admin/tables/${currentTable.value}/${editTarget.value.id}`, { ...formData })
    } else {
      await api.post(`/admin/tables/${currentTable.value}`, { ...formData })
    }
    closeForm()
    fetchData()
  } catch (e) { console.error(e) }
}

function confirmDel(row) { deleteTarget.value = row }

async function doDelete() {
  try {
    await api.delete(`/admin/tables/${currentTable.value}/${deleteTarget.value.id}`)
    deleteTarget.value = null
    fetchData()
  } catch (e) { console.error(e) }
}
</script>

<style scoped>
.table-tabs { display: flex; gap: 4px; flex-wrap: wrap; }
.tab-btn { padding: 6px 16px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--bg-deep); color: var(--text-secondary); font-size: 13px; cursor: pointer; transition: all 0.15s; font-family: inherit; }
.tab-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.tab-active { background: var(--accent); border-color: var(--accent); color: var(--bg-deep); font-weight: 600; }
</style>
