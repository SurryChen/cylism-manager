import DefaultTheme from 'vitepress/theme'
import Layout from './Layout.vue'
import ScreenshotPlaceholder from './components/ScreenshotPlaceholder.vue'
import './styles.css'

export default {
  extends: DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component('ScreenshotPlaceholder', ScreenshotPlaceholder)
  },
}
