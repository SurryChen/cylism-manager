import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import lintCssTokensPlugin from './vite-plugin-lint-css.js'

export default defineConfig({
  plugins: [vue(), lintCssTokensPlugin()],
  server: {
    proxy: {
      '/api': {
        target: 'https://cylism.crazycoding.top/',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  }
})
