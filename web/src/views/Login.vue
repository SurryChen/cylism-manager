<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-header">
        <span class="login-mark">◇</span>
        <div><h1>Cylism</h1><p>Operations Manager</p></div>
      </div>
      <form @submit.prevent="login">
        <div class="form-group">
          <label class="form-label">用户名</label>
          <input v-model="username" class="form-input" placeholder="admin" :required="!temporaryToken" :disabled="!!temporaryToken" autofocus />
        </div>
        <div class="form-group">
          <label class="form-label">密码</label>
          <input v-model="password" class="form-input" type="password" placeholder="••••••" :required="!temporaryToken" :disabled="!!temporaryToken" />
        </div>
        <div class="login-divider"><span>或使用临时秘钥</span></div>
        <div class="form-group">
          <label class="form-label">临时秘钥</label>
          <input v-model.trim="temporaryToken" class="form-input" placeholder="粘贴临时登录秘钥" autocomplete="off" />
        </div>
        <div v-if="error" class="login-error">{{ error }}</div>
        <button type="submit" class="btn btn-primary login-btn" :disabled="loading">
          {{ loading ? '登录中...' : temporaryToken ? '使用临时秘钥登录' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { setTokens } from '../api/index.js'
import { login as loginWithCredentials, temporaryLogin } from '../api/auth.js'

const router = useRouter()
const username = ref('')
const password = ref('')
const temporaryToken = ref('')
const error = ref('')
const loading = ref(false)

async function login() {
  error.value = ''
  loading.value = true
  try {
    const data = temporaryToken.value
      ? await temporaryLogin(temporaryToken.value)
      : await loginWithCredentials(username.value, password.value)
    setTokens(data.access_token, data.refresh_token)
    router.push('/')
  } catch (e) {
    error.value = e.message || '网络错误，请重试'
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 24px;
  background: var(--canvas);
}

.login-page::before,
.login-page::after {
  position: fixed;
  z-index: 0;
  width: 72vw;
  height: 55vh;
  content: '';
  opacity: .72;
  pointer-events: none;
  transform: skewY(-16deg);
}

.login-page::before { top: -28vh; right: -18vw; background: var(--band-a); }
.login-page::after { bottom: -28vh; left: -18vw; background: var(--band-b); }

.login-card {
  position: relative;
  z-index: 1;
  width: 390px;
  max-width: 100%;
  padding: 30px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--surface-raised);
  box-shadow: var(--shadow);
  backdrop-filter: blur(24px) saturate(125%);
}

.login-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 28px;
}

.login-mark { display: grid; width: 35px; height: 35px; place-items: center; border: 1px solid var(--border); border-radius: 10px; background: var(--surface-subtle); color: var(--action-primary); font-size: 20px; }
.login-header h1 {
  margin: 0;
  color: var(--text-primary);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0;
}
.login-header p { margin: 2px 0 0; color: var(--text-muted); font: 10px/1 var(--font-mono); letter-spacing: .08em; text-transform: uppercase; }

.login-error {
  margin-bottom: var(--space-16);
  padding: var(--space-8) var(--space-12);
  border: 1px solid var(--danger);
  border-radius: var(--radius-control);
  background: var(--danger-surface);
  color: var(--danger);
  font-size: 12px;
}

.login-divider { display: flex; align-items: center; gap: 10px; margin: 18px 0 14px; color: var(--text-muted); font-size: 11px; }
.login-divider::before, .login-divider::after { flex: 1; height: 1px; background: var(--border); content: ''; }

.login-btn {
  width: 100%;
  justify-content: center;
  padding: var(--space-12);
  font-size: 14px;
}
</style>
