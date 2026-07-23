<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-header">
        <span class="brand-mark">◆</span>
        <h1>Cylism Manager</h1>
      </div>
      <form @submit.prevent="login">
        <div class="form-group">
          <label class="form-label">用户名</label>
          <input v-model="username" class="form-input" placeholder="admin" required autofocus />
        </div>
        <div class="form-group">
          <label class="form-label">密码</label>
          <input v-model="password" class="form-input" type="password" placeholder="••••••" required />
        </div>
        <div v-if="error" class="login-error">{{ error }}</div>
        <button type="submit" class="btn btn-primary login-btn" :disabled="loading">
          {{ loading ? '登录中...' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { setTokens } from '../api/index.js'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function login() {
  error.value = ''
  loading.value = true
  try {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username.value, password: password.value })
    })
    if (!res.ok) {
      const data = await res.json()
      error.value = data.error || '登录失败'
      loading.value = false
      return
    }
    const data = await res.json()
    setTokens(data.access_token, data.refresh_token)
    router.push('/')
  } catch (e) {
    error.value = '网络错误，请重试'
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: var(--bg-deep);
}

.login-card {
  width: 380px;
  max-width: 90vw;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: var(--space-32);
}

.login-header {
  display: flex;
  align-items: center;
  gap: var(--space-12);
  margin-bottom: var(--space-32);
  justify-content: center;
}

.login-header h1 {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.login-error {
  background: rgba(248, 81, 73, 0.1);
  border: 1px solid var(--danger);
  border-radius: var(--radius-md);
  padding: var(--space-8) var(--space-12);
  font-size: 13px;
  color: var(--danger);
  margin-bottom: var(--space-16);
}

.login-btn {
  width: 100%;
  justify-content: center;
  padding: var(--space-12);
  font-size: 14px;
}
</style>
