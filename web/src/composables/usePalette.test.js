import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  applyPalette,
  getInitialPalette,
  PALETTES,
  setPalette,
  STORAGE_KEY,
} from './usePalette.js'

afterEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('data-palette')
  vi.unstubAllGlobals()
})

describe('palette preferences', () => {
  it('restores a previously saved palette', () => {
    localStorage.setItem(STORAGE_KEY, 'orchid')

    expect(getInitialPalette()).toBe('orchid')
  })

  it('uses night when the system prefers dark and no preference is saved', () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })))

    expect(getInitialPalette()).toBe('night')
  })

  it('uses mint when the system does not prefer dark and no preference is saved', () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))

    expect(getInitialPalette()).toBe('mint')
  })

  it('immediately applies and persists a selected palette', () => {
    setPalette('blue')

    expect(document.documentElement.dataset.palette).toBe('blue')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('blue')
  })

  it('applies an allowed palette to the root element', () => {
    applyPalette('mint')

    expect(document.documentElement.dataset.palette).toBe('mint')
  })

  it('supports the screenshot-inspired Sky Veil palette', () => {
    expect(PALETTES).toContain('sky')

    setPalette('sky')

    expect(document.documentElement.dataset.palette).toBe('sky')
    expect(localStorage.getItem(STORAGE_KEY)).toBe('sky')
  })
})
