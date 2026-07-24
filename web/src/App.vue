<template>
  <div class="app-shell" :class="[{ 'nav-open': mobileNavOpen }, themeClass]">
    <div v-if="mobileNavOpen" class="nav-overlay" @click="closeMobileNav"></div>

    <header class="app-header">
      <div class="header-left">
        <button class="menu-toggle" @click="toggleMobileNav" aria-label="Menu">
          <span class="menu-bar"></span><span class="menu-bar"></span><span class="menu-bar"></span>
        </button>
        <span class="brand-mark">◆</span>
        <h1 class="brand-name">Cylism Manager</h1>
      </div>
      <nav class="header-nav desktop-nav">
        <router-link to="/" class="nav-item" exact-active-class="nav-active"><span class="nav-icon">◫</span><span>概览</span></router-link>
        <router-link to="/servers" class="nav-item" active-class="nav-active"><span class="nav-icon">⬡</span><span>服务器</span></router-link>
        <router-link to="/routes" class="nav-item" active-class="nav-active"><span class="nav-icon">⊞</span><span>路由</span></router-link>
        <router-link to="/certs" class="nav-item" active-class="nav-active"><span class="nav-icon">🔒</span><span>证书</span></router-link>
        <router-link to="/resources" class="nav-item" active-class="nav-active"><span class="nav-icon">▤</span><span>资源</span></router-link>
        <router-link to="/audit" class="nav-item" active-class="nav-active"><span class="nav-icon">☰</span><span>审计</span></router-link>
        <router-link to="/db-admin" class="nav-item" active-class="nav-active"><span class="nav-icon">⊡</span><span>数据</span></router-link>
      </nav>
      <div class="header-right">
        <span class="status-dot" title="Platform running"></span>
        <button class="theme-toggle" @click="toggleTheme" :title="themeLabel">
          {{ themeIcon }}
        </button>
        <button class="btn-logout" @click="logout">退出</button>
      </div>
    </header>

    <nav class="mobile-nav" :class="{ 'mobile-nav--open': mobileNavOpen }">
      <div class="mobile-nav-header"><span class="brand-mark">◆</span><span class="mobile-brand-text">Cylism Manager</span></div>
      <router-link to="/" class="nav-item" exact-active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">◫</span><span>概览</span></router-link>
      <router-link to="/servers" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⬡</span><span>服务器</span></router-link>
      <router-link to="/routes" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⊞</span><span>路由</span></router-link>
      <router-link to="/certs" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">🔒</span><span>证书</span></router-link>
      <router-link to="/resources" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">▤</span><span>资源</span></router-link>
      <router-link to="/audit" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">☰</span><span>审计</span></router-link>
      <router-link to="/db-admin" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⊡</span><span>数据</span></router-link>
      <div class="mobile-nav-footer">
        <button class="theme-toggle theme-toggle--full" @click="toggleTheme">
          {{ themeIcon }} {{ themeLabel }}
        </button>
      </div>
    </nav>

    <main class="app-main"><router-view /></main>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { clearTokens } from './api/index.js'

const router = useRouter()
const mobileNavOpen = ref(false)
const theme = ref('dark')

onMounted(() => {
  const saved = localStorage.getItem('theme')
  if (saved) theme.value = saved
})

const themeClass = computed(() => 'theme-' + theme.value)
const themeIcon = computed(() => theme.value === 'dark' ? '☀' : '☾')
const themeLabel = computed(() => theme.value === 'dark' ? '亮色' : '暗色')

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem('theme', theme.value)
}

function toggleMobileNav() { mobileNavOpen.value = !mobileNavOpen.value }
function closeMobileNav() { mobileNavOpen.value = false }
function logout() { clearTokens(); router.push('/login') }
</script>

<style>
/* ============================================
   Soft Tech Design Tokens
   ============================================ */
