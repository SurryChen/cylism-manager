<template>
  <BaseModal :open="Boolean(instance) && !session" title="登录数据库终端" size="medium" teleport @close="close"><div v-if="instance" class="terminal-form"><p class="form-hint">{{ instance.name }} · 通过所选节点建立 TLS 连接，使用平台已保存的账号。</p><label class="form-group"><span class="form-label">账号</span><SelectMenu v-model="form.account" aria-label="终端账号" :options="accountOptions" /></label><label class="form-group"><span class="form-label">执行集群</span><SelectMenu v-model="form.cluster_id" aria-label="终端执行集群" :options="clusterOptions" :disabled="Boolean(fixedCluster)" @change="loadNodes" /></label><label class="form-group"><span class="form-label">执行节点</span><SelectMenu v-model="form.node_name" aria-label="终端执行节点" :options="nodeOptions" /></label><p class="form-hint">会话最长 30 分钟，关闭后清理临时客户端。命令直接使用所选账号的权限。</p></div><template #actions><button class="btn" :disabled="opening" @click="close">取消</button><button class="btn btn-primary" :disabled="opening || !form.account || !form.node_name" @click="open">{{ opening ? '创建客户端...' : '打开终端' }}</button></template></BaseModal>
  <PodTerminal v-if="session" :pod="{ namespace: instance.name, name: `${session.username} · ${session.node_name}`, containers: ['mysql'] }" :database-session="session" @close="close" @reconnect="reconnect" @configure="cleanup" />
  <ErrorNoticeModal :open="Boolean(error)" title="数据库终端连接失败" :message="error" teleport @close="error = ''" />
</template>
<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import PodTerminal from '../resources/PodTerminal.vue'
import { createDatabaseSession, deleteDatabaseSession, getDatabaseExecutionNodes } from '../../api/databases.js'
const props = defineProps({ instance: { type: Object, default: null }, clusters: { type: Array, default: () => [] } })
const emit = defineEmits(['close'])
const form = reactive({ account: 'admin', cluster_id: 0, node_name: '' }), nodes = ref([]), session = ref(null), opening = ref(false), error = ref('')
const fixedCluster = computed(() => props.instance?.mode === 'managed' ? props.instance.cluster_id : props.instance?.private_gateway?.cluster_id || 0)
const clusterOptions = computed(() => [{ value: 0, label: '选择集群' }, ...props.clusters.map(item => ({ value: item.id, label: item.name }))])
const nodeOptions = computed(() => [{ value: '', label: '选择可访问数据库的节点' }, ...nodes.value.filter(item => item.ready).map(item => ({ value: item.name, label: item.name }))])
const accountOptions = computed(() => [ ...(props.instance?.mode === 'managed' || props.instance?.admin_username ? [{ value: 'admin', label: `管理账号 · ${props.instance?.admin_username || 'root'}` }] : []), ...(props.instance?.username ? [{ value: 'business', label: `业务账号 · ${props.instance.username}` }] : []) ])
let generation = 0
async function loadNodes() {
  const request = ++generation
  form.node_name = ''; nodes.value = []
  if (!form.cluster_id) return
  try { const result = await getDatabaseExecutionNodes(form.cluster_id); if (request !== generation) return; nodes.value = result; if (nodeOptions.value.some(item => item.value === props.instance?.node_name)) form.node_name = props.instance.node_name }
  catch (err) { if (request === generation) error.value = err.message || '读取节点失败' }
}
async function open() {
  if (opening.value) return
  const request = generation, id = props.instance.id
  opening.value = true
  try { const result = await createDatabaseSession(id, { ...form }); if (request !== generation) { await deleteDatabaseSession(id, result.id); return }; session.value = result }
  catch (err) { if (request === generation) error.value = err.message || '创建终端失败' }
  finally { opening.value = false }
}
function cleanup() { generation++; const previous = session.value; session.value = null; if (previous) deleteDatabaseSession(previous.instance_id, previous.id).catch(() => {}) }
function reconnect() { cleanup(); open() }
function close() { if (opening.value) return; cleanup(); emit('close') }
onBeforeUnmount(cleanup)
watch(() => props.instance?.id, () => { cleanup(); if (props.instance) { Object.assign(form, { account: accountOptions.value[0]?.value || '', cluster_id: fixedCluster.value || 0, node_name: '' }); loadNodes() } }, { immediate: true })
</script>
<style scoped>.terminal-form { display: grid; gap: 12px; }.terminal-form .form-group { margin: 0; }</style>
