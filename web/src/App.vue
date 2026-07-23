<template>
  <div class="app-shell" :class="{ 'nav-open': mobileNavOpen }">
    <div v-if="mobileNavOpen" class="nav-overlay" @click="closeMobileNav"></div>

    <header class="app-header">
      <div class="header-left">
        <button class="menu-toggle" @click="toggleMobileNav" aria-label="Menu">
          <span class="menu-bar"></span><span class="menu-bar"></span><span class="menu-bar"></span>
        </button>
        <span class="brand-mark">◆</span>
        <h1 class="brand-name">Cylism</h1>
      </div>
      <nav class="header-nav desktop-nav">
        <router-link to="/" class="nav-item" exact-active-class="nav-active"><span class="nav-icon">◫</span><span>概览</span></router-link>
        <router-link to="/servers" class="nav-item" active-class="nav-active"><span class="nav-icon">⬡</span><span>服务器</span></router-link>
        <router-link to="/sites" class="nav-item" active-class="nav-active"><span class="nav-icon">⊞</span><span>站点</span></router-link>
        <router-link to="/import" class="nav-item" active-class="nav-active"><span class="nav-icon">↗</span><span>导入</span></router-link>
        <router-link to="/audit" class="nav-item" active-class="nav-active"><span class="nav-icon">☰</span><span>审计</span></router-link>
        <router-link to="/db-admin" class="nav-item" active-class="nav-active"><span class="nav-icon">⊡</span><span>数据管理</span></router-link>
      </nav>
      <div class="header-right">
        <span class="status-dot" title="Platform running"></span>
        <button class="btn btn-sm" style="background:none;border-color:var(--text-muted);color:var(--text-secondary);margin-left:12px;" @click="logout">退出</button>
      </div>
    </header>

    <nav class="mobile-nav" :class="{ 'mobile-nav--open': mobileNavOpen }">
      <div class="mobile-nav-header"><span class="brand-mark">◆</span><span style="font-weight:600;font-size:14px;">Cylism</span></div>
      <router-link to="/" class="nav-item" exact-active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">◫</span><span>概览</span></router-link>
      <router-link to="/servers" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⬡</span><span>服务器</span></router-link>
      <router-link to="/sites" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⊞</span><span>站点</span></router-link>
      <router-link to="/import" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">↗</span><span>导入</span></router-link>
      <router-link to="/audit" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">☰</span><span>审计</span></router-link>
      <router-link to="/db-admin" class="nav-item" active-class="nav-active" @click="closeMobileNav"><span class="nav-icon">⊡</span><span>数据管理</span></router-link>
    </nav>

    <main class="app-main"><router-view /></main>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { clearTokens } from './api/index.js'
const router = useRouter()
const mobileNavOpen = ref(false)
function toggleMobileNav() { mobileNavOpen.value = !mobileNavOpen.value }
function closeMobileNav() { mobileNavOpen.value = false }
function logout() { clearTokens(); router.push('/login') }
</script>

