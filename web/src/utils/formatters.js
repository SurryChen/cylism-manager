const DEFAULT_LOCALE = 'zh-CN'
const DEFAULT_DATE_TIME_OPTIONS = { hour12: false }
const SHORT_DATE_OPTIONS = { month: 'short', day: 'numeric', year: 'numeric' }
const SHORT_DATE_TIME_OPTIONS = { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
const CLOCK_TIME_OPTIONS = { hour: '2-digit', minute: '2-digit', second: '2-digit' }
const BYTE_UNITS = ['KiB', 'MiB', 'GiB', 'TiB']

function parseDate(value) {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatDateTime(value, options = DEFAULT_DATE_TIME_OPTIONS) {
  const date = parseDate(value)
  return date ? date.toLocaleString(DEFAULT_LOCALE, options) : '-'
}

export function formatShortDate(value) {
  const date = parseDate(value)
  return date ? date.toLocaleDateString(DEFAULT_LOCALE, SHORT_DATE_OPTIONS) : '-'
}

export function formatShortDateTime(value) {
  return formatDateTime(value, SHORT_DATE_TIME_OPTIONS)
}

export function formatClockTime(value) {
  const date = parseDate(value)
  return date ? date.toLocaleTimeString(DEFAULT_LOCALE, CLOCK_TIME_OPTIONS) : '-'
}

export function formatBytes(value) {
  const bytes = Number(value) || 0
  if (bytes < 1024) return `${bytes.toFixed(0)} B`

  let amount = bytes
  let index = -1
  do {
    amount /= 1024
    index += 1
  } while (amount >= 1024 && index < BYTE_UNITS.length - 1)

  return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${BYTE_UNITS[index]}`
}
