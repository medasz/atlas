<template>
  <div class="login-stage">
    <div class="login-grid-bg"></div>

    <div class="login-card">
      <div class="login-brand">
        <div class="brand-mark">
          <svg viewBox="0 0 40 40" fill="none">
            <circle cx="20" cy="20" r="18" stroke="currentColor" stroke-width="1.5"/>
            <circle cx="20" cy="20" r="8" stroke="currentColor" stroke-width="1.2" stroke-dasharray="4 3"/>
            <line x1="20" y1="2" x2="20" y2="10" stroke="currentColor" stroke-width="1.5"/>
            <line x1="33" y1="7" x2="28" y2="13" stroke="currentColor" stroke-width="1.5"/>
            <line x1="38" y1="20" x2="30" y2="20" stroke="currentColor" stroke-width="1.5"/>
          </svg>
        </div>
        <div class="brand-text">
          <h1>A T L A S</h1>
          <p>频谱情报</p>
        </div>
      </div>

      <div class="login-form-wrap">
        <div class="input-group">
          <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="11" width="18" height="11" rx="2"/>
            <path d="M7 11V7a5 5 0 0110 0v4"/>
            <circle cx="12" cy="16" r="1"/>
          </svg>
          <input
            ref="pwdRef"
            v-model="password"
            type="password"
            placeholder="输入管理员口令"
            @keyup.enter="doLogin"
          />
          <div class="input-underline"></div>
        </div>

        <button class="login-btn" :class="{ loading: loading }" :disabled="loading" @click="doLogin">
          <span v-if="!loading">进入系统</span>
          <span v-else class="btn-spinner">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="12"/></svg>
          </span>
        </button>

        <transition name="fade-down">
          <p v-if="error" class="login-err">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            {{ error }}
          </p>
        </transition>
      </div>
    </div>

    <footer class="login-footer">
      <span>资产 · 漏洞 · 情报</span>
    </footer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const password = ref('')
const loading = ref(false)
const error = ref('')
const router = useRouter()
const pwdRef = ref(null)

onMounted(() => { pwdRef.value?.focus() })

async function doLogin() {
  loading.value = true
  error.value = ''
  try {
    await api.login(password.value)
    localStorage.setItem('atlas_authed', '1')
    router.push('/')
  } catch (e) {
    if (e.message !== 'unauthorized') error.value = '登录失败：' + e.message
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-stage {
  height: 100vh;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  position: relative; overflow: hidden;
  background:
    radial-gradient(ellipse at 50% 30%, rgba(0,212,255,0.06) 0%, transparent 55%),
    radial-gradient(ellipse at 20% 80%, rgba(123,97,255,0.04) 0%, transparent 50%),
    var(--bg-body);
}

.login-grid-bg {
  position: absolute; inset: 0; pointer-events: none; opacity: 0.03;
  background-image:
    linear-gradient(var(--border-subtle) 1px, transparent 1px),
    linear-gradient(90deg, var(--border-subtle) 1px, transparent 1px);
  background-size: 60px 60px;
  mask-image: radial-gradient(ellipse at center, black 30%, transparent 70%);
}

.login-card {
  position: relative;
  display: flex; flex-direction: column; align-items: center; gap: 36px;
  width: 420px; padding: 48px 40px 40px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card), 0 0 80px rgba(0,212,255,0.04);
  backdrop-filter: blur(12px);
}

.login-brand { display: flex; align-items: center; gap: 16px; }
.brand-mark {
  width: 52px; height: 52px;
  color: var(--accent-cyan);
  filter: drop-shadow(0 0 12px rgba(0,212,255,0.35));
}
.brand-mark svg { width: 100%; height: 100%; }
.brand-text h1 {
  font-family: var(--font-heading); font-size: 28px; font-weight: 700;
  letter-spacing: 0.16em; color: var(--text-primary); line-height: 1;
}
.brand-text p {
  font-family: var(--font-heading); font-size: 11px; font-weight: 500;
  letter-spacing: 0.22em; color: var(--text-muted); margin-top: 4px;
  text-transform: uppercase;
}

.login-form-wrap { width: 100%; display: flex; flex-direction: column; gap: 20px; }

.input-group {
  position: relative;
}
.input-group input {
  width: 100%; padding: 12px 16px 12px 42px;
  background: var(--bg-input);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-family: var(--font-mono); font-size: 14px;
  outline: none; transition: border-color 0.25s, box-shadow 0.25s;
}
.input-group input::placeholder { color: var(--text-muted); }
.input-group input:focus {
  border-color: var(--accent-cyan);
  box-shadow: 0 0 0 3px rgba(0,212,255,0.08), inset 0 0 12px rgba(0,212,255,0.04);
}
.input-underline {
  position: absolute; bottom: 0; left: 12px; right: 12px; height: 1px;
  background: linear-gradient(90deg, transparent, var(--accent-cyan), transparent);
  opacity: 0; transition: opacity 0.25s;
}
.input-group input:focus ~ .input-underline { opacity: 0.6; }

.input-icon {
  position: absolute; left: 14px; top: 50%; transform: translateY(-50%);
  width: 18px; height: 18px; color: var(--text-muted);
  transition: color 0.25s;
}
.input-group input:focus ~ .input-icon,
.input-group input:focus + .input-icon { color: var(--accent-cyan); }

.login-btn {
  width: 100%; padding: 12px;
  background: linear-gradient(135deg, rgba(0,212,255,0.12) 0%, rgba(0,212,255,0.05) 100%);
  border: 1px solid var(--accent-cyan);
  border-radius: var(--radius-md);
  color: var(--accent-cyan);
  font-family: var(--font-heading); font-size: 15px; font-weight: 600;
  letter-spacing: 0.08em;
  cursor: pointer; transition: all 0.3s ease;
  position: relative; overflow: hidden;
}
.login-btn::after {
  content: ''; position: absolute; inset: 0;
  background: linear-gradient(135deg, rgba(0,212,255,0.2) 0%, transparent 60%);
  opacity: 0; transition: opacity 0.3s;
}
.login-btn:hover:not(:disabled) {
  box-shadow: 0 0 28px rgba(0,212,255,0.22), inset 0 0 20px rgba(0,212,255,0.06);
  transform: translateY(-1px);
}
.login-btn:hover:not(:disabled)::after { opacity: 1; }
.login-btn:active:not(:disabled) { transform: translateY(0); }
.login-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.login-btn.loading { border-color: var(--text-muted); color: var(--text-muted); }

.btn-spinner svg {
  width: 20px; height: 20px; animation: spin 1s linear infinite;
}

.login-err {
  display: flex; align-items: center; gap: 8px;
  color: var(--accent-red); font-size: 13px;
  background: rgba(255,71,87,0.08); border: 1px solid rgba(255,71,87,0.2);
  border-radius: var(--radius-sm); padding: 10px 14px;
}
.login-err svg { width: 16px; height: 16px; flex-shrink: 0; }

.login-footer {
  position: absolute; bottom: 28px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 500;
  letter-spacing: 0.28em; color: var(--text-muted);
  opacity: 0.5;
}

.fade-down-enter-active { transition: all 0.3s ease; }
.fade-down-leave-active { transition: all 0.2s ease; }
.fade-down-enter-from { opacity: 0; transform: translateY(-8px); }
.fade-down-leave-to { opacity: 0; transform: translateY(-4px); }

@keyframes spin { to { transform: rotate(360deg); } }
</style>
