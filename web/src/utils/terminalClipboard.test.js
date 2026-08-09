import { beforeEach, describe, expect, it, vi } from 'vitest'
import { installTerminalClipboard } from './terminalClipboard'

function makeTerm() {
  let handler = () => true
  const term = {
    attachCustomKeyEventHandler(fn) { handler = fn },
    paste: vi.fn(),
    hasSelection: vi.fn(() => false),
    getSelection: vi.fn(() => 'selected-text'),
    clearSelection: vi.fn(),
  }
  return { term, handler: () => handler }
}

function keydown(overrides = {}) {
  return { type: 'keydown', key: 'v', ctrlKey: true, metaKey: false, shiftKey: false, preventDefault: vi.fn(), ...overrides }
}

describe('installTerminalClipboard', () => {
  beforeEach(() => {
    vi.stubGlobal('navigator', {
      clipboard: {
        readText: vi.fn().mockResolvedValue('hello\r\nworld'),
        writeText: vi.fn().mockResolvedValue(undefined),
      },
    })
  })

  it('pastes clipboard text with CRLF normalized on Ctrl+V', async () => {
    const { term, handler } = makeTerm()
    installTerminalClipboard(term)
    const event = keydown()
    const keep = handler()(event)
    expect(keep).toBe(false)
    expect(event.preventDefault).toHaveBeenCalled()
    await vi.waitFor(() => expect(term.paste).toHaveBeenCalledWith('hello\nworld'))
  })

  it('copies selection on Ctrl+C when text is selected', async () => {
    const { term, handler } = makeTerm()
    term.hasSelection.mockReturnValue(true)
    installTerminalClipboard(term)
    const event = keydown({ key: 'c' })
    const keep = handler()(event)
    expect(keep).toBe(false)
    expect(event.preventDefault).toHaveBeenCalled()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('selected-text')
    expect(term.clearSelection).toHaveBeenCalled()
  })

  it('lets Ctrl+C pass through when nothing is selected', () => {
    const { handler } = makeTerm()
    installTerminalClipboard({ attachCustomKeyEventHandler: handler() })
    const event = keydown({ key: 'c' })
    expect(handler()(event)).toBe(true)
    expect(event.preventDefault).not.toHaveBeenCalled()
  })

  it('keeps Ctrl+Shift+V untouched for xterm defaults', () => {
    const { handler } = makeTerm()
    installTerminalClipboard({ attachCustomKeyEventHandler: handler() })
    const event = keydown({ shiftKey: true })
    expect(handler()(event)).toBe(true)
  })
})
