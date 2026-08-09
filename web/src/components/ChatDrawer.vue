<template>
  <Teleport to="body">
    <div v-if="modelValue" class="chat-overlay" @click.self="$emit('update:modelValue', false)">
      <aside class="chat-drawer" role="dialog" aria-modal="true" aria-label="Agent 聊天">
        <header class="chat-drawer-header">
          <div class="chat-drawer-heading">
            <h2 class="chat-drawer-title">与 {{ runtime?.name || 'Agent' }} 对话</h2>
            <p class="chat-drawer-sub">{{ displayVersion(runtime) }}</p>
          </div>
          <button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="$emit('update:modelValue', false)"><X :size="18" /></button>
        </header>

        <div class="chat-sessions">
          <div class="chat-session-list">
            <button v-for="session in sessions" :key="session.id" type="button" class="chat-session" :class="{ 'is-active': session.id === currentSession }" @click="selectSession(session.id)">
              {{ session.title || session.id }}
            </button>
            <button type="button" class="chat-session chat-session-new" :class="{ 'is-active': !currentSession }" @click="newSession">新建会话</button>
          </div>
        </div>

        <div ref="messageList" class="chat-messages" @scroll="onScroll">
          <div v-if="loadingHistory" class="chat-empty">加载会话历史...</div>
          <div v-else-if="!messages.length" class="chat-empty">还没有消息，发送第一条开始对话</div>
          <div v-for="(message, index) in messages" :key="index" class="chat-message" :class="`is-${message.role}`">
            <div class="chat-bubble">{{ message.content }}<span v-if="streaming && index === messages.length - 1 && message.role === 'assistant'" class="chat-cursor" /></div>
          </div>
          <div v-if="error" class="chat-error">{{ error }}</div>
        </div>

        <footer class="chat-input-bar">
          <textarea v-model="input" rows="1" class="chat-input" placeholder="输入消息，Enter 发送" :disabled="streaming || loadingHistory" @keydown.enter.exact.prevent="send" />
          <button v-if="streaming" type="button" class="btn" @click="stop">停止</button>
          <button v-else type="button" class="btn btn-primary" :disabled="!input.trim() || loadingHistory" @click="send">发送</button>
        </footer>
      </aside>
    </div>
  </Teleport>
</template>

<script setup>
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'
import { chatMessages, chatSessions, chatStream } from '../api/index.js'

const props = defineProps({
  runtime: { type: Object, default: null },
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const sessions = ref([])
const currentSession = ref('')
const messages = ref([])
const input = ref('')
const streaming = ref(false)
const loadingHistory = ref(false)
const error = ref('')
const messageList = ref(null)
const stickToBottom = ref(true)
let activeStream = null

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

async function loadSessions() {
  if (!props.runtime?.id) return
  error.value = ''
  try {
    sessions.value = (await chatSessions(props.runtime.id)) || []
    if (sessions.value.length) {
      await selectSession(sessions.value[0].id)
    } else {
      currentSession.value = ''
      messages.value = []
    }
  } catch (err) {
    error.value = err.message || '读取会话失败'
  }
}

async function selectSession(sessionID) {
  currentSession.value = sessionID
  if (!props.runtime?.id) return
  loadingHistory.value = true
  error.value = ''
  try {
    const detail = await chatMessages(props.runtime.id, sessionID)
    messages.value = (detail?.messages || []).map(message => ({ role: message.role, content: message.content }))
    await scrollToBottom()
  } catch (err) {
    error.value = err.message || '读取会话消息失败'
  } finally {
    loadingHistory.value = false
  }
}

function newSession() {
  currentSession.value = ''
  messages.value = []
  error.value = ''
}

async function send() {
  const text = input.value.trim()
  if (!text || streaming.value || !props.runtime?.id) return
  input.value = ''
  error.value = ''
  stickToBottom.value = true
  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '' })
  streaming.value = true
  await scrollToBottom()
  activeStream = chatStream(props.runtime.id, { session_id: currentSession.value || undefined, message: text }, {
    onEvent: (event) => {
      if (event.type === 'delta') {
        const last = messages.value[messages.value.length - 1]
        if (last && last.role === 'assistant') last.content += event.content
        scrollToBottom()
      } else if (event.type === 'done') {
        streaming.value = false
        loadSessions()
      } else if (event.type === 'error') {
        error.value = event.message || '生成失败'
        streaming.value = false
      }
    },
  })
  try {
    await activeStream
  } catch (err) {
    if (err?.name !== 'AbortError') error.value = err.message || '请求失败'
    streaming.value = false
  } finally {
    activeStream = null
  }
}

