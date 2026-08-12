<template>
  <Teleport to="body">
    <div v-if="modelValue" class="chat-overlay" @click.self="$emit('update:modelValue', false)">
      <section class="chat-modal" role="dialog" aria-modal="true" aria-label="Agent 聊天">
        <header class="chat-modal-header">
          <div class="chat-modal-heading">
            <h2 class="chat-modal-title">与 {{ runtime?.name || 'Agent' }} 对话</h2>
            <p class="chat-modal-sub">{{ displayVersion(runtime) }}</p>
          </div>
          <div class="chat-header-actions"><button type="button" class="icon-button" title="Agent 权限" aria-label="Agent 权限" @click="emit('manage-permissions')"><Shield :size="17" /></button><button type="button" class="icon-button" title="待审批操作" aria-label="待审批操作" @click="approvalOpen = true; loadApprovals()"><ClipboardCheck :size="17" /><span v-if="pendingApprovals.length" class="chat-action-count">{{ pendingApprovals.length }}</span></button><button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="$emit('update:modelValue', false)"><X :size="18" /></button></div>
        </header>

        <div class="chat-sessions">
          <div class="chat-session-list">
            <button v-for="session in sessionItems" :key="session.id" type="button" class="chat-session" :class="{ 'is-active': session.id === currentSession }" @click="selectSession(session.id)">
              <span>{{ session.title || session.id }}</span>
              <small v-if="session.view?.stream.status === 'pending'">思考中</small>
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
    <div v-if="approvalOpen" class="chat-overlay chat-approval-overlay" @click.self="approvalOpen = false">
      <section class="chat-approval-modal" role="dialog" aria-modal="true" aria-label="待审批操作">
        <header class="chat-modal-header"><div><h2 class="chat-modal-title">待审批操作</h2><p class="chat-modal-sub">仅管理员可以批准或拒绝 Runtime 发起的变更</p></div><button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="approvalOpen = false"><X :size="18" /></button></header>
        <div class="chat-approval-list"><div v-if="approvalLoading" class="chat-empty">正在读取审批...</div><div v-else-if="!pendingApprovals.length" class="chat-empty">暂无待审批操作</div><article v-for="operation in pendingApprovals" :key="operation.operation_id" class="chat-approval-item"><div><strong>{{ operation.summary }}</strong><small>{{ operation.created_at || '-' }} · 15 分钟内有效</small></div><div class="chat-approval-actions"><button type="button" class="btn btn-primary" :disabled="approvalWorking" @click="resolveApproval(operation, true)">批准</button><button type="button" class="btn btn-danger" :disabled="approvalWorking" @click="resolveApproval(operation, false)">拒绝</button></div></article></div>
      </section>
    </div>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { ClipboardCheck, Shield, X } from 'lucide-vue-next'
import { agentOperations, chatMessages, chatSessions, chatStream, resolveAgentOperation } from '../api/index.js'

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
const approvalWorking = ref(false)
const pendingApprovals = ref([])
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
    const nextSessions = (await chatSessions(props.runtime.id)) || []
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
    const operations = (await agentOperations(props.runtime.id)) || []
    if (requestVersion === approvalsRequestVersion) pendingApprovals.value = operations.filter(operation => operation.status === 'pending_approval')
  } catch {
    if (requestVersion === approvalsRequestVersion) pendingApprovals.value = []
  } finally {
    if (requestVersion === approvalsRequestVersion) approvalLoading.value = false
  }
}

async function resolveApproval(operation, approve) {
  approvalWorking.value = true
  try {
    await resolveAgentOperation(operation.operation_id, approve)
    ++approvalsRequestVersion
    pendingApprovals.value = pendingApprovals.value.filter(item => item.operation_id !== operation.operation_id)
    await loadApprovals()
  } catch (err) {
    // A second browser tab may resolve the same immutable operation first.
    // It is no longer actionable in this queue even when our refresh fails.
    if (String(err?.message || '').includes('不再等待审批')) {
      ++approvalsRequestVersion
      pendingApprovals.value = pendingApprovals.value.filter(item => item.operation_id !== operation.operation_id)
      await loadApprovals()
    }
  } finally {
    approvalWorking.value = false
  }
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
  if (!currentSession.value) {
    const first = sessions.value[0]
    if (first) currentSession.value = first.id
  }
  const view = currentView.value
  if (view && !view.historyLoaded && !view.localOnly) await loadHistory(view.id)
}

async function selectSession(sessionID) {
  const view = sessionViews.value.get(sessionID) || ensureView({ id: sessionID, title: sessionID })
  currentSession.value = sessionID
  view.error = ''
  if (!view.historyLoaded) await loadHistory(sessionID)
  await scrollToBottom()
}

function newSession() {
  const id = createSessionID()
  sessionViews.value.set(id, createView({ id, title: '新会话' }, true))
  currentSession.value = id
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
.chat-modal-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 18px 22px 12px; border-bottom: 1px solid var(--border-muted); }
.chat-header-actions { display: flex; align-items: center; gap: 6px; }
.chat-header-actions .icon-button { position: relative; }
.chat-action-count { position: absolute; top: -4px; right: -4px; display: grid; min-width: 15px; height: 15px; place-items: center; border-radius: 50%; background: var(--danger); color: var(--action-contrast); font-size: 9px; }
.chat-modal-heading { min-width: 0; }
.chat-modal-title { margin: 0; color: var(--text-primary); font-size: 16px; }
.chat-modal-sub { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.chat-sessions { padding: 10px 22px 0; }
.chat-session-list { display: flex; gap: 6px; overflow-x: auto; padding-bottom: 10px; }
.chat-session { flex: 0 0 auto; max-width: 160px; overflow: hidden; padding: 6px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.chat-session:hover, .chat-session.is-active { border-color: var(--focus); background: var(--surface-hover); color: var(--action-primary); }
.chat-session-new { border-style: dashed; }
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
.chat-approval-list { padding: 10px 22px 22px; }
.chat-approval-item { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 13px 0; border-bottom: 1px solid var(--border-muted); }
.chat-approval-item > div:first-child { display: grid; min-width: 0; gap: 4px; }
.chat-approval-item strong { overflow-wrap: anywhere; font-size: 13px; }
.chat-approval-item small { color: var(--text-secondary); font-size: 11px; }
.chat-approval-actions { display: flex; flex: 0 0 auto; gap: 6px; }
@media (max-width: 640px) {
  .chat-overlay { padding: 12px; }
  .chat-modal { width: 100%; height: calc(100dvh - 24px); min-height: 0; }
  .chat-modal-header, .chat-sessions, .chat-input-bar { padding-left: 16px; padding-right: 16px; }
  .chat-messages { padding: 14px 16px; }
  .chat-approval-modal { width: 100%; max-height: calc(100dvh - 24px); }
  .chat-approval-list { padding-left: 16px; padding-right: 16px; }
  .chat-approval-item { align-items: flex-start; flex-direction: column; }
}
</style>
