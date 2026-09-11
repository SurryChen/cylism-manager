let runtimePromise

export function loadTerminalRuntime() {
  if (!runtimePromise) {
    runtimePromise = Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/xterm/css/xterm.css'),
      import('./terminalClipboard.js'),
    ]).then(([xterm, fit, _style, clipboard]) => ({
      Terminal: xterm.Terminal,
      FitAddon: fit.FitAddon,
      installTerminalClipboard: clipboard.installTerminalClipboard,
    }))
  }
  return runtimePromise
}