:root,
.theme-dark {
  --bg-deep: #0f0f14;
  --bg-surface: #1a1a24;
  --bg-raised: #232336;
  --bg-hover: #2a2a3d;
  --border: #2e2e42;
  --border-muted: #222233;
  --text-primary: #e8e8f0;
  --text-secondary: #9494a8;
  --text-muted: #5c5c72;
  --accent: #7c6ff7;
  --accent-dim: #5a4fd4;
  --accent-glow: rgba(124, 111, 247, 0.15);
  --success: #34d399;
  --warn: #fbbf24;
  --danger: #f87171;
  --overlay-bg: rgba(0, 0, 0, 0.55);
  --shadow-sm: 0 1px 3px rgba(0, 0, 0, 0.08);
  --shadow-md: 0 4px 16px rgba(0, 0, 0, 0.12);
  --shadow-lg: 0 8px 32px rgba(0, 0, 0, 0.16);
  --radius-xs: 4px;
  --radius-sm: 6px;
  --radius-md: 8px;
  --radius-lg: 12px;
  --space-4: 4px;
  --space-8: 8px;
  --space-12: 12px;
  --space-16: 16px;
  --space-20: 20px;
  --space-24: 24px;
  --space-32: 32px;
  --space-48: 48px;
  --space-64: 64px;
  --nav-width: 240px;
  --header-height: 56px;
}

.theme-light {
  --bg-deep: #f5f5fa;
  --bg-surface: #ffffff;
  --bg-raised: #fafafe;
  --bg-hover: #eef0f8;
  --border: #e0e2ec;
  --border-muted: #eef0f6;
  --text-primary: #1a1a2e;
  --text-secondary: #6b6b80;
  --text-muted: #9b9bb0;
  --accent: #6c5ce7;
  --accent-dim: #5a4bd1;
  --accent-glow: rgba(108, 92, 231, 0.1);
  --success: #10b981;
  --warn: #d97706;
  --danger: #ef4444;
  --overlay-bg: rgba(0, 0, 0, 0.3);
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.04);
  --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.06);
  --shadow-lg: 0 8px 24px rgba(0, 0, 0, 0.08);
}

/* ============================================
   Base Reset
   ============================================ */
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box }

html {
  font-size: 15px;
  overflow-y: scroll;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
  background: var(--bg-deep);
  color: var(--text-primary);
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}

/* ============================================
   Scrollbar — overlay, no layout shift
   ============================================ */
* {
  scrollbar-width: thin;
  scrollbar-color: var(--text-muted) transparent;
}

::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: var(--text-muted);
  border-radius: 3px;
}

::-webkit-scrollbar-thumb:hover {
  background: var(--text-secondary);
}

::-webkit-scrollbar-corner {
  background: transparent;
}

/* ============================================
   Layout
   ============================================ */
.app-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-header {
  position: sticky;
  top: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  height: var(--header-height);
  padding: 0 var(--space-24);
  gap: var(--space-32);
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-muted);
  backdrop-filter: blur(12px);
}

.header-left { display: flex; align-items: center; gap: var(--space-8) }
.brand-mark { font-size: 18px; color: var(--accent) }
.brand-name { font-size: 15px; font-weight: 600; letter-spacing: -0.02em; color: var(--text-primary) }
.header-nav { display: flex; gap: var(--space-4); flex: 1 }
.header-right { display: flex; align-items: center; gap: var(--space-8) }

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  padding: var(--space-8) var(--space-16);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  text-decoration: none;
  font-size: 13px;
  font-weight: 500;
  transition: background 0.2s ease, color 0.2s ease;
}
.nav-item:hover { background: var(--bg-hover); color: var(--text-primary) }
.nav-active { background: var(--accent-glow); color: var(--accent) }
.nav-icon { font-size: 14px }

.status-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--success) }

.theme-toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-4);
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
  padding: var(--space-4) var(--space-12);
  font-size: 14px;
  font-family: inherit;
  transition: background 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}
.theme-toggle:hover { background: var(--bg-hover); color: var(--text-primary); border-color: var(--text-muted) }

.theme-toggle--full {
  width: 100%;
  justify-content: center;
  padding: var(--space-8) var(--space-16);
}

.btn-logout {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 12px;
  font-family: inherit;
  padding: var(--space-4) var(--space-12);
  transition: background 0.2s ease, color 0.2s ease;
}
.btn-logout:hover { background: var(--bg-hover); color: var(--text-primary) }

/* Mobile nav */
.menu-toggle { display: none; flex-direction: column; gap: 4px; background: none; border: none; cursor: pointer; padding: 4px }
.menu-bar { width: 20px; height: 2px; background: var(--text-primary); border-radius: 1px; transition: transform 0.2s ease }

