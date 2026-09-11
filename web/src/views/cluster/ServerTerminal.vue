<template>
  <Teleport to="body">
    <div class="overlay terminal-overlay">
      <div class="terminal-modal">
        <div class="terminal-modal-header">
          <span>💻 SSH 终端 — {{ server.name }} ({{ server.host }})</span>
          <button class="btn btn-sm btn-icon" @click="close" title="关闭">✕</button>
        </div>
        <div class="terminal-body">
          <div v-if="status === 'connecting'" class="terminal-placeholder">⏳ 正在连接...</div>
          <div v-else-if="status === 'error'" class="terminal-placeholder terminal-error">
            ❌ {{ error }}
            <button class="btn btn-sm btn-primary" style="margin-left:8px" @click="connect">重试</button>
          </div>
          <div v-else-if="status === 'closed'" class="terminal-placeholder">
            🔌 连接已断开
            <button class="btn btn-sm btn-primary" style="margin-left:8px" @click="connect">重连</button>
          </div>
          <div ref="terminalEl" class="terminal-container" v-show="status === 'connected'"></div>
        </div>
        <div class="terminal-modal-footer">
          <span v-if="status === 'connected'" class="terminal-status-ok">🟢 已连接</span>
          <span v-else-if="status === 'connecting'" class="terminal-status-connecting">🟡 连接中</span>
          <span v-else-if="status === 'error'" class="terminal-status-error">🔴 连接失败</span>
          <span v-else class="terminal-status-closed">⚫ 已断开</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { loadTerminalRuntime } from '../../utils/terminalRuntime.js'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close'])
const terminalEl = ref(null)
const status = ref('connecting')
const error = ref('')
let termInstance = null
let termWs = null

onMounted(() => {
  document.body.style.overflow = 'hidden'
  nextTick(connect)
})

onBeforeUnmount(() => {
  dispose()
  document.body.style.overflow = ''
})

function connect() {
  dispose()
  status.value = 'connecting'
  error.value = ''
  nextTick(openTerminal)
}

async function openTerminal() {
  const el = terminalEl.value
  if (!el) {
    status.value = 'error'
    error.value = '终端容器未就绪'
    return
  }

  let runtime
  try {
    runtime = await loadTerminalRuntime()
  } catch (loadError) {
    status.value = 'error'
    error.value = loadError?.message || '终端组件加载失败'
    return
  }

  const rootStyle = getComputedStyle(document.documentElement)
  const term = new runtime.Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    fontSize: 14,
    fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", monospace',
    letterSpacing: 0,
    lineHeight: 1.2,
    theme: {
      background: rootStyle.getPropertyValue('--terminal-background').trim(),
      foreground: rootStyle.getPropertyValue('--terminal-foreground').trim(),
      cursor: rootStyle.getPropertyValue('--terminal-cursor').trim(),
      selectionBackground: rootStyle.getPropertyValue('--terminal-selection').trim(),
    },
  })
  const fitAddon = new runtime.FitAddon()
  term.loadAddon(fitAddon)
  term.open(el)
  runtime.installTerminalClipboard(term)

  const xtermScreen = el.querySelector('.xterm-screen')
  if (xtermScreen) xtermScreen.style.borderRadius = '8px'
  fitAddon.fit()

  const id = props.server.id
  const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/servers/${id}/terminal`
  const token = localStorage.getItem('access_token')
  const ws = new WebSocket(wsUrl + '?token=' + encodeURIComponent(token || ''))
  ws.binaryType = 'arraybuffer'
  ws.onopen = () => {
    status.value = 'connected'
    term.onData(data => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data)
    })
  }
  ws.onmessage = event => {
    if (event.data instanceof ArrayBuffer) term.write(new Uint8Array(event.data))
  }
  ws.onclose = () => { status.value = 'closed' }
  ws.onerror = () => {
    status.value = 'error'
    error.value = 'WebSocket 连接失败'
  }

  termInstance = term
  termWs = ws
  const sendResize = () => {
    try {
      fitAddon.fit()
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    } catch (_) {}
  }
  const observer = new ResizeObserver(sendResize)
  observer.observe(el)
  term._resizeObserver = observer
}

function dispose() {
  if (termInstance?._resizeObserver) termInstance._resizeObserver.disconnect()
  if (termWs) termWs.close()
  if (termInstance) termInstance.dispose()
  termInstance = null
  termWs = null
}

function close() {
  dispose()
  document.body.style.overflow = ''
  emit('close')
}
</script>

<style scoped>
.terminal-overlay { display: flex; align-items: center; justify-content: center; z-index: 9999; backdrop-filter: blur(6px); }
.terminal-modal { width: 85vw; max-width: 1100px; height: 82vh; display: flex; flex-direction: column; border: 1px solid var(--border); border-radius: 12px; background: var(--surface-raised); box-shadow: var(--shadow); overflow: hidden; }
.terminal-modal-header { display: flex; justify-content: space-between; align-items: center; padding: 10px 16px; border-bottom: 1px solid var(--border-muted); font-size: 13px; font-weight: 600; color: var(--text-primary); flex-shrink: 0; }
.btn-icon { width: 28px; height: 28px; padding: 0; border: none; border-radius: 6px; background: transparent; color: var(--text-secondary); font-size: 16px; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; }
.btn-icon:hover { background: var(--surface-hover); color: var(--text-primary); }
.terminal-body { flex: 1; padding: 12px; overflow: hidden; position: relative; min-height: 0; }
.terminal-placeholder { display: flex; align-items: center; justify-content: center; height: 100%; color: var(--text-secondary); font-size: 14px; gap: 8px; }
.terminal-error { color: var(--danger); }
.terminal-container { width: 100%; height: 100%; overflow: hidden; background: var(--terminal-background); }
.terminal-container :deep(.xterm) { height: 100%; border-radius: 8px; }
.terminal-container :deep(.xterm-viewport) { scrollbar-width: thin; scrollbar-color: var(--text-muted) transparent; }
.terminal-modal-footer { display: flex; align-items: center; padding: 6px 16px; border-top: 1px solid var(--border-muted); font-size: 11px; flex-shrink: 0; }
.terminal-status-ok { color: var(--success); }
.terminal-status-connecting { color: var(--warning); }
.terminal-status-error { color: var(--danger); }
.terminal-status-closed { color: var(--text-muted); }
</style>
