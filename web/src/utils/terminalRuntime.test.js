import { afterEach, describe, expect, it, vi } from 'vitest'
import { loadTerminalRuntime } from './terminalRuntime.js'

vi.mock('@xterm/xterm', () => ({ Terminal: class Terminal {} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class FitAddon {} }))
vi.mock('@xterm/xterm/css/xterm.css', () => ({}))
vi.mock('./terminalClipboard.js', () => ({ installTerminalClipboard: vi.fn() }))

describe('terminal runtime', () => {
  afterEach(() => vi.clearAllMocks())

  it('loads and caches optional terminal dependencies', async () => {
    const first = await loadTerminalRuntime()
    const second = await loadTerminalRuntime()

    expect(first.Terminal).toBeTypeOf('function')
    expect(first.FitAddon).toBeTypeOf('function')
    expect(first.installTerminalClipboard).toBeTypeOf('function')
    expect(second).toBe(first)
  })
})
