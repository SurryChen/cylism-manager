<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">数据管理</h1>
      <button class="btn btn-primary" @click="showAdd = true" :disabled="!currentTable">+ 新增记录</button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

    <div class="card section-gap">
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
                <th v-for="col in columns" :key="col" class="sortable-header" @click="setSort(col)">
                  {{ col }} {{ sort === col ? (order === 'asc' ? '↑' : '↓') : '' }}
                </th>
                <th>操作</th>
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
        <div class="pagination">
          <button class="btn btn-sm" :disabled="page <= 1" @click="page--; fetchData()">上一页</button>
          <span class="pagination-status">{{ page }} / {{ totalPages }}</span>
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
        <p class="modal-copy">
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
import { getAdminTableRows, getAdminTables } from '../api/admin.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const tables = ref([])
const currentTable = ref('')
const columns = ref([])
const rows = ref([])
const loading = ref(false)
const page = ref(1)
const size = 20
const total = ref(0)
const error = ref('')
const sort = ref('id')
const order = ref('desc')
const showAdd = ref(false)
const editTarget = ref(null)
const deleteTarget = ref(null)
const formData = reactive({})
const tablesResource = useAsyncResource(({ signal }) => getAdminTables({ signal }), { tables: [] })
const rowsResource = useAsyncResource(({ signal }, table, query) => getAdminTableRows(table, query, { signal }), { columns: [], rows: [], total: 0 })

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / size)))
const editableColumns = computed(() => columns.value.filter(c => !['id','created_at','updated_at','deleted_at'].includes(c)))

const tableLabels = { servers:'服务器', sites:'站点', certs:'证书', audit_logs:'审计日志', operation_logs:'操作日志', users:'用户' }
function tableLabel(t) { return tableLabels[t] || t }

onMounted(loadTables)

async function loadTables() {
  error.value = ''
  const data = await tablesResource.refresh()
  if (data) tables.value = data.tables || []
  else if (tablesResource.error.value) error.value = tablesResource.error.value.message || '加载数据表失败'
}

async function selectTable(name) {
  currentTable.value = name
  page.value = 1
  sort.value = 'id'
  order.value = 'desc'
  await fetchData()
}

async function fetchData() {
  if (!currentTable.value) return
  error.value = ''
  loading.value = true
  const data = await rowsResource.refresh(currentTable.value, { page: page.value, size, sort: sort.value, order: order.value })
  if (data) {
    columns.value = data.columns || []
    rows.value = data.rows || []
    total.value = data.total || 0
  } else if (rowsResource.error.value) {
    error.value = rowsResource.error.value.message || '加载数据失败'
  }
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
  } catch (e) { error.value = e.message || '保存记录失败' }
}

function confirmDel(row) { deleteTarget.value = row }

async function doDelete() {
  try {
    await api.delete(`/admin/tables/${currentTable.value}/${deleteTarget.value.id}`)
    deleteTarget.value = null
    fetchData()
  } catch (e) { error.value = e.message || '删除记录失败' }
}
</script>
