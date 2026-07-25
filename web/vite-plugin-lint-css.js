import { lintAll } from './lint-css-vars.js'

export default function lintCssTokensPlugin() {
  return {
    name: 'lint-css-tokens',
    buildStart() {
      const errors = lintAll()
      if (errors.length > 0) {
        console.error('\n🎨 CSS Design Token 检查失败:\n')
        for (const e of errors) console.error(`  ❌ ${e}`)
        console.error(`\n  共 ${errors.length} 个错误。请使用 CSS 变量（如 var(--text-primary)）代替硬编码颜色。\n`)
        this.error(`发现 ${errors.length} 个硬编码颜色，构建中止`)
      }
    }
  }
}
