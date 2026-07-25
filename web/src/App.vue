<template>
  <div v-if="isLoginPage" class="login-shell"><router-view /></div>
  <div v-else class="app-shell" :class="{ 'drawer-open': mobileNavOpen }">
    <header class="app-topbar">
      <button class="icon-button mobile-only" aria-label="打开导航菜单" @click="openMobileNav">
        <Menu :size="19" />
      </button>
      <router-link to="/" class="app-brand app-topbar-brand" @click="closeMobileNav">
        <span class="brand-symbol"><Orbit /></span>
        <span><strong>Cylism</strong><small>Operations</small></span>
      </router-link>
      <nav class="topbar-primary-nav" data-testid="primary-navigation" aria-label="一级导航">
        <router-link
          v-for="group in navGroups"
          :key="group.id"
          :to="group.to"
          class="topbar-nav-link"
          :class="{ 'is-active': activeNavGroup.id === group.id }"
          :aria-current="activeNavGroup.id === group.id ? 'page' : undefined"
        >
          {{ group.label }}
        </router-link>
      </nav>
      <div class="environment-status"><span class="status-indicator"></span><span>Production</span></div>
      <div class="topbar-actions">
        <div class="palette-picker">
          <button ref="paletteButton" class="icon-button" aria-label="选择配色" aria-haspopup="menu" :aria-expanded="paletteMenuOpen" @click="togglePaletteMenu">
            <Palette :size="18" />
          </button>
          <Teleport to="body">
          <div v-if="paletteMenuOpen" ref="paletteMenu" class="palette-menu" :style="paletteMenuStyle" data-testid="palette-menu" role="menu" @click.stop>
            <button
              v-for="palette in palettes"
              :key="palette.id"
              class="palette-option"
              :class="{ 'is-selected': activePalette === palette.id }"
              :data-palette-option="palette.id"
              role="menuitemradio"
              :aria-checked="activePalette === palette.id"
              @click="choosePalette(palette.id)"
            >
              <span class="palette-swatch" :class="`swatch-${palette.id}`"></span>
              <span>{{ palette.name }}</span>
              <Check v-if="activePalette === palette.id" :size="14" />
            </button>
          </div>
          </Teleport>
        </div>
        <button class="icon-button" aria-label="退出登录" title="退出登录" @click="logout"><LogOut :size="18" /></button>
      </div>
    </header>

    <aside class="app-sidebar" data-testid="desktop-navigation">
      <div class="sidebar-context"><span>当前模块</span><strong>{{ activeNavGroup.label }}</strong></div>
      <nav class="navigation-groups" aria-label="主导航">
        <section class="navigation-group">
          <h2>{{ activeNavGroup.label }}</h2>
          <router-link
            v-for="item in activeNavGroup.items"
            :key="item.to"
            :to="item.to"
            class="sidebar-link"
            active-class="is-active"
          >
            <component :is="item.icon" :size="17" stroke-width="1.8" />
            <span>{{ item.label }}</span>
          </router-link>
        </section>
      </nav>

      <div class="sidebar-status"><span class="status-indicator"></span><span>平台在线</span><small>4 agents connected</small></div>
    </aside>

    <div class="app-workspace">
      <main class="app-content"><router-view /></main>
    </div>

    <div v-if="mobileNavOpen" class="drawer-scrim" @click="closeMobileNav"></div>
    <aside class="mobile-drawer" :class="{ 'is-open': mobileNavOpen }" data-testid="mobile-navigation" aria-label="移动导航">
      <div class="drawer-header"><span class="app-brand"><span class="brand-symbol"><Orbit /></span><strong>Cylism</strong></span><button class="icon-button" aria-label="关闭导航菜单" @click="closeMobileNav"><X :size="19" /></button></div>
      <nav class="navigation-groups" aria-label="移动主导航">
        <section v-for="group in navGroups" :key="group.label" class="navigation-group">
          <h2>{{ group.label }}</h2>
          <router-link v-for="item in group.items" :key="item.to" :to="item.to" class="sidebar-link" active-class="is-active" @click="closeMobileNav"><component :is="item.icon" :size="17" /><span>{{ item.label }}</span></router-link>
        </section>
      </nav>
      <div class="drawer-palette"><span>界面配色</span><div class="palette-swatches"><button v-for="palette in palettes" :key="palette.id" :data-palette-option="palette.id" :class="['palette-swatch', `swatch-${palette.id}`, { 'is-selected': activePalette === palette.id }]" :aria-label="palette.name" @click="choosePalette(palette.id)"></button></div></div>
    </aside>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Boxes, Check, Database, FileText, Globe, LayoutDashboard, LogOut, Menu, Network, Orbit, Palette, Route, Server, Settings, ShieldCheck, X } from 'lucide-vue-next'
