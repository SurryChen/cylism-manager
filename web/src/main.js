import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import SelectMenu from './components/SelectMenu.vue'
import SurfaceCard from './components/SurfaceCard.vue'
import './styles/theme.css'
import './styles/components.css'
import { initializePalette } from './composables/usePalette.js'

initializePalette()
const app = createApp(App)
app.use(router)
app.component('SelectMenu', SelectMenu)
app.component('SurfaceCard', SurfaceCard)

// 全局 Vue 错误处理：捕获未处理的渲染/生命周期异常
app.config.errorHandler = (err, instance, info) => {
  console.error('[Global] Vue 异常:', err)
  console.error('[Global] 组件:', instance?.$.type?.__name || instance?.$options?.name || 'unknown')
  console.error('[Global] 详情:', info)
}
app.config.warnHandler = (msg, instance, trace) => {
  console.warn('[Global] Vue 警告:', msg, trace)
}

app.mount('#app')
