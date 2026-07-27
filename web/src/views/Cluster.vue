<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">集群节点</h1>
    </div>

    <div class="card section-gap">
      <p class="section-copy">
        节点状态直接来自当前 K3s 集群。新增工作节点请先在“服务器”页面补全 SSH 信息并完成加入集群。
      </p>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <div class="card">
      <div v-if="nodes.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 K8s 节点</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>节点名</th><th>状态</th><th>角色</th><th>K8s 版本</th><th>IP</th><th>映射服务器</th><th>CPU</th><th>内存</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodes" :key="node.name">
              <td class="cell-primary">{{ node.name }}</td>
              <td>
                <span class="badge" :class="node.ready ? 'badge-online' : 'badge-offline'">
                  {{ node.ready ? '就绪' : '未就绪' }}
                </span>
              </td>
              <td>{{ node.roles || '-' }}</td>
              <td>{{ node.version || '-' }}</td>
              <td>{{ node.internal_ip || '-' }}</td>
              <td>
                <span v-if="mappedServer(node)" class="badge badge-online">{{ mappedServer(node).name }}</span>
                <span v-else class="badge badge-offline">未映射</span>
              </td>
              <td>{{ node.cpu_cores || '-' }}核</td>
              <td>{{ node.memory_mb ? (node.memory_mb / 1024).toFixed(1) + 'G' : '-' }}</td>
              <td>
                <div class="btn-group action-cell">
                  <button class="btn btn-sm" @click="drainNode(node.name)">驱逐</button>
                  <button class="btn btn-sm btn-danger" @click="confirmRemoveNode(node)">移出</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="drainTarget" class="overlay" @click.self="drainTarget = null">
      <div class="modal">
        <h2 class="modal-title">驱逐节点</h2>
        <p class="modal-copy">确定驱逐 <strong>{{ drainTarget.name }}</strong> 吗？</p>
        <div class="modal-actions">
          <button class="btn" @click="drainTarget = null">取消</button>
          <button class="btn btn-danger" @click="doDrain">确认驱逐</button>
        </div>
      </div>
    </div>

    <div v-if="removeTarget" class="overlay" @click.self="removeTarget = null">
      <div class="modal">
        <h2 class="modal-title">移出集群</h2>
        <p class="modal-copy">确定将 <strong>{{ removeTarget.name }}</strong> 从集群中移出吗？</p>
        <div class="modal-actions">
          <button class="btn" @click="removeTarget = null">取消</button>
          <button class="btn btn-danger" @click="doRemoveNode">确认移出</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const nodes = ref([])
const servers = ref([])
const error = ref('')
const drainTarget = ref(null)
const removeTarget = ref(null)

const serverLookup = computed(() => servers.value)

onMounted(() => {
  fetchData()
})

async function fetchData() {
  error.value = ''
  try {
    const [nodeList, serverList] = await Promise.all([
      api.get('/nodes'),
      api.get('/servers'),
    ])
    nodes.value = nodeList || []
    servers.value = serverList || []
  } catch (e) {
    error.value = e.message || '加载节点失败'
  }
}

function mappedServer(node) {
  return serverLookup.value.find(server => {
    // 优先用 IP 匹配（server.host === node.internal_ip）
    if (server.host && server.host === node.internal_ip) return true
    // fallback 用 k8s_node_name（忽略大小写）
    if (server.k8s_node_name && server.k8s_node_name.toLowerCase() === node.name.toLowerCase()) return true
    return false
  })
}

function drainNode(name) {
  drainTarget.value = nodes.value.find(node => node.name === name) || { name }
}

async function doDrain() {
  if (!drainTarget.value) return
  try {
    await api.post(`/nodes/${drainTarget.value.name}/drain`)
    drainTarget.value = null
    fetchData()
  } catch (e) {
    error.value = e.message || '驱逐失败'
  }
}

function confirmRemoveNode(node) {
  removeTarget.value = node
}

async function doRemoveNode() {
  if (!removeTarget.value) return
  try {
    await api.delete(`/nodes/${removeTarget.value.name}`)
    removeTarget.value = null
    fetchData()
  } catch (e) {
    error.value = e.message || '移出失败'
  }
}
</script>

<style scoped>
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
</style>
