import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './styles/theme.css'
import './styles/components.css'
import { initializePalette } from './composables/usePalette.js'

initializePalette()
const app = createApp(App)
app.use(router)
app.mount('#app')
