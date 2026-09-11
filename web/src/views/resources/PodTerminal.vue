<template>
  <Teleport to="body">
    <div class="overlay pod-terminal-overlay">
      <div class="pod-terminal-modal" role="dialog" aria-modal="true" aria-label="容器终端">
        <header class="pod-terminal-header">
          <div><h2>容器终端</h2><p>{{ pod.namespace }} / {{ pod.name }}</p></div>
          <button class="icon-button" title="关闭终端" aria-label="关闭终端" @click="close"><X :size="17" /></button>
        </header>

        <div v-if="status === 'selecting'" class="container-picker">
          <p>该 Pod 包含多个容器，请选择要进入的容器。</p>
          <select v-model="container" class="form-select" aria-label="选择容器"><option value="" disabled>选择容器</option><option v-for="item in containers" :key="item" :value="item">{{ item }}</option></select>
          <div class="modal-actions"><button class="btn" @click="close">取消</button><button class="btn btn-primary" :disabled="!container" @click="connect">进入终端</button></div>
        </div>

        <div v-else class="pod-terminal-body">
          <div v-if="status === 'connecting'" class="terminal-placeholder">正在连接容器终端...</div>
          <div v-else-if="status === 'error'" class="terminal-placeholder terminal-error"><span>{{ error }}</span><button class="btn btn-sm btn-primary" @click="connect">重试</button></div>
          <div v-else-if="status === 'closed'" class="terminal-placeholder"><span>终端会话已断开</span><button class="btn btn-sm btn-primary" @click="connect">重新连接</button></div>
          <div ref="terminalEl" class="terminal-container" v-show="status === 'connected'"></div>
        </div>

        <footer v-if="status !== 'selecting'" class="pod-terminal-footer"><span>{{ container }}</span><span :class="['terminal-state', `terminal-state-${status}`]">{{ statusLabel }}</span></footer>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { useBodyScrollLock } from '../../composables/useBodyScrollLock.js'
import { loadTerminalRuntime } from '../../utils/terminalRuntime.js'

const props = defineProps({
  pod: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const terminalEl = ref(null)
const containers = computed(() => props.pod.containers || [])
const container = ref(containers.value.length === 1 ? containers.value[0] : '')
const status = ref(containers.value.length === 1 ? 'connecting' : 'selecting')
const error = ref('')
const statusLabel = computed(() => ({ connecting: '连接中', connected: '已连接', error: '连接失败', closed: '已断开' }[status.value] || ''))

let terminal = null
let websocket = null
let resizeObserver = null
const bodyScrollLock = useBodyScrollLock(true)

onMounted(() => {
  if (status.value === 'connecting') connect()
})

onBeforeUnmount(() => {
  disposeTerminal()
})

function connect() {
  if (!container.value) return
  disposeTerminal()
  status.value = 'connecting'
  error.value = ''
  nextTick(openTerminal)
}

async function openTerminal() {
  const element = terminalEl.value
  if (!element) {
    status.value = 'error'
    error.value = '终端容器未就绪'
    return
  }

  let terminalRuntime
  try {
    terminalRuntime = await loadTerminalRuntime()
  } catch (loadError) {
    status.value = 'error'
    error.value = loadError?.message || '终端组件加载失败'
    return
  }
  const { Terminal, FitAddon, installTerminalClipboard } = terminalRuntime

  const rootStyle = getComputedStyle(document.documentElement)
  const term = new Terminal({
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
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(element)
  installTerminalClipboard(term)
  fitAddon.fit()

  const token = localStorage.getItem('access_token') || ''
  const namespace = encodeURIComponent(props.pod.namespace)
  const name = encodeURIComponent(props.pod.name)
  const selectedContainer = encodeURIComponent(container.value)
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const ws = new WebSocket(`${protocol}//${location.host}/api/k8s/pods/${namespace}/${name}/terminal?container=${selectedContainer}&token=${encodeURIComponent(token)}`)
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    status.value = 'connected'
    term.onData(data => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data)
    })
  }
  ws.onmessage = event => {
    if (event.data instanceof ArrayBuffer) {
      term.write(new Uint8Array(event.data))
      return
    }
    try {
      const message = JSON.parse(event.data)
      if (message.type === 'error') {
        status.value = 'error'
        error.value = message.detail || '容器终端连接失败'
        ws.close()
      }
    } catch (_) {}
  }
  ws.onclose = () => {
    if (status.value !== 'error') status.value = 'closed'
  }
  ws.onerror = () => {
    status.value = 'error'
    error.value = 'WebSocket 连接失败'
  }

  const sendResize = () => {
    try {
      fitAddon.fit()
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    } catch (_) {}
  }
  resizeObserver = new ResizeObserver(sendResize)
  resizeObserver.observe(element)
  terminal = term
  websocket = ws
}

function disposeTerminal() {
  if (resizeObserver) resizeObserver.disconnect()
  if (websocket) websocket.close()
  if (terminal) terminal.dispose()
  resizeObserver = null
  websocket = null
  terminal = null
}

function close() {
  disposeTerminal()
  bodyScrollLock.unlock()
  emit('close')
}
</script>

<style scoped>
.pod-terminal-overlay { z-index: 1300; }
.pod-terminal-modal { display: grid; width: min(980px, calc(100vw - 32px)); height: min(680px, calc(100dvh - 32px)); grid-template-rows: auto minmax(0, 1fr) auto; overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-raised); box-shadow: var(--shadow); }
.pod-terminal-header, .pod-terminal-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 16px; border-bottom: 1px solid var(--border-muted); }
.pod-terminal-header h2 { margin: 0; color: var(--text-primary); font-size: 15px; }
.pod-terminal-header p { margin: 4px 0 0; color: var(--text-muted); font: 11px/1.2 var(--font-mono); }
.pod-terminal-footer { min-height: 42px; border-top: 1px solid var(--border-muted); border-bottom: 0; color: var(--text-muted); font: 11px/1 var(--font-mono); }
.terminal-state-connected { color: var(--success); }
.terminal-state-connecting { color: var(--warning); }
.terminal-state-error { color: var(--danger); }
.terminal-state-closed { color: var(--text-muted); }
.pod-terminal-body { position: relative; min-height: 0; }
.terminal-container { position: absolute; inset: 0; padding: 12px; background: var(--terminal-background); }
.terminal-container :deep(.xterm), .terminal-container :deep(.xterm-viewport) { height: 100%; }
.terminal-container :deep(.xterm-viewport) { overflow-y: auto; }
.terminal-placeholder { display: flex; height: 100%; align-items: center; justify-content: center; gap: 12px; padding: 24px; color: var(--text-secondary); font-size: 13px; }
.terminal-error { color: var(--danger); }
.container-picker { display: grid; align-content: center; gap: 16px; max-width: 420px; width: 100%; margin: auto; padding: 32px; }
.container-picker p { margin: 0; color: var(--text-secondary); font-size: 13px; line-height: 1.6; }
@media (max-width:640px) { .pod-terminal-modal { width: 100vw; height: 100dvh; border: 0; border-radius: 0; }.pod-terminal-header, .pod-terminal-footer { padding-right: 12px; padding-left: 12px; } }
</style>
