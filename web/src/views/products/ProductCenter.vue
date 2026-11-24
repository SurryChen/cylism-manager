<template>
  <div class="product-page">
    <SectionTabsHeader :title="projectMode ? '项目中心' : '产品中心'" :tabs="tabs" :active-tab="tab" @select="selectTab"><template #actions><span class="product-muted">共 {{ total }} 个{{ projectMode ? '项目' : '产品' }}</span></template></SectionTabsHeader>
    <SurfaceCard class="product-table-card" padding="none">
      <div class="product-toolbar">
        <input v-model.trim="keyword" class="form-input product-search" :aria-label="projectMode ? '搜索项目' : '搜索产品'" placeholder="搜索名称或简介" />
        <SelectMenu v-model="stage" class="product-stage-filter" aria-label="阶段筛选"><option value="">全部阶段</option><option v-for="item in visibleStages" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <button class="icon-button" title="重置筛选" aria-label="重置筛选" @click="keyword = ''; stage = ''"><RotateCcw :size="15" /></button>
        <div class="product-actions product-primary"><button v-if="!projectMode" class="btn" :disabled="busy" @click="deleteTarget = null; clearOpen = true">清空全部产品</button><button class="btn btn-primary" :disabled="busy" @click="openCreate">{{ projectMode ? '创建项目' : '创建产品' }}</button></div>
      </div>
      <EmptyState v-if="loading && !rows.length" variant="loading" :message="projectMode ? '正在读取项目...' : '正在读取产品...'" />
      <EmptyState v-else-if="resource.error.value" :message="projectMode ? '项目读取失败' : '产品读取失败'"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
      <EmptyState v-else-if="!total" :message="projectMode ? (tab === 'archived' ? '暂无已归档项目' : '暂无项目，可先记录一个想法') : (tab === 'archived' ? '暂无已归档产品' : '暂无产品，可先记录一个想法')" />
      <div v-else class="table-wrap"><DataTable><thead><tr><th>{{ projectMode ? '项目' : '产品' }}</th><th>阶段</th><th>发行渠道</th><TableActionCell header /></tr></thead><tbody>
        <tr v-for="product in rows" :key="product.id">
          <td><div class="product-title-cell"><span class="product-mark"><img v-if="product.icon_url" :src="product.icon_url" alt="" referrerpolicy="no-referrer" @error="$event.target.style.display = 'none'" /><template v-else>{{ product.name.slice(0, 1) }}</template></span><div><router-link :to="detailPath(product)">{{ product.name }}</router-link><span class="cell-secondary product-line">{{ product.summary || '暂无简介' }}</span></div></div></td>
          <td><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span></td>
          <td>{{ product.channels?.length || 0 }} 个</td><TableActionCell><div class="btn-group"><router-link class="btn btn-sm" :to="detailPath(product)">查看详情</router-link><button class="icon-button" :title="`删除${projectMode ? '项目' : '产品'}：${product.name}`" :aria-label="`删除${projectMode ? '项目' : '产品'}：${product.name}`" :disabled="busy" @click="deleteTarget = product"><Trash2 :size="15" /></button></div></TableActionCell>
        </tr>
      </tbody></DataTable></div>
      <PaginationControls :page="page" :page-size="pageSize" :total="total" :loading="loading" @page-change="changePage" @page-size-change="changePageSize" />
    </SurfaceCard>
    <ProductEditor :project-mode="projectMode" :open="editorOpen" :busy="busy" @close="editorOpen = false" @save="save" />
    <ProductDeleteDialog :project-mode="projectMode" :open="clearOpen || Boolean(deleteTarget)" :all="clearOpen" :product="deleteTarget" :busy="busy" @cancel="clearOpen = false; deleteTarget = null" @confirm="remove" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import DataTable from '../../components/DataTable.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { RotateCcw, Trash2 } from 'lucide-vue-next'
import { getProductPage, createProduct, deleteProduct, clearProducts, deleteProjectWorkspace } from '../../api/products.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import PaginationControls from '../../components/PaginationControls.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ProductEditor from './ProductEditor.vue'
import ProductDeleteDialog from './ProductDeleteDialog.vue'
import { stages, stageClass, labelFor } from './product-utils.js'
import '../../styles/products.css'

const props = defineProps({ projectMode: Boolean })
const router = useRouter()
function detailPath(product) { return props.projectMode ? '/projects/' + product.project_id : '/products/' + product.id }
const resource = usePaginatedResource(getProductPage, 'items')
const { items: rows, total, page, pageSize, loading, changePage, changePageSize } = resource
const editorOpen = ref(false), busy = ref(false), error = ref('')
const deleteTarget = ref(null), clearOpen = ref(false)
const tab = ref('current'), keyword = ref(''), stage = ref('')
const tabs = [{ id: 'current', label: props.projectMode ? '当前项目' : '当前产品' }, { id: 'archived', label: '已归档' }]
const visibleStages = computed(() => stages.filter(item => tab.value === 'archived' ? item.id === 'archived' : item.id !== 'archived'))
function selectTab(value) { tab.value = value; stage.value = '' }
async function load() { await resource.load({ scope: tab.value, stage: stage.value, keyword: keyword.value }); if (resource.error.value) error.value = resource.error.value.message }
watch([tab, stage, keyword], load)
watch(resource.error, cause => { if (cause) error.value = cause.message })
function openCreate() { editorOpen.value = true }
async function save(payload) {
  busy.value = true
  try { const product = await createProduct(payload); editorOpen.value = false; await router.push(detailPath(product)) } catch (e) { error.value = e.message } finally { busy.value = false }
}
async function remove(confirmation) {
  if (busy.value || (!clearOpen.value && !deleteTarget.value)) return
  busy.value = true
  try {
    if (clearOpen.value) await clearProducts(confirmation)
    else if (props.projectMode) await deleteProjectWorkspace(deleteTarget.value.project_id)
    else await deleteProduct(deleteTarget.value.id)
    clearOpen.value = false; deleteTarget.value = null
    await resource.refresh()
  } catch (cause) { error.value = cause.message } finally { busy.value = false }
}
onMounted(load)
</script>