import { clearTokens } from './api/index.js'
import { usePalette } from './composables/usePalette.js'

const router = useRouter()
const route = useRoute()
const mobileNavOpen = ref(false)
const paletteMenuOpen = ref(false)
const paletteButton = ref(null)
const paletteMenu = ref(null)
const paletteMenuStyle = ref({})
const { activePalette, selectPalette } = usePalette()
const isLoginPage = computed(() => route.path === '/login')

const palettes = [
  { id: 'mint', name: '薄荷玻璃' },
  { id: 'blue', name: '雾蓝玻璃' },
  { id: 'orchid', name: '兰花玻璃' },
  { id: 'sky', name: '天空紫雾' },
  { id: 'night', name: '夜间玻璃' },
]

const navGroups = computed(() => [
  { id: 'overview', label: '概览', to: '/', items: [{ label: '概览', to: '/', icon: LayoutDashboard }] },
  { id: 'infrastructure', label: '基础设施', to: '/servers', items: [{ label: '组网配置', to: '/network', icon: Network }, { label: '服务器', to: '/servers', icon: Server }, { label: '工作负载', to: '/workloads', icon: Globe }, { label: '服务发现', to: '/services', icon: Network }, { label: '配置', to: '/configs', icon: Settings }, { label: '路由', to: '/routes', icon: Route }, { label: '证书', to: '/certs', icon: ShieldCheck }, { label: '资源', to: '/resources', icon: Boxes }] },
  { id: 'records', label: '记录', to: '/audit', items: [{ label: '审计', to: '/audit', icon: FileText }, { label: '数据管理', to: '/db-admin', icon: Database }] },
])

const activeNavGroup = computed(() => navGroups.value.find((group) => group.items.some((item) => item.to === route.path)) || navGroups.value[0])

function choosePalette(palette) {
  selectPalette(palette)
  paletteMenuOpen.value = false
}

function updatePaletteMenuPosition() {
  if (!paletteButton.value || !paletteMenuOpen.value) return

  const rect = paletteButton.value.getBoundingClientRect()
  const menuWidth = 184
  const viewportPadding = 12
  const left = Math.max(viewportPadding, Math.min(window.innerWidth - menuWidth - viewportPadding, rect.right - menuWidth))
  const top = Math.max(viewportPadding, Math.min(window.innerHeight - 248, rect.bottom + 10))

  paletteMenuStyle.value = { left: `${left}px`, top: `${top}px` }
}

function togglePaletteMenu() {
  paletteMenuOpen.value = !paletteMenuOpen.value
}

function closePaletteMenuOnOutsideClick(event) {
  if (paletteButton.value?.contains(event.target) || paletteMenu.value?.contains(event.target)) return
  paletteMenuOpen.value = false
}

watch(paletteMenuOpen, async (isOpen) => {
  if (isOpen) {
    await nextTick()
    updatePaletteMenuPosition()
    document.addEventListener('click', closePaletteMenuOnOutsideClick)
    return
  }

  document.removeEventListener('click', closePaletteMenuOnOutsideClick)
})

onMounted(() => {
  window.addEventListener('resize', updatePaletteMenuPosition)
  window.addEventListener('scroll', updatePaletteMenuPosition, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', closePaletteMenuOnOutsideClick)
  window.removeEventListener('resize', updatePaletteMenuPosition)
  window.removeEventListener('scroll', updatePaletteMenuPosition, true)
})

function openMobileNav() { mobileNavOpen.value = true }
function closeMobileNav() { mobileNavOpen.value = false }
function logout() { clearTokens(); router.push('/login') }
</script>
