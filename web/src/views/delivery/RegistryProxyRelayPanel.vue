<template>
  <Teleport to="body">
    <div class="overlay" @click.self="$emit('close')">
      <div class="modal relay-modal" role="dialog" aria-modal="true" :aria-label="`${proxy.name} 中转服务`">
        <div class="relay-heading"><h2 class="modal-title">{{ proxy.name }} · 中转服务</h2><button class="icon-button" title="关闭" aria-label="关闭" @click="$emit('close')"><X :size="16" /></button></div>
        <p v-if="error" class="form-error" role="alert">{{ error }}</p>
        <div class="relay-section-heading"><h3>中转入口</h3><button class="icon-button" title="刷新中转服务" aria-label="刷新中转服务" @click="load"><RefreshCw :size="15" /></button></div>
        <div class="relay-table-wrap"><table class="data-table"><thead><tr><th>中转机器</th><th>镜像地址</th><th>状态</th><th>上次检查</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="relay in relays" :key="relay.id"><td>{{ serverName(relay.server_id) }}</td><td><div class="relay-endpoint"><code class="relay-mono">{{ relayEndpoint(relay) }}</code><button class="icon-button" type="button" :title="`复制 ${relayEndpoint(relay)}`" :aria-label="`复制 ${relayEndpoint(relay)}`" @click="copyEndpoint(relay)"><Copy :size="14" /></button></div></td><td><span class="badge" :class="relay.status === 'ready' ? 'badge-online' : 'badge-danger'" :title="relay.last_error">{{ relay.status === 'ready' ? '就绪' : '异常' }}</span></td><td>{{ dateText(relay.last_checked_at) }}</td><td class="action-cell"><div class="relay-actions"><button v-if="relay.status === 'failed'" class="btn btn-sm" :disabled="Boolean(busy)" @click="retryRelay(relay)">重试</button><button class="btn btn-sm" :disabled="Boolean(busy)" @click="checkRelay(relay)">检查</button><button class="btn btn-sm btn-danger" :disabled="Boolean(busy)" @click="deleteTarget = relay">移除</button></div></td></tr><tr v-if="!relays.length"><td colspan="5" class="relay-empty">{{ loading ? '正在读取中转入口...' : '尚无中转入口' }}</td></tr></tbody></table></div>
        <form class="relay-form relay-deploy-form" @submit.prevent="createRelay">
          <div class="relay-field relay-server-field"><label class="form-label" for="relay-server">中转机器</label><SelectMenu id="relay-server" v-model="relayForm.server_id" aria-label="中转机器" required><option value="" disabled>选择中转机器</option><option v-for="server in servers" :key="server.id" :value="String(server.id)">{{ server.name }} · {{ server.host }}</option></SelectMenu></div>
          <div class="relay-field relay-ip-field"><label class="form-label" for="relay-listen-ip">监听 IP</label><input id="relay-listen-ip" v-model.trim="relayForm.listen_ip" class="form-input" required inputmode="decimal" placeholder="例如 100.67.232.15" /></div>
          <div class="relay-field relay-port-field"><label class="form-label" for="relay-listen-port">端口</label><input id="relay-listen-port" v-model.number="relayForm.listen_port" class="form-input" required type="number" min="1024" max="65535" /></div>
          <button class="btn btn-primary relay-submit" :disabled="Boolean(busy)">{{ busy === 'create' ? '部署中...' : '部署中转' }}</button>
        </form>
        <div class="relay-footer"><span>在节点镜像源中，将 {{ proxy.registry }} 的镜像地址设为上方中转地址。</span><RouterLink v-if="proxy.cluster_id" class="btn" :to="`/cluster/${proxy.cluster_id}?tab=registry-mirrors`" @click="$emit('close')">打开节点镜像源</RouterLink></div>
      </div>
    </div>
    <div v-if="deleteTarget" class="overlay relay-confirm-overlay" @click.self="deleteTarget = null"><div class="modal relay-confirm" role="dialog" aria-modal="true" aria-label="确认移除中转"><h2 class="modal-title">移除中转入口</h2><p>将停止 {{ serverName(deleteTarget.server_id) }} 上的 HAProxy 服务。使用该入口的节点需要先切回其他入口。</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="Boolean(busy)" @click="deleteRelay">确认移除</button></div></div></div>
  </Teleport>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { Copy, X, RefreshCw } from 'lucide-vue-next'
import { getRegistryProxyRelays, createRegistryProxyRelay, checkRegistryProxyRelay, retryRegistryProxyRelay, deleteRegistryProxyRelay } from '../../api/registry-proxies.js'

