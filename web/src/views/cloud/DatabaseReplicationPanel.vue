<template>
  <section class="replication-panel">
    <div class="replication-toolbar"><div><h3>复制关系 <span>{{ relations.length }}</span></h3><p class="cell-secondary">状态以最近一次检查为准</p></div><button class="btn" @click="$emit('discover')"><Search :size="15" />发现已有复制</button></div>
    <EmptyState v-if="loading" variant="loading" message="读取复制关系..." />
    <EmptyState v-else-if="!relations.length" message="尚未登记复制关系" description="已有从库可通过“发现已有复制”核对并登记；此处为空不代表数据库没有运行复制。" />
    <article v-for="relation in relations" :key="relation.id" class="replication-relation">
      <header class="relation-heading"><span class="badge" :class="relation.last_checked_at ? (relation.status === 'running' ? 'badge-online' : 'badge-danger') : 'badge-offline'">{{ relation.last_checked_at ? (relation.status === 'running' ? '线程正常' : '需检查') : '待检查' }}</span><span class="cell-secondary">{{ relation.last_checked_at ? `检查于 ${formatTime(relation.last_checked_at)}` : '尚无当前状态记录' }}</span></header>
      <div class="replication-path"><div class="replication-endpoint"><span class="endpoint-role">源实例</span><strong>{{ resource(relation.source).name }}</strong><span class="cell-secondary">{{ resource(relation.source).location }}</span></div><ArrowRight :size="24" class="replication-arrow" aria-label="复制到" /><div class="replication-endpoint"><span class="endpoint-role">从实例</span><strong>{{ resource(relation.target).name }}</strong><span class="cell-secondary">{{ resource(relation.target).location }}</span></div></div>
      <dl class="replication-facts"><div><dt>I/O 线程</dt><dd>{{ threadLabel(relation, relation.io_running) }}</dd></div><div><dt>SQL 线程</dt><dd>{{ threadLabel(relation, relation.sql_running) }}</dd></div><div><dt>复制延迟</dt><dd>{{ relation.last_checked_at && relation.lag_seconds != null ? `${relation.lag_seconds} 秒` : '未知' }}</dd></div><div><dt>从库模式</dt><dd>{{ relation.last_checked_at ? (relation.read_only ? '只读' : '可写') : '待检查' }}</dd></div></dl>
      <div class="relation-channel"><span class="cell-secondary">通道</span><code>{{ relation.channel || '默认通道' }}</code></div>
      <details v-if="relation.observation" class="relation-details"><summary>复制范围与诊断</summary><dl><div><dt>数据库过滤</dt><dd>{{ observation(relation).schemas || '未设置' }}</dd></div><div><dt>数据表过滤</dt><dd>{{ observation(relation).tables || '未设置' }}</dd></div><div><dt>线程错误码（I/O / SQL）</dt><dd>{{ observation(relation).io_errno || 0 }} / {{ observation(relation).sql_errno || 0 }}</dd></div></dl></details>
      <footer class="relation-actions"><button class="btn btn-primary" @click="$emit('check', relation)">检查当前状态</button><button class="btn" @click="$emit('detach', relation)">解除复制</button></footer>
    </article>
  </section>
</template>

<script setup>
import { ArrowRight, Search } from 'lucide-vue-next'
import EmptyState from '../../components/EmptyState.vue'
const props = defineProps({ relations: { type: Array, default: () => [] }, resources: { type: Array, default: () => [] }, loading: Boolean })
defineEmits(['discover', 'check', 'detach'])
const resource = ref => props.resources.find(item => item.ref === ref) || { name: ref, location: '' }
const threadLabel = (relation, running) => relation.last_checked_at ? (running ? '运行中' : '未运行') : '待检查'
const formatTime = time => new Date(time).toLocaleString('zh-CN', { hour12: false })
function observation(relation) { try { return JSON.parse(relation.observation) } catch { return {} } }
</script>

<style scoped>
.replication-panel { min-width: 0; }
.replication-toolbar, .relation-heading, .relation-actions { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); }
.replication-toolbar { margin-bottom: var(--space-20); }
.replication-toolbar h3 { margin: 0; font-size: 15px; font-weight: 600; }.replication-toolbar h3 span { margin-left: 6px; color: var(--text-muted); font-weight: 400; }
.replication-toolbar p { margin: 6px 0 0; font-size: 12px; }
.replication-relation { padding: 20px 0; border-top: 1px solid var(--border-muted); }.relation-heading { justify-content: flex-start; flex-wrap: wrap; }
.replication-path { display: grid; grid-template-columns: minmax(0,1fr) 32px minmax(0,1fr); align-items: center; gap: var(--space-20); padding: 24px 0; }
.replication-endpoint { display: grid; gap: 8px; min-width: 0; }.replication-endpoint strong { font-size: 16px; font-weight: 600; overflow-wrap: anywhere; }.endpoint-role { color: var(--text-secondary); font-size: 12px; }.replication-arrow { color: var(--text-muted); }
.replication-facts { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 20px; margin: 0; padding: 16px 0; border-block: 1px solid var(--border-muted); }
.replication-facts dt, .relation-details dt { font-size: 12px; color: var(--text-secondary); margin-bottom: 8px; }.replication-facts dd { margin: 0; font-size: 14px; }
.relation-channel { display: flex; flex-wrap: wrap; gap: 12px; padding: 16px 0; font-size: 12px; }.relation-channel code { overflow-wrap: anywhere; }
.relation-details { font-size: 12px; color: var(--text-secondary); }.relation-details summary { cursor: pointer; }.relation-details dl { display: grid; gap: 16px; }.relation-details dd { margin: 0; overflow-wrap: anywhere; }
.relation-actions { justify-content: flex-start; padding-top: 20px; }
@media (max-width: 640px) { .replication-toolbar { align-items: flex-start; flex-direction: column; }.replication-path { gap: 12px; }.replication-facts { grid-template-columns: repeat(2,minmax(0,1fr)); }.replication-endpoint strong { font-size: 14px; } }
</style>
