// 为 xterm 终端补齐 Windows/Linux 的 Ctrl+V 粘贴与选中时 Ctrl+C 复制。
// 保留 xterm 默认的 Ctrl+Shift+C / Ctrl+Shift+V；无选中时 Ctrl+C 仍发送中断。
export function installTerminalClipboard(term) {
  term.attachCustomKeyEventHandler((event) => {
    if (event.type !== 'keydown') return true
    const key = (event.key || '').toLowerCase()
    if (event.shiftKey) return true
    if (!event.ctrlKey && !event.metaKey) return true
    if (key === 'v') {
      event.preventDefault()
      navigator.clipboard.readText()
        .then(text => term.paste(String(text || '').replace(/\r\n/g, '\n')))
        .catch(() => {})
      return false
    }
    if (key === 'c' && term.hasSelection()) {
      event.preventDefault()
      navigator.clipboard.writeText(term.getSelection()).catch(() => {})
      term.clearSelection()
      return false
    }
    return true
  })
}
