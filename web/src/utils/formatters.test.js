import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatBytes, formatClockTime, formatDateTime, formatShortDate, formatShortDateTime } from './formatters.js'

describe('formatters', () => {
  afterEach(() => vi.restoreAllMocks())

  it('formats date-time values with zh-CN locale and 24-hour time by default', () => {
    const spy = vi.spyOn(Date.prototype, 'toLocaleString').mockReturnValue('formatted-date-time')

    expect(formatDateTime('2026-09-08T07:16:22Z')).toBe('formatted-date-time')
    expect(spy).toHaveBeenCalledWith('zh-CN', { hour12: false })
  })

  it('formats short date and short date-time values for compact tables', () => {
    const dateSpy = vi.spyOn(Date.prototype, 'toLocaleDateString').mockReturnValue('formatted-date')
    const dateTimeSpy = vi.spyOn(Date.prototype, 'toLocaleString').mockReturnValue('formatted-short-date-time')

    expect(formatShortDate('2026-09-08T07:16:22Z')).toBe('formatted-date')
    expect(formatShortDateTime('2026-09-08T07:16:22Z')).toBe('formatted-short-date-time')
    expect(dateSpy).toHaveBeenCalledWith('zh-CN', { month: 'short', day: 'numeric', year: 'numeric' })
    expect(dateTimeSpy).toHaveBeenCalledWith('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  })

  it('formats clock time with seconds', () => {
    const spy = vi.spyOn(Date.prototype, 'toLocaleTimeString').mockReturnValue('formatted-clock-time')

    expect(formatClockTime('2026-09-08T07:16:22Z')).toBe('formatted-clock-time')
    expect(spy).toHaveBeenCalledWith('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  })

  it('returns placeholder for empty and invalid dates', () => {
    expect(formatDateTime('')).toBe('-')
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime('not-a-date')).toBe('-')
    expect(formatShortDate(undefined)).toBe('-')
    expect(formatShortDateTime('invalid')).toBe('-')
    expect(formatClockTime(Number.NaN)).toBe('-')
  })

  it('formats byte values with binary units', () => {
    expect(formatBytes()).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KiB')
    expect(formatBytes(10 * 1024)).toBe('10 KiB')
    expect(formatBytes(1536 * 1024)).toBe('1.5 MiB')
    expect(formatBytes(2 * 1024 * 1024 * 1024)).toBe('2.0 GiB')
  })
})