const props = defineProps({ proxy: { type: Object, required: true }, servers: { type: Array, default: () => [] } })
defineEmits(['close'])
const relays = ref([])
const error = ref('')
const busy = ref('')
const loading = ref(false)
const relayForm = ref({ server_id: '', listen_ip: '', listen_port: 31501 })
const deleteTarget = ref(null)
function serverName(id) { return props.servers.find(server => server.id === id)?.name || `节点 #${id}` }
function dateText(value) { return value ? new Date(value).toLocaleString('zh-CN') : '-' }
function relayEndpoint(relay) { return `http://${relay.listen_ip}:${relay.listen_port}` }
async function copyEndpoint(relay) { try { await navigator.clipboard.writeText(relayEndpoint(relay)) } catch { error.value = '复制失败，请手动复制镜像地址' } }
async function load() { loading.value = true; error.value = ''; try { relays.value = await getRegistryProxyRelays(props.proxy.id) || [] } catch (err) { error.value = err.message || '读取中转入口失败' } finally { loading.value = false } }
async function createRelay() { busy.value = 'create'; error.value = ''; try { await createRegistryProxyRelay(props.proxy.id, { server_id: Number(relayForm.value.server_id), listen_ip: relayForm.value.listen_ip, listen_port: Number(relayForm.value.listen_port) }); relayForm.value = { server_id: '', listen_ip: '', listen_port: 31501 }; await load() } catch (err) { error.value = err.message || '部署中转失败' } finally { busy.value = '' } }
async function checkRelay(relay) { busy.value = 'check'; error.value = ''; try { await checkRegistryProxyRelay(props.proxy.id, relay.id); await load() } catch (err) { error.value = err.message || '检查中转失败' } finally { busy.value = '' } }
async function retryRelay(relay) { busy.value = 'retry'; error.value = ''; try { await retryRegistryProxyRelay(props.proxy.id, relay.id); await load() } catch (err) { const message = err.message || '重试中转失败'; await load(); error.value = message } finally { busy.value = '' } }
async function deleteRelay() { busy.value = 'delete'; error.value = ''; try { await deleteRegistryProxyRelay(props.proxy.id, deleteTarget.value.id); deleteTarget.value = null; await load() } catch (err) { error.value = err.message || '移除中转失败'; deleteTarget.value = null } finally { busy.value = '' } }
onMounted(load)
</script>

<style scoped>
.relay-modal { width: min(760px, calc(100vw - 32px)); max-height: calc(100vh - 48px); overflow: auto; }
.relay-heading, .relay-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.relay-heading .modal-title { margin: 0; }
.relay-section-heading { margin-top: 24px; margin-bottom: 8px; border-bottom: 1px solid var(--border-muted); }
.relay-section-heading h3 { font-size: 14px; font-weight: 650; margin: 0 0 10px; }
.relay-table-wrap { overflow-x: auto; }
.relay-table-wrap .data-table { min-width: 590px; }
.relay-table-wrap .data-table th, .relay-table-wrap .data-table td { padding: 9px 12px; }
.relay-actions { display: flex; justify-content: flex-end; gap: 6px; }
.relay-mono { font-family: var(--font-mono); font-size: 11px; }
.relay-endpoint { display: flex; align-items: center; gap: 4px; white-space: nowrap; }
.relay-endpoint .icon-button { width: 30px; height: 30px; flex: 0 0 30px; }
.relay-empty { text-align: center; color: var(--text-secondary); }
.relay-form { display: grid; align-items: end; gap: 10px; margin-top: 16px; }
.relay-deploy-form { grid-template-columns: minmax(220px, 2fr) minmax(190px, 1.35fr) minmax(92px, .55fr) auto; }
.relay-field { min-width: 0; }
.relay-field .form-label { margin-bottom: 6px; }
.relay-field .form-input, .relay-field :deep(.select-menu) { width: 100%; min-width: 0; }
.relay-field .form-input, .relay-field :deep(.select-menu-trigger), .relay-submit { height: 36px; min-height: 36px; }
.relay-submit { white-space: nowrap; }
.relay-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 18px; color: var(--text-secondary); font-size: 12px; }
.relay-confirm-overlay { z-index: 1400; }
.relay-confirm { width: min(420px, calc(100vw - 32px)); }
@media (max-width: 760px) { .relay-deploy-form { grid-template-columns: repeat(2, minmax(0, 1fr)); }.relay-server-field { grid-column: 1 / -1; }.relay-submit { justify-self: end; } }
@media (max-width: 520px) { .relay-deploy-form { grid-template-columns: minmax(0, 1fr); }.relay-server-field { grid-column: auto; }.relay-submit { width: 100%; }.relay-footer { align-items: stretch; flex-direction: column; } }
</style>