.mobile-nav {
  display: none;
  flex-direction: column;
  position: fixed;
  top: 0; left: 0; bottom: 0;
  width: var(--nav-width);
  background: var(--bg-surface);
  z-index: 60;
  padding: var(--space-16) 0;
  border-right: 1px solid var(--border);
  transform: translateX(-100%);
  transition: transform 0.25s ease;
}
.mobile-nav--open { transform: translateX(0) }
.mobile-nav-header { display: flex; align-items: center; gap: var(--space-8); padding: 0 var(--space-16); margin-bottom: var(--space-16) }
.mobile-brand-text { font-weight: 600; font-size: 14px; }
.mobile-nav .nav-item { margin: 0 var(--space-8); padding: var(--space-12) var(--space-16) }
.mobile-nav-footer { padding: var(--space-16); border-top: 1px solid var(--border-muted); margin-top: auto }

.nav-overlay { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.45); z-index: 55; backdrop-filter: blur(2px) }

.app-main { flex: 1; padding: var(--space-32) var(--space-48); max-width: 1280px; width: 100%; margin: 0 auto }

/* ============================================
   Buttons
   ============================================ */
.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-8) var(--space-16);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 500;
  font-family: inherit;
  cursor: pointer;
  transition: background 0.2s ease, border-color 0.2s ease, color 0.2s ease, box-shadow 0.2s ease;
  white-space: nowrap;
}
.btn:hover { background: var(--bg-hover); border-color: var(--text-muted) }
.btn:disabled { opacity: 0.45; cursor: not-allowed }
.btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px }

.btn-primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
  font-weight: 600;
}
.btn-primary:hover { opacity: 0.9; box-shadow: 0 2px 8px var(--accent-glow) }
.theme-light .btn-primary { color: #fff }

.btn-danger {
  color: var(--danger);
  border-color: var(--danger);
}
.btn-danger:hover { background: rgba(248, 113, 113, 0.1) }

.btn-sm { padding: var(--space-4) var(--space-12); font-size: 12px }
.btn-group { display: flex; gap: var(--space-8); flex-wrap: wrap }

/* ============================================
   Cards
   ============================================ */
.card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-24);
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.2s ease;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-16);
}

/* ============================================
   Tables
   ============================================ */
.data-table {
  width: 100%;
  border-collapse: collapse;
}
.data-table th {
  text-align: left;
  padding: var(--space-8) var(--space-16);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--border-muted);
}
.data-table td {
  padding: var(--space-12) var(--space-16);
  font-size: 14px;
  border-bottom: 1px solid var(--border-muted);
  font-variant-numeric: tabular-nums;
}
.data-table tr { transition: background 0.15s ease }
.data-table tr:hover td { background: var(--bg-hover) }

/* ============================================
   Badges
   ============================================ */
.badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-4);
  padding: 2px var(--space-8);
  border-radius: var(--radius-xs);
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}
.badge-online { background: rgba(52, 211, 153, 0.12); color: var(--success) }
.badge-offline { background: rgba(148, 148, 168, 0.08); color: var(--text-secondary) }
.badge-deploying { background: rgba(251, 191, 36, 0.12); color: var(--warn) }
.badge-warn { background: rgba(251, 191, 36, 0.12); color: var(--warn) }
.badge-danger { background: rgba(248, 113, 113, 0.12); color: var(--danger) }
.badge-dot { width: 6px; height: 6px; border-radius: 50%; display: inline-block }
.badge-online .badge-dot { background: var(--success) }
.badge-offline .badge-dot { background: var(--text-muted) }

/* ============================================
   Forms
   ============================================ */
.form-group { margin-bottom: var(--space-16) }
.form-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: var(--space-4);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.form-input,
.form-select {
  width: 100%;
  padding: var(--space-8) var(--space-12);
  background: var(--bg-deep);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-primary);
  font-size: 14px;
  font-family: inherit;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.form-input:focus,
.form-select:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-glow);
}
.form-input::placeholder { color: var(--text-muted) }
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-16) }

