<template>
  <div id="app-root">
    <el-dialog
      v-model="showLegal"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
      width="560px"
      class="legal-dialog"
    >
      <template #header>
        <div class="legal-header">
          <svg class="legal-shield" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
            <path d="M9 12l2 2 4-4"/>
          </svg>
          <span>授权声明</span>
        </div>
      </template>
      <div class="legal-body">
        <div class="legal-line">
          <span class="legal-bullet"></span>
          <span>本平台仅可用于您<strong>合法拥有或已获书面授权</strong>的资产安全评估。</span>
        </div>
        <div class="legal-line">
          <span class="legal-bullet"></span>
          <span>严禁对未授权第三方系统进行任何扫描、探测或漏洞验证。使用本工具产生的一切后果由使用者自行承担。</span>
        </div>
        <div class="legal-line">
          <span class="legal-bullet"></span>
          <span>继续操作即表示您已阅读、理解并同意上述条款。</span>
        </div>
      </div>
      <template #footer>
        <button class="legal-ack-btn" @click="ackLegal">
          <span>我已知悉并遵守</span>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
        </button>
      </template>
    </el-dialog>
    <router-view />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const showLegal = ref(false)

onMounted(() => {
  if (localStorage.getItem('atlas_legal_ack') !== '1') {
    showLegal.value = true
  }
})

function ackLegal() {
  localStorage.setItem('atlas_legal_ack', '1')
  showLegal.value = false
}
</script>

<style>
#app-root {
  height: 100vh;
  background: var(--bg-body);
  background-image:
    radial-gradient(ellipse at 50% 0%, rgba(0,212,255,0.04) 0%, transparent 60%),
    radial-gradient(ellipse at 80% 100%, rgba(123,97,255,0.03) 0%, transparent 50%);
}

.legal-dialog .el-dialog {
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  box-shadow: var(--shadow-card), 0 0 60px rgba(0,212,255,0.08);
}
.legal-dialog .el-dialog__header {
  padding: 24px 28px 0;
  background: transparent;
  border-bottom: none;
}
.legal-dialog .el-dialog__body { padding: 20px 28px; }
.legal-dialog .el-dialog__footer { padding: 0 28px 28px; display: flex; justify-content: center; }

.legal-header {
  display: flex; align-items: center; gap: 12px;
  font-family: var(--font-heading); font-size: 20px; font-weight: 600;
  color: var(--accent-cyan); letter-spacing: 0.04em;
}
.legal-shield { width: 26px; height: 26px; color: var(--accent-amber); }

.legal-body { display: flex; flex-direction: column; gap: 14px; }
.legal-line {
  display: flex; align-items: flex-start; gap: 10px;
  color: var(--text-secondary); line-height: 1.7; font-size: 14px;
}
.legal-line strong { color: var(--accent-amber); font-weight: 600; }
.legal-bullet {
  flex-shrink: 0; width: 6px; height: 6px; border-radius: 50%;
  background: var(--accent-cyan); margin-top: 9px;
  box-shadow: 0 0 6px rgba(0,212,255,0.5);
}

.legal-ack-btn {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 10px 28px;
  background: transparent;
  border: 1.5px solid var(--accent-cyan);
  border-radius: var(--radius-md);
  color: var(--accent-cyan);
  font-family: var(--font-heading); font-size: 15px; font-weight: 600;
  letter-spacing: 0.04em;
  cursor: pointer;
  transition: all 0.25s ease;
  position: relative; overflow: hidden;
}
.legal-ack-btn::before {
  content: ''; position: absolute; inset: 0;
  background: linear-gradient(135deg, rgba(0,212,255,0.15) 0%, transparent 50%);
  opacity: 0; transition: opacity 0.25s;
}
.legal-ack-btn:hover {
  border-color: var(--accent-cyan);
  box-shadow: 0 0 24px rgba(0,212,255,0.2);
  transform: translateY(-1px);
}
.legal-ack-btn:hover::before { opacity: 1; }
.legal-ack-btn svg { width: 16px; height: 16px; transition: transform 0.25s; }
.legal-ack-btn:hover svg { transform: translateX(3px); }
</style>
