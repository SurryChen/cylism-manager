import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import dns from 'node:dns'
import { Agent as HttpsAgent } from 'node:https'
import lintCssTokensPlugin from './vite-plugin-lint-css.js'

// cylism.crazycoding.top 的 DNS 记录当前指向阿里云备案拦截节点，
// 通过自定义 lookup 固定解析到真实服务器 IP，域名本身保持不变
// （SNI / Host 头 / 证书校验都还是 cylism.crazycoding.top）。
const FIXED_IP = '149.13.91.192'

const lookup = (hostname, options, callback) => {
  if (hostname === 'cylism.crazycoding.top') {
    if (options?.all) {
      callback(null, [{ address: FIXED_IP, family: 4 }])
    } else {
      callback(null, FIXED_IP, 4)
    }
  } else {
    dns.lookup(hostname, options, callback)
  }
}

const fixedIpAgent = new HttpsAgent({ lookup })

export default defineConfig({
  plugins: [vue(), lintCssTokensPlugin()],
  server: {
    proxy: {
      '/api': {
        target: 'https://cylism.crazycoding.top/',
        changeOrigin: true,
        ws: true,
        // The upstream terminal endpoint enforces same-origin WebSocket upgrades.
        // This proxy is only exposed by Vite's local development server.
        rewriteWsOrigin: true,
        agent: fixedIpAgent
      }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  }
})
