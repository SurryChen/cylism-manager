<template>
  <Teleport to="body">
    <div v-if="modelValue" class="chat-overlay" @click.self="$emit('update:modelValue', false)">
      <section class="chat-modal" role="dialog" aria-modal="true" aria-label="Agent 聊天">
        <header class="chat-modal-header">
          <div class="chat-modal-heading">
            <h2 class="chat-modal-title">与 {{ runtime?.name || 'Agent' }} 对话</h2>
            <p class="chat-modal-sub">{{ displayVersion(runtime) }}</p>
          </div>
          <div class="chat-header-actions"><button type="button" class="icon-button" title="Agent 权限" aria-label="Agent 权限" @click="emit('manage-permissions')"><Shield :size="17" /></button><button type="button" class="icon-button" title="待审批操作" aria-label="待审批操作" @click="approvalOpen = true; loadApprovals()"><ClipboardCheck :size="17" /><span v-if="pendingApprovals.length" class="chat-action-count">{{ pendingApprovals.length }}</span></button><div class="chat-header-menu"><button type="button" class="icon-button" title="当前会话操作" aria-label="当前会话操作" @click="sessionMenuOpen = !sessionMenuOpen"><MoreHorizontal :size="17" /></button><div v-if="sessionMenuOpen" class="chat-session-menu chat-header-session-menu"><button type="button" @click="toggleArchivedSessions"><EyeOff v-if="showArchived" :size="14" /><Eye v-else :size="14" />{{ showArchived ? '隐藏已归档' : '显示已归档' }}</button><template v-if="currentSessionItem"><button v-if="!currentSessionItem.localOnly" type="button" @click="openRenameSession(currentSessionItem)"><Pencil :size="14" />重命名</button><button v-if="!currentSessionItem.localOnly && !currentSessionItem.archived" type="button" :disabled="currentSessionItem.view?.stream.status !== 'idle'" @click="archiveSession(currentSessionItem, true)"><Archive :size="14" />归档</button><button v-else-if="!currentSessionItem.localOnly && currentSessionItem.archived" type="button" @click="archiveSession(currentSessionItem, false)"><ArchiveRestore :size="14" />恢复</button><button v-if="!currentSessionItem.localOnly" type="button" @click="exportSession(currentSessionItem)"><Download :size="14" />导出</button><button type="button" class="is-danger" :disabled="currentSessionItem.view?.stream.status !== 'idle'" @click="removeSession(currentSessionItem)"><Trash2 :size="14" />删除</button></template></div></div><button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="$emit('update:modelValue', false)"><X :size="18" /></button></div>
        </header>

        <div class="chat-sessions">
          <div class="chat-session-list">
            <button v-for="session in sessionItems" :key="session.id" type="button" class="chat-session" :class="{ 'is-active': session.id === currentSession, 'is-archived': session.archived }" @click="selectSession(session.id)">
              <span>{{ session.title || session.id }}</span>
              <small v-if="session.archived">已归档</small>
              <small v-else-if="session.view?.stream.status === 'pending'">思考中</small>
              <small v-else-if="session.view?.stream.status === 'streaming'">生成中</small>
              <small v-else-if="session.view?.stream.status === 'error'">失败</small>
            </button>
            <button type="button" class="chat-session chat-session-new" @click="newSession">新建会话</button>
          </div>
        </div>

        <div ref="messageList" class="chat-messages" @scroll="onScroll">
          <button v-if="currentView?.hasMoreHistory && !currentView.loadingHistory" type="button" class="chat-history-more" @click="loadOlderHistory">加载更早消息</button>
          <div v-if="currentView?.loadingHistory && !currentView.messages.length" class="chat-empty">加载会话历史...</div>
          <div v-else-if="!currentView?.messages.length" class="chat-empty">还没有消息，发送第一条开始对话</div>
          <div v-for="message in (currentView?.messages || [])" :key="message.id" class="chat-message" :class="[`is-${message.role}`, `is-${message.status}`]">
            <div class="chat-bubble">
              <span v-if="message.status === 'pending'">正在思考...</span>
              <div v-else class="chat-markdown" v-html="renderMarkdown(message.content)" />
              <span v-if="message.status === 'streaming'" class="chat-cursor" />
              <span v-if="message.status === 'stopped'" class="chat-message-state">已停止生成</span>
              <span v-if="message.status === 'error'" class="chat-message-state">{{ message.error || '生成失败' }}</span>
              <button v-if="message.status === 'error'" type="button" class="chat-retry" @click="retryMessage(message)">重试</button>
            </div>
          </div>
          <div v-if="currentView?.error" class="chat-error">{{ currentView.error }}</div>
          <div v-if="sessionsError" class="chat-error">{{ sessionsError }}</div>
        </div>

        <footer class="chat-input-bar">
          <textarea
            :value="currentView?.draft || ''"
            rows="1"
            class="chat-input"
            placeholder="输入消息，Enter 发送"
            :disabled="!currentView || currentView.loadingHistory || currentView.stream.status === 'pending' || currentView.stream.status === 'streaming'"
            @input="setDraft"
            @compositionstart="isComposing = true"
            @compositionend="isComposing = false"
            @keydown="onInputKeydown"
          />
          <button v-if="currentView?.stream.status === 'pending' || currentView?.stream.status === 'streaming'" type="button" class="btn" @click="stopCurrent">停止</button>
          <button v-else type="button" class="btn btn-primary" :disabled="!currentView?.draft?.trim() || currentView?.loadingHistory" @click="send">发送</button>
        </footer>
      </section>
    </div>
  </Teleport>
  <Teleport to="body">
    <div v-if="renameOpen" class="chat-overlay chat-session-dialog-overlay" @click.self="closeRenameSession">
      <form class="chat-session-dialog" aria-label="重命名会话" @submit.prevent="confirmRenameSession">
        <header class="chat-session-dialog-header"><h2>重命名会话</h2><button type="button" class="icon-button" title="关闭" aria-label="关闭重命名" @click="closeRenameSession"><X :size="18" /></button></header>
        <label class="chat-session-dialog-field">会话名称<input v-model="renameTitle" class="chat-input" maxlength="160" autofocus /></label>
        <div class="chat-session-dialog-actions"><button type="button" class="btn" @click="closeRenameSession">取消</button><button type="submit" class="btn btn-primary" :disabled="!renameTitle.trim() || renameSaving">{{ renameSaving ? '保存中' : '确定' }}</button></div>
      </form>
    </div>
  </Teleport>
  <Teleport to="body">
    <div v-if="approvalOpen" class="chat-overlay chat-approval-overlay" @click.self="approvalOpen = false">
      <section class="chat-approval-modal" role="dialog" aria-modal="true" aria-label="待审批操作">
        <header class="chat-modal-header"><div><h2 class="chat-modal-title">Agent 操作</h2><p class="chat-modal-sub">仅管理员可以批准或拒绝 Runtime 发起的变更</p></div><button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="approvalOpen = false"><X :size="18" /></button></header>
        <div class="chat-approval-tabs"><button type="button" :class="{ 'is-active': approvalTab === 'pending' }" @click="approvalTab = 'pending'; loadApprovals()">待审批<span v-if="pendingApprovals.length">{{ pendingApprovals.length }}</span></button><button type="button" :class="{ 'is-active': approvalTab === 'history' }" @click="approvalTab = 'history'; loadApprovalHistory()">历史</button></div>
        <div class="chat-approval-list"><div v-if="approvalLoading" class="chat-empty">正在读取操作...</div><template v-else-if="approvalTab === 'pending'"><div v-if="!pendingApprovals.length" class="chat-empty">暂无待审批操作</div><article v-for="operation in pendingApprovals" :key="operation.operation_id" class="chat-approval-item"><div><strong>{{ operation.summary }}</strong><small>{{ operation.created_at || '-' }} · {{ operation.expires_at || '15 分钟内有效' }}</small></div><div class="chat-approval-actions"><button type="button" class="btn btn-primary" :disabled="approvalWorkingIDs.has(operation.operation_id)" @click="resolveApproval(operation, true)">批准</button><button type="button" class="btn btn-danger" :disabled="approvalWorkingIDs.has(operation.operation_id)" @click="resolveApproval(operation, false)">拒绝</button></div></article></template><template v-else><div v-if="!approvalHistory.length" class="chat-empty">暂无操作历史</div><article v-for="operation in approvalHistory" :key="operation.operation_id" class="chat-approval-item chat-approval-history"><div><strong>{{ operation.summary }}</strong><small>{{ operation.created_at || '-' }}</small><button v-if="operation.error_summary" type="button" class="chat-operation-error-toggle" :aria-expanded="expandedOperationID === operation.operation_id" @click="toggleOperationError(operation.operation_id)">{{ expandedOperationID === operation.operation_id ? '收起错误详情' : '查看错误详情' }}</button><pre v-if="operation.error_summary && expandedOperationID === operation.operation_id" class="chat-operation-error-detail">{{ operation.error_summary }}</pre></div><span class="chat-operation-status" :class="`is-${operation.status}`">{{ approvalStatusLabel(operation.status) }}</span></article></template><div v-if="approvalFeedback" class="chat-error">{{ approvalFeedback }}</div></div>
      </section>
    </div>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { Archive, ArchiveRestore, ClipboardCheck, Download, Eye, EyeOff, MoreHorizontal, Pencil, Shield, Trash2, X } from 'lucide-vue-next'
