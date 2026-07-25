import { readFileSync, readdirSync } from 'fs'
import { resolve, relative, join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC_DIR = resolve(__dirname, 'src')

const FORBIDDEN = [
  { re: /(?<![-.\w"'])#[0-9a-fA-F]{3,8}\b/g, label: 'hex color' },
  { re: /\b(rgb|rgba|hsl|hsla)\s*\(\s*[^)]+\s*\)/gi, label: 'rgb/hsl color function' },
  { re: /(?<![-\w])color\s*:\s*(white|black|red|green|blue|gray|grey|transparent|currentColor|inherit|initial|unset)\b(?![-\w])/g, label: 'named color (use token)' },
]

// Files allowed to use raw colors (theme definitions, base component styles)
const ALLOWED = new Set([
  resolve(SRC_DIR, 'styles/theme.css'),
  resolve(SRC_DIR, 'styles/components.css'),
])

function walk(dir, exts) {
  const results = []
  const entries = readdirSync(dir, { withFileTypes: true })
  for (const e of entries) {
    const full = join(dir, e.name)
    if (e.name.startsWith('.') || e.name === 'node_modules') continue
    if (e.isDirectory()) { results.push(...walk(full, exts)) }
    else if (exts.some(ext => e.name.endsWith(ext))) { results.push(full) }
  }
  return results
}

function lintFile(filePath) {
  const src = readFileSync(filePath, 'utf8')
  const lines = src.split('\n')
  const rel = relative(SRC_DIR, filePath)

  if (ALLOWED.has(filePath)) return []

  const errors = []
  for (const { re, label } of FORBIDDEN) {
    for (let i = 0; i < lines.length; i++) {
      for (const m of lines[i].matchAll(re)) {
        errors.push(`${rel}:${i + 1}:${m.index + 1}  硬编码${label} "${m[0]}" — 请使用 CSS 变量`)
      }
    }
  }
  return errors
}

export function lintAll() {
  const files = walk(SRC_DIR, ['.vue', '.css'])
  let errors = []
  for (const f of files) errors.push(...lintFile(f))
  return errors
}
