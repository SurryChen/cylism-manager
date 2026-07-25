<template>
  <div>
    <div class="page-header"><h1 class="page-title">组网配置</h1></div>

    <!-- 方案信息 + Auth Key -->
    <section class="card section-gap">
      <div class="card-header">
        <div>
          <h2 class="card-title">组网方案: Tailscale</h2>
          <span v-if="tsStatus.initialized" class="badge badge-online" style="margin-left:var(--space-8)">已配置</span>
          <span v-else class="badge badge-offline" style="margin-left:var(--space-8)">未配置</span>
        </div>
      </div>
      <div v-if="error" class="err-msg">{{ error }}</div>
      <div v-if="successMsg" class="ok-msg">{{ successMsg }}</div>
      <form @submit.prevent="initTailscale" class="form-row">
        <input v-model="authKey" class="form-input" placeholder="tskey-auth-xxxxxxxxxxxx" style="flex:1" />
        <button type="submit" class="btn btn-primary" :disabled="initing">
          {{ initing ? '提交中...' : tsStatus.initialized ? '覆盖更新' : '初始化' }}
        </button>
      </form>
      <p style="margin-top:8px;font-size:12px;color:var(--text-muted)">
        前往 <a href="https://login.tailscale.com/admin/settings/keys" target="_blank" style="color:var(--color-accent)">Tailscale 控制台</a> 生成密钥。
        <template v-if="tsStatus.initialized">重新填入将<span style="color:var(--color-warn)">覆盖</span>现有配置。</template>
      </p>
    </section>

    <!-- 网络概览卡片 -->
    <div class="mini-card-grid">
      <div class="mini-card">
        <div class="mini-card-value">{{ onlineCount }}</div>
        <div class="mini-card-label">在线节点</div>
      </div>
      <div class="mini-card">
        <div class="mini-card-value">{{ totalNodeCount }}</div>
        <div class="mini-card-label">已组网节点</div>
      </div>
      <div class="mini-card">
        <div class="mini-card-value">{{ servers.length }}</div>
        <div class="mini-card-label">已注册服务器</div>
      </div>
      <div class="mini-card">
        <div class="mini-card-value" style="font-size:14px;word-break:break-all">{{ tsStatus.ip || '-' }}</div>
        <div class="mini-card-label">本机 Tailscale IP</div>
      </div>
    </div>

    <!-- 节点列表 -->
    <section class="card section-gap">
      <div class="card-header"><h2 class="card-title">节点列表</h2></div>
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无已注册服务器</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>节点名称</th><th>主机地址</th><th>TS IP</th><th>TS 状态</th><th>集群角色</th><th>K8s 节点</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id">
              <td class="cell-primary">{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td>{{ srv.tailscale_ip || '-' }}</td>
              <td>
                <span v-if="srv.tailscale_online" class="dot dot-green"></span>
                <span v-else class="dot dot-gray"></span>
                {{ srv.tailscale_online ? '在线' : srv.tailscale_ip ? '离线' : '-' }}
              </td>
              <td>{{ roleLabel(srv.cluster_role) }}</td>
              <td>{{ srv.k8s_node_name || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 手动安装命令 -->
    <section v-if="installCmd" class="card section-gap">
      <div class="card-header"><h2 class="card-title">新节点手动安装</h2></div>
      <code style="font-size:11px;word-break:break-all;color:var(--text-muted)">{{ installCmd }}</code>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/index.js'

const servers = ref([])
const tsStatus = ref({ initialized: false, ip: '', online: false })
const authKey = ref('')
const initing = ref(false)
const error = ref('')
const successMsg = ref('')
const installCmd = ref('')

const onlineCount = computed(() => servers.value.filter(s => s.tailscale_online).length)
const totalNodeCount = computed(() => servers.value.filter(s => s.tailscale_ip).length)

function roleLabel(r) {
  if (!r) return '-'
  if (r === 'control-plane') return '控制平面'
  if (r === 'worker') return '工作节点'
  return r
}

onMounted(async () => {
  try { tsStatus.value = await api.get('/tailscale/status') } catch (_) {}
  try { servers.value = await api.get('/servers') || [] } catch (_) {}
  try { const s = await api.get('/tailscale/install-script'); installCmd.value = s.command } catch (_) {}
})

async function initTailscale() {
  if (!authKey.value) return
  initing.value = true
  error.value = ''
  successMsg.value = ''
  try {
    await api.post('/tailscale/init', { auth_key: authKey.value })
    tsStatus.value = await api.get('/tailscale/status')
    authKey.value = ''
    successMsg.value = '配置已更新'
    setTimeout(() => { successMsg.value = '' }, 4000)
  } catch (e) {
    error.value = e.message || '初始化失败'
  }
  initing.value = false
}
</script>

<style scoped>
.mini-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: var(--space-12);
  margin-bottom: var(--space-16);
}
.mini-card {
  background: var(--surface);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-control);
  padding: var(--space-12);
}
.mini-card-value {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 4px;
}
.mini-card-label {
  font-size: 11px;
  color: var(--text-muted);
}
.dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  display: inline-block;
  vertical-align: middle;
  margin-right: 4px;
}
.dot-green { background: var(--color-success); }
.dot-gray  { background: var(--text-muted); }
.err-msg {
  margin-bottom: 10px; padding: 6px 10px;
  background: var(--danger-surface); border: 1px solid var(--danger);
  border-radius: var(--radius-control); color: var(--danger); font-size: 13px;
}
.ok-msg {
  margin-bottom: 10px; padding: 6px 10px;
  background: var(--success-surface); border: 1px solid var(--color-success);
  border-radius: var(--radius-control); color: var(--color-success); font-size: 13px;
}
</style>
