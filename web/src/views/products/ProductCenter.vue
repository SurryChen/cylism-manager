<template>
  <div class="product-page">
    <SectionTabsHeader title="产品中心" :tabs="tabs" :active-tab="tab" @select="selectTab"><template #actions><span class="product-muted">共 {{ filtered.length }} 个产品</span></template></SectionTabsHeader>
    <SurfaceCard class="product-table-card" padding="none">
      <div class="product-toolbar">
        <input v-model.trim="keyword" class="form-input product-search" aria-label="搜索产品" placeholder="搜索名称、简介或下一步行动" @input="page = 1" />
        <SelectMenu v-model="stage" class="product-stage-filter" aria-label="阶段筛选" @update:model-value="page = 1"><option value="">全部阶段</option><option v-for="item in visibleStages" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <button class="icon-button" title="重置筛选" aria-label="重置筛选" @click="keyword = ''; stage = ''; page = 1"><RotateCcw :size="15" /></button>
        <button class="btn btn-primary product-primary" :disabled="busy" @click="openCreate">创建产品</button>
      </div>
      <EmptyState v-if="resource.loading.value" variant="loading" message="正在读取产品..." />
      <EmptyState v-else-if="resource.error.value" message="产品读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
      <EmptyState v-else-if="!filtered.length" :message="tab === 'archived' ? '暂无已归档产品' : '暂无产品，可先记录一个想法'" />
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>产品</th><th>阶段</th><th>下一步行动</th><th>截止日期</th><th>发行渠道</th><th>操作</th></tr></thead><tbody>
        <tr v-for="product in rows" :key="product.id">
          <td><div class="product-title-cell"><span class="product-mark"><img v-if="product.icon_url" :src="product.icon_url" alt="" referrerpolicy="no-referrer" @error="$event.target.style.display = 'none'" /><template v-else>{{ product.identifier || product.name.slice(0, 1) }}</template></span><div><router-link :to="`/products/${product.id}`">{{ product.name }}</router-link><span class="cell-secondary product-line">{{ product.summary || '暂无简介' }}</span></div></div></td>
          <td><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span></td>
          <td>{{ product.next_action || '—' }}</td><td>{{ dateOnly(product.next_due_date) || '—' }}</td>
          <td>{{ product.channels?.length || 0 }} 个</td><td><router-link class="btn btn-sm" :to="`/products/${product.id}`">查看详情</router-link></td>
        </tr>
      </tbody></table></div>
      <div v-if="filtered.length > pageSize" class="pagination"><button class="btn btn-sm" :disabled="page === 1" @click="page--">上一页</button><span>{{ page }} / {{ totalPages }}</span><button class="btn btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button></div>
    </SurfaceCard>
    <ProductEditor :open="editorOpen" :busy="busy" :options="options" @close="editorOpen = false" @save="save" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { RotateCcw } from 'lucide-vue-next'
import { getProducts, getProductResourceOptions, createProduct } from '../../api/products.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ProductEditor from './ProductEditor.vue'
import { stages, stageClass, labelFor, dateOnly } from './product-utils.js'
import '../../styles/products.css'

const router = useRouter()
const resource = useAsyncResource(getProducts, [])
const editorOpen = ref(false), busy = ref(false), error = ref('')
const options = ref({ projects: [], applications: [] })
const tab = ref('current'), keyword = ref(''), stage = ref(''), page = ref(1)
const pageSize = 20
const tabs = [{ id: 'current', label: '当前产品' }, { id: 'archived', label: '已归档' }]
const visibleStages = computed(() => stages.filter(item => tab.value === 'archived' ? item.id === 'archived' : item.id !== 'archived'))
const filtered = computed(() => (resource.data.value || []).filter(product => (tab.value === 'archived' ? product.stage === 'archived' : product.stage !== 'archived') && (!stage.value || product.stage === stage.value) && `${product.name} ${product.summary} ${product.next_action}`.toLowerCase().includes(keyword.value.toLowerCase())))
const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)))
const rows = computed(() => filtered.value.slice((page.value - 1) * pageSize, page.value * pageSize))
function selectTab(value) { tab.value = value; stage.value = ''; page.value = 1 }
async function load() { await resource.refresh(); if (resource.error.value) error.value = resource.error.value.message }
async function openCreate() {
  busy.value = true
  try { options.value = await getProductResourceOptions(); editorOpen.value = true } catch (e) { error.value = e.message } finally { busy.value = false }
}
async function save(payload) {
  busy.value = true
  try { const product = await createProduct(payload); editorOpen.value = false; await router.push(`/products/${product.id}`) } catch (e) { error.value = e.message } finally { busy.value = false }
}
onMounted(load)
</script>
