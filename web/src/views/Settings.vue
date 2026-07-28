<template>
  <div class="settings-view">
    <ViewHeader title="系统设置" sub="扫描速率与审计策略 · 修改即时生效并持久化至配置文件">
      <button class="btn-reload" @click="load" :disabled="saving">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
        </svg>
        重新加载
      </button>
    </ViewHeader>

    <Banner v-if="error" type="error">{{ error }}</Banner>
    <Banner v-if="notice" type="warn">{{ notice }}</Banner>

    <!-- 扫描速率 -->
    <section class="card">
      <div class="card-head">
        <span class="card-icon" v-html="scanIcon"></span>
        <div>
          <h2 class="card-title">扫描速率</h2>
          <p class="card-desc">控制探测并发与对单个目标的请求频率，防止触发目标防护或造成网络拥塞</p>
        </div>
      </div>

      <div class="field-grid">
        <label class="field">
          <span class="field-label">全局最大并发</span>
          <input class="field-input" type="number" min="1" v-model.number="form.maxConcurrency" />
          <span class="field-hint">单实例同时执行的探测上限（建议 500）</span>
        </label>
        <label class="field">
          <span class="field-label">每目标速率 (RPS)</span>
          <input class="field-input" type="number" min="1" v-model.number="form.perTargetRPS" />
          <span class="field-hint">对单个 IP / 域名的每秒请求数（建议 10）</span>
        </label>
        <label class="field">
          <span class="field-label">默认扫描模式</span>
          <select class="field-input" v-model="form.defaultMode">
            <option v-for="m in modes" :key="m" :value="m">{{ m }}</option>
          </select>
          <span class="field-hint">新建任务时预填的探测方式</span>
        </label>
        <label class="field">
          <span class="field-label">默认端口范围</span>
          <input class="field-input" type="text" v-model="form.defaultPortRange" placeholder="top1000 | 1-1000 | 80,443,8080" />
          <span class="field-hint">支持 top1000 / 列表 / 区间</span>
        </label>
        <label class="field">
          <span class="field-label">抓包网卡 (RawIface)</span>
          <select class="field-input" v-model="form.rawIface">
            <option value="">自动（默认，留空）</option>
            <option v-for="i in interfaces" :key="i.name" :value="i.name">
              {{ i.name }}{{ i.addrs && i.addrs.length ? ' (' + i.addrs[0] + ')' : '' }}
            </option>
          </select>
          <span class="field-hint">raw 模式(SYN/ACK/FIN/Null/Xmas)抓包所用网卡；当前后端实例主机可用接口如下（通常选非 lo 的出口网卡）</span>
        </label>
      </div>
    </section>

    <!-- 审计日志 -->
    <section class="card">
      <div class="card-head">
        <span class="card-icon" v-html="auditIcon"></span>
        <div>
          <h2 class="card-title">审计日志</h2>
          <p class="card-desc">记录操作行为（登录、创建任务、黑名单变更等）到审计表，便于溯源</p>
        </div>
      </div>

      <label class="toggle-row">
        <span class="toggle-info">
          <span class="toggle-label">启用审计记录</span>
          <span class="toggle-hint">关闭后所有操作不再写入审计日志</span>
        </span>
        <button class="toggle" :class="{ on: form.auditEnabled }" @click="form.auditEnabled = !form.auditEnabled">
          <span class="toggle-knob"></span>
        </button>
      </label>
    </section>

    <footer class="view-foot">
      <span class="foot-meta">最后更新：{{ lastSaved || '尚未保存' }}</span>
      <PrimaryButton text="保存配置" loading-text="保存中…" :loading="saving" @click="save" />
    </footer>
  </div>
</template>

<script setup>
import { reactive, ref, onMounted } from 'vue'
import { api } from '../api'
import ViewHeader from '../components/ViewHeader.vue'
import Banner from '../components/Banner.vue'
import PrimaryButton from '../components/PrimaryButton.vue'

const modes = ['connect', 'syn', 'ack', 'fin', 'null', 'xmas']

const scanIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>'
const auditIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="9" y1="15" x2="15" y2="15"/><line x1="9" y1="11" x2="13" y2="11"/></svg>'

const form = reactive({
  maxConcurrency: 500,
  perTargetRPS: 10,
  defaultMode: 'connect',
  defaultPortRange: 'top1000',
  rawIface: '',
  auditEnabled: true
})

const saving = ref(false)
const error = ref('')
const notice = ref('')
const lastSaved = ref('')
const interfaces = ref([])

async function loadInterfaces() {
  try {
    const list = await api.getInterfaces()
    if (Array.isArray(list)) interfaces.value = list
  } catch (e) {
    // 静默降级：interfaces 保持为空 → 下拉仅「自动（默认）」选项，不污染 error
    interfaces.value = []
  }
}