function stop() {
  if (activeStream) activeStream.abort()
  streaming.value = false
}

watch(() => props.modelValue, (open) => {
  if (open) loadSessions()
}, { immediate: true })

onBeforeUnmount(() => {
  if (activeStream) activeStream.abort()
})
</script>

<style scoped>
.chat-overlay { position: fixed; z-index: 1500; inset: 0; display: flex; justify-content: flex-end; background: var(--overlay); backdrop-filter: blur(8px); }
.chat-drawer { display: flex; width: min(440px, 100vw); height: 100%; flex-direction: column; border-left: 1px solid var(--border); background: var(--surface-glass); box-shadow: var(--shadow); backdrop-filter: blur(30px) saturate(145%); }
.chat-drawer-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 18px 18px 12px; border-bottom: 1px solid var(--border-muted); }
.chat-drawer-heading { min-width: 0; }
.chat-drawer-title { margin: 0; color: var(--text-primary); font-size: 16px; }
.chat-drawer-sub { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.chat-sessions { padding: 10px 18px 0; }
.chat-session-list { display: flex; gap: 6px; overflow-x: auto; padding-bottom: 10px; }
.chat-session { flex: 0 0 auto; max-width: 160px; overflow: hidden; padding: 6px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.chat-session:hover, .chat-session.is-active { border-color: var(--focus); background: var(--surface-hover); color: var(--action-primary); }
.chat-session-new { border-style: dashed; }
.chat-messages { flex: 1; overflow-y: auto; padding: 14px 18px; }
.chat-empty { padding: 40px 0; color: var(--text-muted); font-size: 12px; text-align: center; }
.chat-message { display: flex; margin-bottom: 12px; }
.chat-message.is-user { justify-content: flex-end; }
.chat-message.is-assistant { justify-content: flex-start; }
.chat-bubble { max-width: 86%; padding: 9px 12px; border-radius: 14px; font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; white-space: pre-wrap; }
.chat-message.is-user .chat-bubble { border-bottom-right-radius: 4px; background: var(--action-primary); color: var(--action-contrast); }
.chat-message.is-assistant .chat-bubble { border-bottom-left-radius: 4px; background: var(--surface-raised); border: 1px solid var(--border-muted); color: var(--text-primary); }
.chat-cursor { display: inline-block; width: 2px; height: 1em; margin-left: 2px; vertical-align: -0.15em; background: var(--action-primary); animation: chat-blink 1s steps(2, start) infinite; }
@keyframes chat-blink { to { visibility: hidden; } }
.chat-error { margin-top: 8px; padding: 9px 11px; border-radius: var(--radius-control); background: var(--danger-surface); color: var(--danger); font-size: 12px; }
.chat-input-bar { display: flex; align-items: flex-end; gap: 8px; padding: 12px 18px 18px; border-top: 1px solid var(--border-muted); }
.chat-input { flex: 1; resize: none; max-height: 120px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 9px 10px; background: var(--surface-input); color: var(--text-primary); font: inherit; font-size: 13px; }
.chat-input:focus { outline: 2px solid var(--focus); outline-offset: -1px; }
.chat-input:disabled { opacity: .6; cursor: not-allowed; }
@media (max-width: 640px) { .chat-drawer { width: 100vw; } }
</style>