<style>
:root {
  --bg-deep:#0d1117;--bg-surface:#161b22;--bg-raised:#1c2333;--bg-hover:#21283a;
  --border:#30363d;--border-muted:#21262d;
  --text-primary:#e6edf3;--text-secondary:#8b949e;--text-muted:#484f58;
  --accent:#2dd4bf;--accent-dim:#1a7f72;--accent-glow:rgba(45,212,191,0.15);
  --warn:#f59e0b;--danger:#f85149;--success:#3fb950;
  --radius-sm:4px;--radius-md:8px;--radius-lg:12px;
  --space-4:4px;--space-8:8px;--space-12:12px;--space-16:16px;--space-24:24px;--space-32:32px;--space-48:48px;--space-64:64px;
  --nav-width:240px;--header-height:56px;
}
*,*::before,*::after{margin:0;padding:0;box-sizing:border-box}
html{font-size:15px}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;background:var(--bg-deep);color:var(--text-primary);line-height:1.5;-webkit-font-smoothing:antialiased}
.app-shell{display:flex;flex-direction:column;min-height:100vh}
.app-header{position:sticky;top:0;z-index:50;display:flex;align-items:center;height:var(--header-height);padding:0 var(--space-24);background:var(--bg-surface);border-bottom:1px solid var(--border-muted);gap:var(--space-32)}
.header-left{display:flex;align-items:center;gap:var(--space-8)}
.brand-mark{font-size:18px;color:var(--accent)}
.brand-name{font-size:15px;font-weight:600;letter-spacing:-0.02em;color:var(--text-primary)}
.header-nav{display:flex;gap:var(--space-4);flex:1}
.nav-item{display:flex;align-items:center;gap:var(--space-8);padding:var(--space-8) var(--space-16);border-radius:var(--radius-md);color:var(--text-secondary);text-decoration:none;font-size:13px;font-weight:500;transition:background 0.15s,color 0.15s}
.nav-item:hover{background:var(--bg-hover);color:var(--text-primary)}
.nav-active{background:var(--accent-glow);color:var(--accent)}
.nav-icon{font-size:14px}
.header-right{display:flex;align-items:center}
.status-dot{width:8px;height:8px;border-radius:50%;background:var(--accent);box-shadow:0 0 6px var(--accent)}
.menu-toggle{display:none;flex-direction:column;gap:4px;background:none;border:none;cursor:pointer;padding:var(--space-4)}
.menu-bar{display:block;width:20px;height:2px;background:var(--text-primary);border-radius:2px;transition:transform 0.2s,opacity 0.2s}
.nav-open .menu-bar:nth-child(1){transform:translateY(6px) rotate(45deg)}
.nav-open .menu-bar:nth-child(2){opacity:0}
.nav-open .menu-bar:nth-child(3){transform:translateY(-6px) rotate(-45deg)}
.mobile-nav{display:none;position:fixed;top:0;left:0;bottom:0;width:var(--nav-width);background:var(--bg-surface);border-right:1px solid var(--border);z-index:100;padding:var(--space-16);flex-direction:column;gap:var(--space-4);transform:translateX(-100%);transition:transform 0.25s cubic-bezier(0.4,0,0.2,1)}
.mobile-nav--open{transform:translateX(0)}
.mobile-nav-header{display:flex;align-items:center;gap:var(--space-8);padding:var(--space-8) var(--space-12);margin-bottom:var(--space-8);font-size:14px;color:var(--text-primary)}
.nav-overlay{position:fixed;inset:0;background:rgba(0,0,0,0.5);z-index:90;opacity:0;animation:fadeIn 0.2s ease forwards}
@keyframes fadeIn{to{opacity:1}}
.app-main{flex:1;padding:var(--space-32) var(--space-24);max-width:1200px;width:100%;margin:0 auto}
.card{background:var(--bg-surface);border:1px solid var(--border);border-radius:var(--radius-lg);padding:var(--space-24)}
.card-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:var(--space-16)}
.card-title{font-size:13px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:0.04em}
.btn{display:inline-flex;align-items:center;gap:var(--space-8);padding:var(--space-8) var(--space-16);border:1px solid var(--border);border-radius:var(--radius-md);background:var(--bg-raised);color:var(--text-primary);font-size:13px;font-weight:500;cursor:pointer;transition:background 0.15s,border-color 0.15s;text-decoration:none;white-space:nowrap}
.btn:hover{background:var(--bg-hover);border-color:var(--text-muted)}
.btn-primary{background:var(--accent);border-color:var(--accent);color:var(--bg-deep);font-weight:600}
.btn-primary:hover{background:#5eeadb;border-color:#5eeadb}
.btn-danger{border-color:var(--danger);color:var(--danger)}
.btn-danger:hover{background:rgba(248,81,73,0.1)}
.btn-sm{padding:var(--space-4) var(--space-12);font-size:12px}
.btn-group{display:flex;gap:var(--space-8)}
.data-table{width:100%;border-collapse:collapse}
.data-table th{text-align:left;padding:var(--space-8) var(--space-16);font-size:12px;font-weight:600;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;border-bottom:1px solid var(--border-muted)}
.data-table td{padding:var(--space-12) var(--space-16);font-size:14px;border-bottom:1px solid var(--border-muted);font-variant-numeric:tabular-nums}
.data-table tr:hover td{background:var(--bg-hover)}
.badge{display:inline-flex;align-items:center;gap:var(--space-4);padding:2px var(--space-8);border-radius:var(--radius-sm);font-size:12px;font-weight:500;white-space:nowrap}
.badge-online{background:rgba(63,185,80,0.12);color:var(--success)}
.badge-offline{background:rgba(139,148,158,0.1);color:var(--text-secondary)}
.badge-deploying{background:rgba(245,158,11,0.12);color:var(--warn)}
.badge-warn{background:rgba(245,158,11,0.12);color:var(--warn)}
.badge-danger{background:rgba(248,81,73,0.12);color:var(--danger)}
.badge-dot{width:6px;height:6px;border-radius:50%;display:inline-block}
.badge-online .badge-dot{background:var(--success)}
.badge-offline .badge-dot{background:var(--text-muted)}
.form-group{margin-bottom:var(--space-16)}
.form-label{display:block;font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:var(--space-4);text-transform:uppercase;letter-spacing:0.04em}
.form-input,.form-select{width:100%;padding:var(--space-8) var(--space-12);background:var(--bg-deep);border:1px solid var(--border);border-radius:var(--radius-md);color:var(--text-primary);font-size:14px;font-family:inherit;transition:border-color 0.15s}
.form-input:focus,.form-select:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 2px var(--accent-glow)}
.form-input::placeholder{color:var(--text-muted)}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:var(--space-16)}
.empty-state{display:flex;flex-direction:column;align-items:center;justify-content:center;padding:var(--space-64) var(--space-24);color:var(--text-muted);text-align:center;gap:var(--space-12)}
.empty-icon{font-size:32px;opacity:0.5}
.empty-text{font-size:14px;max-width:320px}
.metric-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:var(--space-16);margin-bottom:var(--space-32)}
.metric{background:var(--bg-surface);border:1px solid var(--border);border-radius:var(--radius-lg);padding:var(--space-16) var(--space-24)}
.metric-value{font-size:28px;font-weight:700;font-variant-numeric:tabular-nums;letter-spacing:-0.02em;line-height:1.2}
.metric-label{font-size:12px;font-weight:500;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.05em;margin-top:var(--space-4)}
.metric-accent{color:var(--accent)}.metric-warn{color:var(--warn)}
.page-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:var(--space-32);gap:var(--space-16);flex-wrap:wrap}
.page-title{font-size:20px;font-weight:700;letter-spacing:-0.02em}
.overlay{position:fixed;inset:0;background:rgba(0,0,0,0.6);display:flex;align-items:center;justify-content:center;z-index:100;padding:var(--space-16)}
.modal{background:var(--bg-surface);border:1px solid var(--border);border-radius:var(--radius-lg);padding:var(--space-32);width:520px;max-width:100%;max-height:90vh;overflow-y:auto}
.modal-title{font-size:16px;font-weight:700;margin-bottom:var(--space-24)}
.modal-actions{display:flex;justify-content:flex-end;gap:var(--space-12);margin-top:var(--space-24)}
.table-wrap{overflow-x:auto;-webkit-overflow-scrolling:touch}
@media (max-width:768px){.desktop-nav{display:none}.menu-toggle{display:flex}.mobile-nav{display:flex}.app-header{padding:0 var(--space-16);gap:var(--space-16)}.app-main{padding:var(--space-24) var(--space-16)}.metric-grid{grid-template-columns:repeat(2,1fr);gap:var(--space-12)}.metric-value{font-size:24px}.form-row{grid-template-columns:1fr;gap:0}.page-header{flex-direction:column;align-items:flex-start}.modal{padding:var(--space-24)}}
@media (max-width:480px){.metric-grid{grid-template-columns:1fr 1fr;gap:var(--space-8)}.metric{padding:var(--space-12) var(--space-16)}.metric-value{font-size:20px}.card{padding:var(--space-16)}.data-table th,.data-table td{padding:var(--space-8);font-size:13px}.brand-name{display:none}}

/* 操作日志 */
.log-list { display: flex; flex-direction: column; }
.log-item { display: flex; gap: var(--space-12); padding: var(--space-8) 0; border-bottom: 1px solid var(--border-muted); align-items: flex-start; }
.log-item:last-child { border-bottom: none; }
.log-status { font-size: 16px; min-width: 24px; text-align: center; line-height: 1.4; }
.log-content { flex: 1; min-width: 0; }
.log-step { font-size: 14px; font-weight: 500; }
.log-detail { font-size: 12px; color: var(--text-secondary); margin-top: 2px; word-break: break-all; }
.log-time { font-size: 11px; color: var(--text-muted); margin-top: 2px; }
.log-success { color: var(--success); }
.log-failed { color: var(--danger); }
.log-running { color: var(--warn); }
.row-selected td { background: var(--accent-glow) !important; }

</style>