/* ============================================
   Empty State
   ============================================ */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--space-64) var(--space-24);
  color: var(--text-muted);
  text-align: center;
  gap: var(--space-12);
}
.empty-icon { font-size: 32px; opacity: 0.4 }
.empty-text { font-size: 14px; max-width: 320px }

/* ============================================
   Metrics
   ============================================ */
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-16);
  margin-bottom: var(--space-32);
}
.metric {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-16) var(--space-24);
  box-shadow: var(--shadow-sm);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.metric:hover { border-color: var(--accent-dim); box-shadow: var(--shadow-md) }
.metric-value {
  font-size: 28px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
  line-height: 1.2;
}
.metric-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-top: var(--space-4);
}
.metric-accent { color: var(--accent) }
.metric-warn { color: var(--warn) }
.metric-success { color: var(--success) }

/* ============================================
   Page Header
   ============================================ */
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-32);
  gap: var(--space-16);
  flex-wrap: wrap;
}
.page-title {
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

/* ============================================
   Modal
   ============================================ */
.overlay {
  position: fixed;
  inset: 0;
  background: var(--overlay-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: var(--space-16);
  backdrop-filter: blur(4px);
}
.modal {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: var(--space-32);
  width: 520px;
  max-width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}
.modal-title {
  font-size: 16px;
  font-weight: 700;
  margin-bottom: var(--space-24);
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-12);
  margin-top: var(--space-24);
}

/* ============================================
   Table Tabs
   ============================================ */
.table-tabs { display: flex; gap: 4px }
.tab-btn {
  padding: var(--space-8) var(--space-16);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--bg-deep);
  color: var(--text-secondary);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: inherit;
}
.tab-btn:hover { background: var(--bg-hover); color: var(--text-primary) }
.tab-active {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
  font-weight: 600;
}

.table-wrap { overflow-x: auto; -webkit-overflow-scrolling: touch }

/* ============================================
   Log
   ============================================ */
.log-list { display: flex; flex-direction: column }
.log-item {
  display: flex;
  gap: var(--space-12);
  padding: var(--space-8) 0;
  border-bottom: 1px solid var(--border-muted);
  align-items: flex-start;
}
.log-item:last-child { border-bottom: none }
.log-status { font-size: 16px; min-width: 24px; text-align: center; line-height: 1.4 }
.log-content { flex: 1; min-width: 0 }
.log-step { font-size: 14px; font-weight: 500 }
.log-detail { font-size: 12px; color: var(--text-secondary); margin-top: 2px; word-break: break-all }
.log-time { font-size: 11px; color: var(--text-muted); margin-top: 2px }
.log-success { color: var(--success) }
.log-failed { color: var(--danger) }
.log-running { color: var(--warn) }
.row-selected td { background: var(--accent-glow) !important }

/* ============================================
   K8s Banner
   ============================================ */
.k8s-banner {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  padding: var(--space-8) var(--space-16);
  border-radius: var(--radius-sm);
  font-size: 13px;
  margin-bottom: var(--space-16);
}
.k8s-banner-warn {
  background: rgba(251, 191, 36, 0.08);
  border: 1px solid rgba(251, 191, 36, 0.2);
  color: var(--warn);
}
.k8s-banner-ok {
  background: rgba(52, 211, 153, 0.08);
  border: 1px solid rgba(52, 211, 153, 0.2);
  color: var(--success);
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 768px) {
  .desktop-nav { display: none }
  .menu-toggle { display: flex }
  .mobile-nav { display: flex }
  .app-header { padding: 0 var(--space-16); gap: var(--space-16) }
  .app-main { padding: var(--space-24) var(--space-16) }
  .metric-grid { grid-template-columns: repeat(2, 1fr); gap: var(--space-12) }
  .metric-value { font-size: 24px }
  .form-row { grid-template-columns: 1fr; gap: 0 }
  .page-header { flex-direction: column; align-items: flex-start }
  .modal { padding: var(--space-24); width: calc(100% - 32px) }
}

@media (max-width: 480px) {
  .metric-grid { grid-template-columns: 1fr 1fr; gap: var(--space-8) }
  .metric { padding: var(--space-12) var(--space-16) }
  .metric-value { font-size: 20px }
  .card { padding: var(--space-16) }
  .data-table th,
  .data-table td { padding: var(--space-8); font-size: 13px }
  .brand-name { display: none }
}
</style>