import { agentOperations, archiveChatSession, chatMessages, chatSessions, chatStream, deleteChatSession, exportChatSession, renameChatSession, resolveAgentOperation } from '../api/index.js'

const props = defineProps({
  runtime: { type: Object, default: null },
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const sessions = ref([])
const sessionViews = ref(new Map())
const currentSession = ref('')
const messageList = ref(null)
const stickToBottom = ref(true)
const isComposing = ref(false)
const approvalOpen = ref(false)
const approvalLoading = ref(false)
const approvalWorkingIDs = reactive(new Set())
const pendingApprovals = ref([])
const approvalHistory = ref([])
const approvalTab = ref('pending')
const approvalFeedback = ref('')
const expandedOperationID = ref('')
const showArchived = ref(false)
const sessionMenuOpen = ref(false)
const renameOpen = ref(false)
const renameTarget = ref(null)
const renameTitle = ref('')
const renameSaving = ref(false)
const sessionsError = ref('')
let sessionsRequestVersion = 0
let approvalsRequestVersion = 0
let messageSequence = 0

const currentView = computed(() => sessionViews.value.get(currentSession.value) || null)
const sessionItems = computed(() => {
  const remoteIDs = new Set(sessions.value.map(session => session.id))
  const local = [...sessionViews.value.values()]
    .filter(view => view.localOnly && !remoteIDs.has(view.id))
    .map(view => ({ ...view, view }))
  return [...local, ...sessions.value.map(session => ({ ...session, view: sessionViews.value.get(session.id) }))]
})
const currentSessionItem = computed(() => sessionItems.value.find(session => session.id === currentSession.value) || null)

const markdown = new MarkdownIt({ breaks: true, html: false, linkify: true }).use(taskLists, { enabled: true })

function renderMarkdown(content) {
  return DOMPurify.sanitize(markdown.render(String(content || '')), { USE_PROFILES: { html: true } })
}

function tagVersion(image) {
  const match = String(image || '').match(/:([^/@]+)$/)
  return match && match[1] && match[1] !== 'latest' ? match[1] : ''
}
function displayVersion(item) {
  return item?.runtime_version || tagVersion(item?.image) || 'Agent Runtime'
}

async function scrollToBottom() {
  if (!stickToBottom.value) return
  await nextTick()
  if (messageList.value) messageList.value.scrollTop = messageList.value.scrollHeight
}

async function scrollToLatest() {
  stickToBottom.value = true
  await nextTick()
  if (messageList.value) messageList.value.scrollTop = messageList.value.scrollHeight
}

function onScroll() {
  const element = messageList.value
  if (!element) return
  stickToBottom.value = element.scrollHeight - element.scrollTop - element.clientHeight < 40
}

function createView(summary, localOnly = false) {
  return reactive({
    id: summary.id,
    title: summary.title || summary.id,
    updated_at: summary.updated_at,
    localOnly,
    draft: '',
    messages: [],
    historyLoaded: localOnly,
    loadingHistory: false,
    historyRequestVersion: 0,
    historyCursor: null,
    hasMoreHistory: false,
    error: '',
    stream: { status: 'idle', requestId: '', abort: null, assistantID: '' },
  })
}

function ensureView(summary, localOnly = false) {
  let view = sessionViews.value.get(summary.id)
  if (!view) {
    view = createView(summary, localOnly)
    sessionViews.value.set(summary.id, view)
  } else {
    view.title = summary.title || view.title || summary.id
    view.updated_at = summary.updated_at || view.updated_at
    if (!localOnly) view.localOnly = false
  }
  return view
}

async function refreshSessions() {
  if (!props.runtime?.id) return
  const requestVersion = ++sessionsRequestVersion
  try {
    sessionsError.value = ''
    const nextSessions = (await (showArchived.value ? chatSessions(props.runtime.id, { archived: true }) : chatSessions(props.runtime.id))) || []
    if (requestVersion !== sessionsRequestVersion) return
    sessions.value = nextSessions
    nextSessions.forEach(session => ensureView(session))
    const knownIDs = new Set(nextSessions.map(session => session.id))
    for (const [id, view] of sessionViews.value) {
      if (!view.localOnly && !knownIDs.has(id) && id !== currentSession.value && view.stream.status === 'idle') sessionViews.value.delete(id)
    }
  } catch (err) {
    if (requestVersion === sessionsRequestVersion) sessionsError.value = err.message || '读取会话失败'
  }
}

async function loadApprovals() {
  if (!props.runtime?.id) return
  const requestVersion = ++approvalsRequestVersion
  approvalLoading.value = true
  try {
    const operations = (await agentOperations(props.runtime.id, { status: 'pending_approval' })) || []
    if (requestVersion === approvalsRequestVersion) pendingApprovals.value = operations
  } catch (err) {
    if (requestVersion === approvalsRequestVersion) {
      pendingApprovals.value = []
      approvalFeedback.value = err.message || '读取待审批操作失败'
    }
  } finally {
    if (requestVersion === approvalsRequestVersion) approvalLoading.value = false
  }
}

async function loadApprovalHistory() {
  if (!props.runtime?.id) return
  approvalLoading.value = true
  try {
    approvalHistory.value = (await agentOperations(props.runtime.id)) || []
  } catch (err) {
    approvalHistory.value = []
    approvalFeedback.value = err.message || '读取操作历史失败'
  } finally {
    approvalLoading.value = false
  }
}

async function resolveApproval(operation, approve) {
  approvalWorkingIDs.add(operation.operation_id)
  approvalFeedback.value = ''
  try {
    const result = await resolveAgentOperation(operation.operation_id, approve)
    approvalFeedback.value = result?.status ? `操作状态：${approvalStatusLabel(result.status)}` : ''
  } catch (err) {
    approvalFeedback.value = err.message || '审批操作失败'
  } finally {
    approvalWorkingIDs.delete(operation.operation_id)
    // Resolving can atomically make the operation terminal before returning an
    // HTTP error. Always reload the authoritative queue and history.
    await Promise.all([loadApprovals(), loadApprovalHistory()])
  }
}

function approvalStatusLabel(status) {
  return ({ pending_approval: '待审批', approved: '已批准', rejected: '已拒绝', succeeded: '已完成', failed: '执行失败', stale: '已过期资源', expired: '审批过期' })[status] || status || '未知'
}

function toggleOperationError(operationID) {
  expandedOperationID.value = expandedOperationID.value === operationID ? '' : operationID
}

async function loadHistory(sessionID, { before = null } = {}) {
  const view = sessionViews.value.get(sessionID)
  if (!view || !props.runtime?.id || view.loadingHistory) return
  const requestVersion = ++view.historyRequestVersion
  view.loadingHistory = true
  view.error = ''
  try {
    const detail = before ? await chatMessages(props.runtime.id, sessionID, { limit: 50, before }) : await chatMessages(props.runtime.id, sessionID)
    if (requestVersion !== view.historyRequestVersion) return
    const messages = (detail?.messages || []).map(message => ({
      id: message.id || `history-${sessionID}-${messageSequence++}`,
      role: message.role,
      content: message.content,
      status: 'completed',
    }))
    view.messages = before ? [...messages, ...view.messages] : messages
    view.historyCursor = detail?.next_cursor || null
    view.hasMoreHistory = detail?.has_more === true
    view.historyLoaded = true
    await scrollToBottom()
  } catch (err) {
    if (requestVersion === view.historyRequestVersion) view.error = err.message || '读取会话消息失败'
  } finally {
    view.loadingHistory = false
  }
}

async function loadSessions() {
  await refreshSessions()
  const first = sessions.value[0]
  if (!first) return
  currentSession.value = first.id
  const view = sessionViews.value.get(first.id)
  if (view && !view.historyLoaded && !view.localOnly) await loadHistory(view.id)
  await scrollToLatest()
}

async function selectSession(sessionID) {
  const view = sessionViews.value.get(sessionID) || ensureView({ id: sessionID, title: sessionID })
  currentSession.value = sessionID
  sessionMenuOpen.value = false
  view.error = ''
  if (!view.historyLoaded) await loadHistory(sessionID)
  await scrollToBottom()
}

function newSession() {
  const id = createSessionID()
  sessionViews.value.set(id, createView({ id, title: '新会话' }, true))
  currentSession.value = id
  sessionMenuOpen.value = false
}

function openRenameSession(session) {
  sessionMenuOpen.value = false
  renameTarget.value = session
  renameTitle.value = session.title || session.id
  renameOpen.value = true
}

function closeRenameSession() {
  if (renameSaving.value) return
  renameOpen.value = false
  renameTarget.value = null
  renameTitle.value = ''
}

async function confirmRenameSession() {
  const session = renameTarget.value
  const title = renameTitle.value.trim()
  if (!session || !title || !props.runtime?.id) return
  renameSaving.value = true
  let saved = false
  try {
    const updated = await renameChatSession(props.runtime.id, session.id, title)
    session.title = updated?.title || title
    const view = sessionViews.value.get(session.id)
    if (view) view.title = session.title
    await refreshSessions()
    saved = true
  } catch (err) {
    sessionsError.value = err.message || '重命名会话失败'
  } finally {
    renameSaving.value = false
    if (saved) closeRenameSession()
  }
}

async function toggleArchivedSessions() {
  showArchived.value = !showArchived.value
  sessionMenuOpen.value = false
  await refreshSessions()
}

async function archiveSession(session, archived) {
  if (!props.runtime?.id || session.view?.stream.status !== 'idle') return
  sessionMenuOpen.value = false
  try {
    await archiveChatSession(props.runtime.id, session.id, archived)
    if (archived && currentSession.value === session.id) {
      const fallback = sessionItems.value.find(item => item.id !== session.id && !item.archived)
      currentSession.value = fallback?.id || ''
    }
    await refreshSessions()
  } catch (err) {
    sessionsError.value = err.message || '更新会话归档状态失败'
  }
}

async function exportSession(session) {
  if (!props.runtime?.id) return
  sessionMenuOpen.value = false
  try {
    const data = await exportChatSession(props.runtime.id, session.id)
    const blob = new Blob([JSON.stringify(data?.snapshot || data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `nanobot-session-${session.id}.json`
    link.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    sessionsError.value = err.message || '导出会话失败'
  }
}

async function removeSession(session) {
  if (session.view?.stream.status !== 'idle') return
  sessionMenuOpen.value = false
  const confirmed = globalThis.confirm?.('永久删除此会话？这只会删除当前会话记录，不会删除 Nanobot 已提炼的记忆。')
  if (!confirmed) return
  try {
    if (session.localOnly) {
      sessionViews.value.delete(session.id)
    } else if (props.runtime?.id) {
      await deleteChatSession(props.runtime.id, session.id)
      sessionViews.value.delete(session.id)
      await refreshSessions()
    }
    if (currentSession.value === session.id) {
      currentSession.value = sessionItems.value[0]?.id || ''
      if (currentSession.value) await selectSession(currentSession.value)
    }
  } catch (err) {
    sessionsError.value = err.message || '删除会话失败'
  }
}

function createSessionID() {
  if (typeof globalThis.crypto?.randomUUID === 'function') return globalThis.crypto.randomUUID()
  return `session-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

function setDraft(event) {
  if (currentView.value) currentView.value.draft = event.target.value
}

function onInputKeydown(event) {
  if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) return
  if (isComposing.value || event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  send()
}

function createMessageID(sessionID) {
  return `${sessionID}-${Date.now()}-${messageSequence++}`
}

async function send() {
  const view = currentView.value
  const text = view?.draft?.trim()
  if (!view || !text || !props.runtime?.id || view.stream.status === 'pending' || view.stream.status === 'streaming') return
  view.draft = ''
  view.error = ''
  stickToBottom.value = true
  view.messages.push({ id: createMessageID(view.id), role: 'user', content: text, status: 'completed' })
  const assistant = reactive({ id: createMessageID(view.id), role: 'assistant', content: '', status: 'pending', error: '', retryText: text })
  view.messages.push(assistant)
  await runStream(view, text, assistant)
}

async function runStream(view, text, assistant) {
  const requestID = createMessageID(view.id)
  view.stream = { status: 'pending', requestId: requestID, abort: null, assistantID: assistant.id }
  await scrollToBottom()
  const stream = chatStream(props.runtime.id, { session_id: view.id, message: text }, {
    onEvent: (event) => {
      if (view.stream.requestId !== requestID || view.stream.assistantID !== assistant.id) return
      if (event.type === 'delta') {
        assistant.status = 'streaming'
        assistant.content += event.content || ''
        scrollToBottom()
      } else if (event.type === 'done') {
        assistant.status = 'completed'
        view.localOnly = false
        view.stream.status = 'idle'
        view.stream.abort = null
        refreshSessions()
      } else if (event.type === 'error') {
        assistant.status = 'error'
        assistant.error = event.message || '生成失败'
        view.stream.status = 'idle'
        view.stream.abort = null
      }
    },
  })
  view.stream.abort = stream.abort
  try {
    await stream
  } catch (err) {
    if (view.stream.requestId !== requestID) return
    if (err?.name === 'AbortError') {
      assistant.status = 'stopped'
    } else {
      assistant.status = 'error'
      assistant.error = err.message || '请求失败'
    }
    view.stream.status = 'idle'
    view.stream.abort = null
  } finally {
    if (view.stream.requestId === requestID) view.stream.requestId = ''
  }
}

function retryMessage(message) {
  const view = currentView.value
  if (!view || message.role !== 'assistant' || message.status !== 'error' || view.stream.status !== 'idle') return
  message.content = ''
  message.error = ''
  message.status = 'pending'
  runStream(view, message.retryText, message)
}

function stopSession(sessionID) {
  const view = sessionViews.value.get(sessionID)
  if (!view || !view.stream.abort) return
  const assistant = view.messages.find(message => message.id === view.stream.assistantID)
  const requestID = view.stream.requestId
  view.stream.requestId = ''
  view.stream.status = 'idle'
  view.stream.abort()
  view.stream.abort = null
  if (assistant && assistant.id === view.stream.assistantID) assistant.status = 'stopped'
  if (requestID) view.error = ''
}

function stopCurrent() {
  stopSession(currentSession.value)
}

async function loadOlderHistory() {
  const view = currentView.value
  if (view?.historyCursor) await loadHistory(view.id, { before: view.historyCursor })
}

watch(() => props.modelValue, (open) => {
  if (open) {
    loadSessions()
    loadApprovals()
  }
}, { immediate: true })

onBeforeUnmount(() => {
  for (const view of sessionViews.value.values()) view.stream.abort?.()
})
</script>

<style scoped>
.chat-overlay { position: fixed; z-index: 1500; inset: 0; display: flex; align-items: center; justify-content: center; padding: 24px; background: var(--overlay); backdrop-filter: blur(8px); }
.chat-modal { display: flex; width: min(760px, 100%); height: min(720px, calc(100dvh - 48px)); min-height: 420px; flex-direction: column; overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow); backdrop-filter: blur(30px) saturate(145%); }
.chat-modal-header { position: relative; z-index: 3; display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 18px 22px 12px; border-bottom: 1px solid var(--border-muted); }
.chat-header-actions { display: flex; align-items: center; gap: 6px; }
.chat-header-actions .icon-button { position: relative; }
.chat-header-menu { position: relative; }
.chat-action-count { position: absolute; top: -4px; right: -4px; display: grid; min-width: 15px; height: 15px; place-items: center; border-radius: 50%; background: var(--danger); color: var(--action-contrast); font-size: 9px; }
.chat-modal-heading { min-width: 0; }
.chat-modal-title { margin: 0; color: var(--text-primary); font-size: 16px; }
.chat-modal-sub { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.chat-sessions { padding: 10px 22px 0; }
.chat-session-list { display: flex; gap: 6px; overflow-x: auto; padding-bottom: 10px; }
.chat-session { flex: 0 0 auto; max-width: 160px; overflow: hidden; padding: 6px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.chat-session:hover, .chat-session.is-active { border-color: var(--focus); background: var(--surface-hover); color: var(--action-primary); }
.chat-session.is-archived { border-style: dashed; opacity: .8; }
.chat-session-menu { position: absolute; z-index: 2; top: 34px; right: 0; display: grid; min-width: 122px; overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow); }
.chat-header-session-menu { z-index: 20; top: calc(100% + 6px); }
.chat-session-menu button { display: flex; align-items: center; gap: 7px; border: 0; padding: 8px 10px; background: transparent; color: var(--text-secondary); font: inherit; font-size: 12px; text-align: left; cursor: pointer; }
.chat-session-menu button:hover { background: var(--surface-hover); color: var(--text-primary); }
.chat-session-menu button:disabled { opacity: .45; cursor: not-allowed; }
.chat-session-menu .is-danger { color: var(--danger); }
.chat-session-new { border-style: dashed; }
.chat-session-dialog-overlay { z-index: 1700; }
.chat-session-dialog { width: min(420px, 100%); border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow); backdrop-filter: blur(30px) saturate(145%); }
.chat-session-dialog-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 16px 18px 12px; border-bottom: 1px solid var(--border-muted); }
.chat-session-dialog-header h2 { margin: 0; color: var(--text-primary); font-size: 15px; }
.chat-session-dialog-field { display: grid; gap: 7px; padding: 16px 18px; color: var(--text-secondary); font-size: 12px; }
.chat-session-dialog-field .chat-input { width: 100%; box-sizing: border-box; }
.chat-session-dialog-actions { display: flex; justify-content: flex-end; gap: 8px; padding: 0 18px 18px; }
.chat-messages { flex: 1; min-height: 0; overflow-y: auto; padding: 18px 22px; }
.chat-empty { padding: 40px 0; color: var(--text-muted); font-size: 12px; text-align: center; }
.chat-history-more { display: block; margin: 0 auto 14px; border: 0; background: transparent; color: var(--action-primary); font: inherit; font-size: 12px; cursor: pointer; }
.chat-message { display: flex; margin-bottom: 12px; }
.chat-message.is-user { justify-content: flex-end; }
.chat-message.is-assistant { justify-content: flex-start; }
.chat-bubble { max-width: 86%; padding: 9px 12px; border-radius: 14px; font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
.chat-message.is-user .chat-bubble { border-bottom-right-radius: 4px; background: var(--action-primary); color: var(--action-contrast); }
.chat-message.is-assistant .chat-bubble { border-bottom-left-radius: 4px; background: var(--surface-raised); border: 1px solid var(--border-muted); color: var(--text-primary); }
.chat-markdown { overflow-wrap: anywhere; }
.chat-markdown:empty::before { content: '\200b'; }
.chat-markdown :deep(p) { margin: 0 0 8px; }
.chat-markdown :deep(p:last-child) { margin-bottom: 0; }
.chat-markdown :deep(h1), .chat-markdown :deep(h2), .chat-markdown :deep(h3) { margin: 12px 0 6px; color: var(--text-primary); font-size: 1em; }
.chat-markdown :deep(ul), .chat-markdown :deep(ol) { margin: 6px 0; padding-left: 20px; }
.chat-markdown :deep(li + li) { margin-top: 2px; }
.chat-markdown :deep(a) { color: var(--link); text-decoration: underline; }
.chat-message.is-user .chat-markdown :deep(a), .chat-message.is-user .chat-markdown :deep(h1), .chat-message.is-user .chat-markdown :deep(h2), .chat-message.is-user .chat-markdown :deep(h3) { color: var(--action-contrast); }
.chat-markdown :deep(code) { padding: 1px 4px; border-radius: 3px; background: var(--surface-subtle); font-family: var(--font-mono); font-size: .9em; }
.chat-markdown :deep(pre) { overflow-x: auto; margin: 8px 0; padding: 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); }
.chat-markdown :deep(pre code) { padding: 0; background: transparent; white-space: pre; }
.chat-markdown :deep(blockquote) { margin: 8px 0; padding-left: 10px; border-left: 2px solid var(--border); color: var(--text-secondary); }
.chat-markdown :deep(table) { width: 100%; margin: 8px 0; border-collapse: collapse; font-size: 12px; }
.chat-markdown :deep(th), .chat-markdown :deep(td) { padding: 4px 6px; border: 1px solid var(--border-muted); text-align: left; }
.chat-cursor { display: inline-block; width: 2px; height: 1em; margin-left: 2px; vertical-align: -0.15em; background: var(--action-primary); animation: chat-blink 1s steps(2, start) infinite; }
.chat-message-state { display: block; margin-top: 4px; color: var(--text-muted); font-size: 11px; }
.chat-retry { margin-top: 8px; border: 0; border-bottom: 1px solid currentColor; padding: 0; background: transparent; color: var(--action-primary); font: inherit; font-size: 12px; cursor: pointer; }
@keyframes chat-blink { to { visibility: hidden; } }
.chat-error { margin-top: 8px; padding: 9px 11px; border-radius: var(--radius-control); background: var(--danger-surface); color: var(--danger); font-size: 12px; }
.chat-input-bar { display: flex; align-items: flex-end; gap: 8px; padding: 12px 22px 18px; border-top: 1px solid var(--border-muted); }
.chat-input { flex: 1; resize: none; max-height: 120px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 9px 10px; background: var(--surface-input); color: var(--text-primary); font: inherit; font-size: 13px; }
.chat-input:focus { outline: 2px solid var(--focus); outline-offset: -1px; }
.chat-input:disabled { opacity: .6; cursor: not-allowed; }
.chat-approval-overlay { z-index: 1600; }
.chat-approval-modal { width: min(560px, 100%); max-height: min(620px, calc(100dvh - 48px)); overflow: auto; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow); backdrop-filter: blur(30px) saturate(145%); }
.chat-approval-tabs { display: flex; gap: 4px; padding: 10px 22px 0; border-bottom: 1px solid var(--border-muted); }
.chat-approval-tabs button { display: flex; align-items: center; gap: 5px; border: 0; border-bottom: 2px solid transparent; padding: 7px 9px; background: transparent; color: var(--text-muted); font: inherit; font-size: 12px; cursor: pointer; }
.chat-approval-tabs button.is-active { border-bottom-color: var(--action-primary); color: var(--action-primary); }
.chat-approval-tabs span { display: grid; min-width: 16px; height: 16px; place-items: center; border-radius: 8px; background: var(--danger-surface); color: var(--danger); font-size: 10px; }
.chat-approval-list { padding: 10px 22px 22px; }
.chat-approval-item { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 13px 0; border-bottom: 1px solid var(--border-muted); }
.chat-approval-item > div:first-child { display: grid; min-width: 0; gap: 4px; }
.chat-approval-item strong { overflow-wrap: anywhere; font-size: 13px; }
.chat-approval-item small { color: var(--text-secondary); font-size: 11px; }
.chat-approval-actions { display: flex; flex: 0 0 auto; gap: 6px; }
.chat-approval-history { align-items: flex-start; }
.chat-operation-error-toggle { justify-self: start; border: 0; padding: 0; background: transparent; color: var(--action-primary); font: inherit; font-size: 11px; cursor: pointer; }
.chat-operation-error-toggle:hover { text-decoration: underline; }
.chat-operation-error-detail { width: 100%; max-height: 180px; box-sizing: border-box; overflow: auto; margin: 2px 0 0; padding: 8px; border: 1px solid var(--danger); border-radius: var(--radius-control); background: var(--danger-surface); color: var(--text-primary); font-family: var(--font-mono); font-size: 11px; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
.chat-operation-status { flex: 0 0 auto; border: 1px solid var(--border-muted); border-radius: 999px; padding: 3px 7px; color: var(--text-secondary); font-size: 11px; }
.chat-operation-status.is-succeeded { border-color: var(--success); color: var(--success); }
.chat-operation-status.is-failed, .chat-operation-status.is-stale, .chat-operation-status.is-expired, .chat-operation-status.is-rejected { border-color: var(--danger); color: var(--danger); }
@media (max-width: 640px) {
  .chat-overlay { padding: 12px; }
  .chat-modal { width: 100%; height: calc(100dvh - 24px); min-height: 0; }
  .chat-modal-header, .chat-sessions, .chat-input-bar { padding-left: 16px; padding-right: 16px; }
  .chat-messages { padding: 14px 16px; }
  .chat-approval-modal { width: 100%; max-height: calc(100dvh - 24px); }
  .chat-approval-list { padding-left: 16px; padding-right: 16px; }
  .chat-approval-tabs { padding-left: 16px; padding-right: 16px; }
  .chat-approval-item { align-items: flex-start; flex-direction: column; }
}
</style>
