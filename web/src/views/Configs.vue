<template>
  <div>
    <div class="page-header"><h1 class="page-title">配置</h1></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ error }}</div>

    <div class="card section-gap">
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab === 'configmaps' }]" @click="selectTab('configmaps')">ConfigMaps</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'secrets' }]" @click="selectTab('secrets')">Secrets</button>
      </div>
    </div>

    <!-- ConfigMaps -->
    <div v-if="activeTab === 'configmaps'" class="card">
      <div v-if="loadedTabs.configmaps && configmaps.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 ConfigMap</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>键数量</th><th>被引用</th><th>年龄</th></tr></thead>
          <tbody>
            <template v-for="cm in safeConfigMaps" :key="cm.namespace + '/' + cm.name">
              <tr class="clickable" @click="toggleCmExpand(cm)">
                <td class="cell-primary">{{ cm.name }}</td><td>{{ cm.namespace }}</td>
                <td>{{ cm.keys_count }}</td>
                <td>{{ (cm.used_by || []).map(u => `${u.kind}/${u.name}`).join(', ') || '-' }}</td>
                <td>{{ cm.age }}</td>
              </tr>
              <tr v-if="expandedCm === cm.namespace + '/' + cm.name" class="detail-row">
                <td colspan="5">
                  <div class="detail-panel">
                    <div class="detail-grid" v-for="(val, key) in cmDetail.data" :key="key">
                      <span class="detail-label">{{ key }}</span><span style="font-family:var(--font-mono);font-size:11px;word-break:break-all">{{ val }}</span>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Secrets -->
    <div v-if="activeTab === 'secrets'" class="card">
      <div v-if="loadedTabs.secrets && secrets.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 Secret</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>类型</th><th>键数量</th><th>被引用</th><th>年龄</th></tr></thead>
          <tbody>
            <template v-for="sec in safeSecrets" :key="sec.namespace + '/' + sec.name">
              <tr class="clickable" @click="toggleSecretExpand(sec)">
                <td class="cell-primary">{{ sec.name }}</td><td>{{ sec.namespace }}</td>
                <td><span class="badge badge-deploying">{{ sec.type }}</span></td>
                <td>{{ sec.keys_count }}</td>
                <td>{{ (sec.used_by || []).map(u => `${u.kind}/${u.name}`).join(', ') || '-' }}</td>
                <td>{{ sec.age }}</td>
              </tr>
              <tr v-if="expandedSecret === sec.namespace + '/' + sec.name" class="detail-row">
                <td colspan="6">
                  <div class="detail-panel">
                    <div class="detail-grid" v-for="(val, key) in secretDetail.data" :key="key">
                      <span class="detail-label">{{ key }}</span>
                      <span style="font-family:var(--font-mono);font-size:11px;word-break:break-all">
                        <template v-if="revealedKeys[key]">{{ val }}</template>
                        <template v-else>••••••</template>
                        <button class="btn btn-sm" style="margin-left:8px" @click="toggleReveal(key)">{{ revealedKeys[key] ? '隐藏' : '显示' }}</button>
                      </span>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, reactive } from 'vue'
import { api } from '../api/index.js'

const activeTab = ref('configmaps')
const configmaps = ref([])
const secrets = ref([])
const loadingTab = ref('')
const error = ref('')
const expandedCm = ref('')
const expandedSecret = ref('')
const cmDetail = ref({})
const secretDetail = ref({})
const revealedKeys = reactive({})
const loadedTabs = reactive({ configmaps: false, secrets: false })

const safeConfigMaps = computed(() => (configmaps.value || []).filter(cm => cm != null))
const safeSecrets = computed(() => (secrets.value || []).filter(s => s != null))

onMounted(() => selectTab('configmaps'))

async function selectTab(tab) {
  activeTab.value = tab
  expandedCm.value = ''
  expandedSecret.value = ''
  if (loadingTab.value === tab) return

  loadingTab.value = tab
  error.value = ''
  try {
    if (tab === 'configmaps') configmaps.value = await api.get('/k8s/configmaps') || []
    else secrets.value = await api.get('/k8s/secrets') || []
    loadedTabs[tab] = true
  } catch (e) {
    error.value = e.message || '加载失败，请检查集群连接'
  } finally {
    loadingTab.value = ''
  }
}

async function toggleCmExpand(cm) {
  const key = cm.namespace + '/' + cm.name
  if (expandedCm.value === key) { expandedCm.value = ''; return }
  expandedCm.value = key
  try {
    cmDetail.value = await api.get(`/k8s/configmaps/${cm.namespace}/${cm.name}`)
  } catch(e) { console.error(e) }
}


async function toggleSecretExpand(sec) {
  const key = sec.namespace + '/' + sec.name
  if (expandedSecret.value === key) { expandedSecret.value = ''; return }
  expandedSecret.value = key
  Object.keys(revealedKeys).forEach(k => delete revealedKeys[k])
  try {
    secretDetail.value = await api.get(`/k8s/secrets/${sec.namespace}/${sec.name}`)
  } catch(e) { console.error(e) }
}
function toggleReveal(key) {
  revealedKeys[key] = !revealedKeys[key]
}
</script>

<style scoped>
.clickable { cursor: pointer; }
.detail-row td { padding: 8px 12px; border-bottom: 0; background: var(--surface-subtle); }
.detail-panel { padding: var(--space-12) 0; }
</style>