async function load() {
  error.value = ''
  notice.value = ''
  try {
    const cfg = await api.getConfig()
    form.maxConcurrency = cfg.scan.max_concurrency
    form.perTargetRPS = cfg.scan.per_target_rps
    form.defaultMode = cfg.scan.default_mode
    form.defaultPortRange = cfg.scan.default_port_range
    form.rawIface = cfg.scan.raw_iface || ''
    form.auditEnabled = cfg.audit.enabled
  } catch (e) {
    error.value = '加载配置失败：' + e.message
  }
}

async function save() {
  error.value = ''
  notice.value = ''
  saving.value = true
  try {
    const res = await api.updateConfig({
      scan: {
        max_concurrency: form.maxConcurrency,
        per_target_rps: form.perTargetRPS,
        default_mode: form.defaultMode,
        default_port_range: form.defaultPortRange,
        raw_iface: form.rawIface
      },
      audit: { enabled: form.auditEnabled }
    })
    if (res.warning) notice.value = res.warning
    lastSaved.value = new Date().toLocaleTimeString()
  } catch (e) {
    error.value = '保存失败：' + e.message
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  load()
  loadInterfaces()
})
</script>

<style scoped>
.settings-view { max-width: 880px; margin: 0 auto; }

.btn-reload {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 14px; border-radius: var(--radius-md);
  background: var(--bg-surface); border: 1px solid var(--border-subtle);
  color: var(--text-secondary); font-family: var(--font-heading);
  font-size: 12px; font-weight: 600; letter-spacing: 0.05em; cursor: pointer;
  transition: all 0.2s ease; white-space: nowrap;
}
.btn-reload svg { width: 14px; height: 14px; }
.btn-reload:hover:not(:disabled) { border-color: var(--accent-cyan); color: var(--accent-cyan); }
.btn-reload:disabled { opacity: 0.5; cursor: not-allowed; }

.card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: 22px 24px; margin-bottom: 18px;
  position: relative; overflow: hidden;
}
.card::before {
  content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 2px;
  background: linear-gradient(180deg, var(--accent-cyan), transparent);
  opacity: 0.6;
}
.card-head { display: flex; align-items: center; gap: 14px; margin-bottom: 20px; }
.card-icon {
  width: 38px; height: 38px; flex-shrink: 0; display: flex; align-items: center;
  justify-content: center; border-radius: var(--radius-md);
  background: rgba(0,212,255,0.08); color: var(--accent-cyan);
}
.card-icon :deep(svg) { width: 20px; height: 20px; }
.card-title { font-family: var(--font-heading); font-size: 16px; font-weight: 700; letter-spacing: 0.05em; color: var(--text-primary); margin: 0 0 3px; }
.card-desc { margin: 0; font-size: 12px; color: var(--text-muted); line-height: 1.5; }

.field-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px 18px; }
.field { display: flex; flex-direction: column; gap: 6px; }
.field-label { font-family: var(--font-heading); font-size: 12px; font-weight: 600; letter-spacing: 0.04em; color: var(--text-secondary); }
.field-input {
  background: var(--bg-input); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md); color: var(--text-primary);
  padding: 10px 12px; font-family: var(--font-mono); font-size: 13px;
  transition: all 0.2s ease; width: 100%; outline: none;
}
.field-input:focus { border-color: var(--accent-cyan); box-shadow: 0 0 0 2px rgba(0,212,255,0.15); }
select.field-input { cursor: pointer; }
.field-hint { font-size: 11px; color: var(--text-muted); }

.toggle-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; border-radius: var(--radius-md);
  background: var(--bg-input); cursor: pointer;
}
.toggle-info { display: flex; flex-direction: column; gap: 3px; }
.toggle-label { font-family: var(--font-heading); font-size: 14px; font-weight: 600; color: var(--text-primary); }
.toggle-hint { font-size: 11px; color: var(--text-muted); }

.toggle {
  width: 48px; height: 26px; border-radius: 999px; flex-shrink: 0;
  border: 1px solid var(--border-subtle); background: var(--bg-elevated);
  position: relative; cursor: pointer; transition: all 0.25s ease;
}
.toggle-knob { position: absolute; top: 2px; left: 2px; width: 20px; height: 20px; border-radius: 50%; background: var(--text-muted); transition: all 0.25s ease; }
.toggle.on { background: rgba(0,230,118,0.2); border-color: var(--accent-green); }
.toggle.on .toggle-knob { left: 24px; background: var(--accent-green); box-shadow: 0 0 8px rgba(0,230,118,0.6); }

.view-foot { display: flex; align-items: center; justify-content: flex-end; gap: 16px; margin-top: 8px; }
.foot-meta { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); }

/* 小屏：表单两列改为单列，页脚纵向堆叠 */
@media (max-width: 640px) {
  .field-grid { grid-template-columns: 1fr; }
  .view-foot { flex-direction: column; align-items: stretch; }
  .view-foot :deep(.primary-btn) { justify-content: center; }
}
</style>
