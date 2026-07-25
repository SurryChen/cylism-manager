<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">系统设置</h1>
    </div>

    <div class="settings-grid">
      <section class="card">
        <div class="card-header">
          <h2 class="card-title">Tailscale</h2>
          <span class="badge" :class="tailscale.initialized ? 'badge-online' : 'badge-offline'">
            {{ tailscale.initialized ? '已配置' : '待配置' }}
          </span>
        </div>
        <div class="detail-grid">
          <span class="detail-label">控制面 IP</span><span>{{ tailscale.ip || '-' }}</span>
          <span class="detail-label">在线状态</span><span>{{ tailscale.online ? '在线' : '离线' }}</span>
        </div>
        <p class="settings-copy" style="margin-top:var(--space-12)">
          Tailscale Auth Key、服务器导入和组网相关配置统一收敛到系统设置与服务器页，不再单独占一个基础设施页面。
        </p>
      </section>

      <section class="card">
        <div class="card-header">
          <h2 class="card-title">集群引导</h2>
        </div>
        <p class="settings-copy">
          Worker 加入所需的 join token、默认 SSH 超时和更多平台级设置会在这里集中管理。
        </p>
        <span class="badge badge-offline">本轮先提供入口与状态展示</span>
      </section>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const tailscale = ref({ initialized: false, ip: '', online: false })

onMounted(async () => {
  try {
    tailscale.value = await api.get('/tailscale/status')
  } catch (_) {
    tailscale.value = { initialized: false, ip: '', online: false }
  }
})
</script>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-16);
}

.settings-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
</style>
