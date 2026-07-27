<template>
  <div class="vulns-view">
    <ViewHeader title="漏洞管理" sub="检测结果与检测模板 · 按严重等级聚合，支持模板热加载" />

    <Banner v-if="error" type="error">{{ error }}</Banner>

    <!-- 切换标签 -->
    <div class="tabs">
      <button class="tab" :class="{ active: tab === 'results' }" @click="tab = 'results'">
        检测结果
        <span class="tab-count">{{ vulns.length }}</span>
      </button>
      <button class="tab" :class="{ active: tab === 'templates' }" @click="tab = 'templates'">
        检测模板
        <span class="tab-count">{{ templates.length }}</span>
      </button>
    </div>

    <!-- 检测结果 -->
    <section v-if="tab === 'results'">
      <div class="filter-bar">
        <div class="search-box">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
          </svg>
          <input class="search-input" v-model.trim="assetFilter" placeholder="按资产过滤 (IP / 域名)…" @keyup.enter="loadVulns" />
        </div>
        <button class="btn-ghost" @click="loadVulns" :disabled="loading">刷新</button>
      </div>

      <!-- 等级统计 -->
      <div class="stat-row" v-if="vulns.length">
        <div class="stat-chip" v-for="s in levelStats" :key="s.label" :style="{ borderColor: s.color, color: s.color }">
          <span class="stat-dot" :style="{ background: s.color }"></span>
          <span class="stat-num">{{ s.count }}</span>
          <span class="stat-label">{{ s.label }}</span>
        </div>
      </div>

      <div class="vuln-card" v-if="vulns.length">
        <el-table :data="vulns" v-loading="loading" stripe class="vuln-table">
          <el-table-column prop="asset_ref" label="资产" min-width="180" show-overflow-tooltip />
          <el-table-column label="漏洞名称" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ row.name || row.kpid }}</template>
          </el-table-column>
          <el-table-column prop="cve" label="CVE" width="150" />
          <el-table-column label="等级" width="120" align="center">
            <template #default="{ row }">
              <StatusTag :text="levelLabel(row.level)" :tone="levelTone(row.level)" />
            </template>
          </el-table-column>
          <el-table-column prop="type" label="类型" width="140" show-overflow-tooltip>
            <template #default="{ row }">{{ row.type || '—' }}</template>
          </el-table-column>
          <el-table-column label="状态" width="110" align="center">
            <template #default="{ row }">
              <StatusTag :text="statusLabel(row.status)" :tone="statusTone(row.status)" />
            </template>
          </el-table-column>
          <el-table-column label="首次发现" width="160" show-overflow-tooltip>
            <template #default="{ row }">{{ fmtDate(row.first_found) }}</template>
          </el-table-column>
        </el-table>
      </div>
      <EmptyState v-else :text="loading ? '加载中…' : '暂无漏洞记录'" />
    </section>

    <!-- 检测模板 -->
    <section v-else>
      <div class="filter-bar">
        <span class="hint-text">已加载 {{ templates.length }} 个检测模板（nuclei 风格）</span>
        <button class="btn-ghost" @click="openEditor">+ 新增模板</button>
      </div>

      <div class="tpl-grid" v-if="templates.length">
        <div class="tpl-card" v-for="t in templates" :key="t.id">
          <div class="tpl-top">
            <StatusTag :text="(t.severity || '—').toUpperCase()" :tone="sevTone((t.severity || '').toLowerCase())" />
            <span class="tpl-name">{{ t.name }}</span>
          </div>
          <div class="tpl-id">{{ t.id }}</div>
          <div class="tpl-tags" v-if="t.tags">
            <span class="tag" v-for="tag in t.tags.split(',')" :key="tag.trim()">{{ tag.trim() }}</span>
          </div>
        </div>
      </div>
      <EmptyState v-else text="暂无模板，点击右上角新增" />
    </section>

    <!-- 新增模板弹窗 -->
    <div class="modal-mask" v-if="editorOpen" @click.self="editorOpen = false">
      <div class="modal">
        <div class="modal-head">
          <h3>新增检测模板</h3>
          <button class="modal-close" @click="editorOpen = false">✕</button>
        </div>
        <p class="modal-desc">粘贴 nuclei 风格 YAML 模板（需含 <code>id</code> 与 <code>requests</code>）</p>
        <textarea class="tpl-editor" v-model="tplContent" spellcheck="false"
          placeholder="id: CVE-2024-xxxx&#10;info:&#10;  name: Example RCE&#10;  severity: high&#10;requests:&#10;  - method: GET&#10;    path: /&#10;    matchers:&#10;      - type: status&#10;        status: 200"></textarea>
        <div class="modal-foot">
          <span class="editor-err" v-if="editorErr">{{ editorErr }}</span>
          <PrimaryButton text="提交模板" loading-text="提交中…" :loading="submitting" @click="submitTemplate" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api'
import ViewHeader from '../components/ViewHeader.vue'
import Banner from '../components/Banner.vue'
import EmptyState from '../components/EmptyState.vue'
import StatusTag from '../components/StatusTag.vue'
import PrimaryButton from '../components/PrimaryButton.vue'

const tab = ref('results')
const vulns = ref([])
const templates = ref([])
const loading = ref(false)
const error = ref('')
const assetFilter = ref('')

const editorOpen = ref(false)
const tplContent = ref('')
const submitting = ref(false)
const editorErr = ref('')

// level (int) → 严重等级映射
const LEVELS = [
  { label: 'INFO', color: '#00D4FF' },
  { label: 'LOW', color: '#00E676' },
  { label: 'MEDIUM', color: '#F5A623' },
  { label: 'HIGH', color: '#FF8C42' },
  { label: 'CRITICAL', color: '#FF4757' }
]
function levelLabel(lvl) { return (LEVELS[lvl] || { label: 'L' + lvl }).label }
// 等级 → 统一配色（INFO 青 / LOW 绿 / MEDIUM 橙 / HIGH·CRITICAL 红）
function levelTone(lvl) { return ['cyan', 'green', 'amber', 'red', 'red'][lvl] || 'muted' }
function statusLabel(s) { return { open: '未修复', fixed: '已修复', recur: '复发' }[s] || s || '—' }
function statusTone(s) { return { open: 'red', fixed: 'green', recur: 'amber' }[s] || 'muted' }
// 模板 severity 字符串 → 配色
function sevTone(s) {
  return { info: 'cyan', low: 'green', medium: 'amber', high: 'red', critical: 'red' }[s] || 'muted'
}

const levelStats = computed(() => {
  const counts = {}
  vulns.value.forEach((v) => { counts[v.level] = (counts[v.level] || 0) + 1 })
  return LEVELS.map((l, i) => ({ ...l, count: counts[i] || 0 })).filter((s) => s.count > 0)
})

function fmtDate(d) {
  if (!d) return '—'
  try { return new Date(d).toLocaleDateString() } catch { return '—' }
}

async function loadVulns() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.listVulns(assetFilter.value)
    vulns.value = res.items || []
  } catch (e) {
    error.value = '加载漏洞失败：' + e.message
  } finally {
    loading.value = false
  }
}

async function loadTemplates() {
  try {
    const res = await api.listTemplates()
    templates.value = res.items || []
  } catch (e) {
    error.value = '加载模板失败：' + e.message
  }
}

function openEditor() { editorOpen.value = true; tplContent.value = ''; editorErr.value = '' }

async function submitTemplate() {
  editorErr.value = ''
  if (!tplContent.value.trim()) { editorErr.value = '模板内容不能为空'; return }
  submitting.value = true
  try {
    await api.addTemplate(tplContent.value)
    editorOpen.value = false
    await loadTemplates()
  } catch (e) {
    editorErr.value = e.message
  } finally {
    submitting.value = false
  }
}

onMounted(() => { loadVulns(); loadTemplates() })
</script>

<style scoped>
.vulns-view { max-width: 980px; margin: 0 auto; }

.tabs { display: flex; gap: 8px; margin-bottom: 18px; }
.tab {
  display: flex; align-items: center; gap: 8px;
  padding: 9px 16px; border-radius: var(--radius-md);
  background: var(--bg-surface); border: 1px solid var(--border-subtle);
  color: var(--text-secondary); font-family: var(--font-heading);
  font-size: 13px; font-weight: 600; letter-spacing: 0.05em; cursor: pointer; transition: all 0.2s ease;
}
.tab.active { border-color: var(--accent-cyan); color: var(--accent-cyan); background: rgba(0,212,255,0.06); }
.tab-count { font-family: var(--font-mono); font-size: 11px; padding: 1px 7px; border-radius: 999px; background: var(--bg-elevated); color: var(--text-muted); }
.tab.active .tab-count { color: var(--accent-cyan); }

.filter-bar { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.search-box {
  display: flex; align-items: center; gap: 8px; flex: 1; max-width: 360px;
  background: var(--bg-input); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md); padding: 0 12px; transition: all 0.2s ease;
}
.search-box:focus-within { border-color: var(--accent-cyan); box-shadow: 0 0 0 2px rgba(0,212,255,0.15); }
.search-box svg { width: 15px; height: 15px; color: var(--text-muted); flex-shrink: 0; }
.search-input { flex: 1; background: transparent; border: none; outline: none; color: var(--text-primary); padding: 10px 0; font-family: var(--font-mono); font-size: 13px; }
.btn-ghost {
  padding: 9px 16px; border-radius: var(--radius-md);
  background: var(--bg-surface); border: 1px solid var(--border-subtle);
  color: var(--text-secondary); font-family: var(--font-heading); font-size: 12px;
  font-weight: 600; letter-spacing: 0.05em; cursor: pointer; transition: all 0.2s ease;
}
.btn-ghost:hover:not(:disabled) { border-color: var(--accent-cyan); color: var(--accent-cyan); }
.btn-ghost:disabled { opacity: 0.5; cursor: not-allowed; }

.stat-row { display: flex; gap: 10px; margin-bottom: 16px; flex-wrap: wrap; }
.stat-chip {
  display: flex; align-items: center; gap: 7px;
  padding: 8px 14px; border-radius: var(--radius-md);
  border: 1px solid; background: var(--bg-surface); font-family: var(--font-heading);
}
.stat-dot { width: 7px; height: 7px; border-radius: 50%; }
.stat-num { font-family: var(--font-mono); font-size: 15px; font-weight: 700; }
.stat-label { font-size: 11px; font-weight: 600; letter-spacing: 0.05em; }

.vuln-card {
  background: var(--bg-surface); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg); overflow: hidden;
}
.vuln-table { border-radius: 0; }
.cell-asset { font-family: var(--font-mono); color: var(--text-primary); }
.cell-name { color: var(--text-primary); font-weight: 500; }
.cell-muted { color: var(--text-muted); }

.hint-text { font-size: 12px; color: var(--text-muted); flex: 1; }
.tpl-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 14px; }
.tpl-card {
  background: var(--bg-surface); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg); padding: 16px; transition: all 0.2s ease;
}
.tpl-card:hover { border-color: rgba(0,212,255,0.3); }
.tpl-top { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.tpl-name { font-family: var(--font-heading); font-size: 13px; font-weight: 600; color: var(--text-primary); }
.tpl-id { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); margin-bottom: 10px; }
.tpl-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.tag {
  font-family: var(--font-mono); font-size: 10px; padding: 2px 7px; border-radius: var(--radius-sm, 4px);
  background: rgba(123,97,255,0.12); color: #9B8CFF; border: 1px solid rgba(123,97,255,0.25);
}

.modal-mask {
  position: fixed; inset: 0; background: rgba(3,7,14,0.7); backdrop-filter: blur(3px);
  display: flex; align-items: center; justify-content: center; z-index: 50; padding: 20px;
}
.modal {
  width: 100%; max-width: 560px; background: var(--bg-elevated);
  border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
  padding: 22px; box-shadow: 0 20px 60px rgba(0,0,0,0.5);
}
.modal-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
.modal-head h3 { margin: 0; font-family: var(--font-heading); font-size: 16px; font-weight: 700; color: var(--text-primary); }
.modal-close { background: transparent; border: none; color: var(--text-muted); font-size: 16px; cursor: pointer; }
.modal-close:hover { color: var(--accent-red); }
.modal-desc { margin: 0 0 14px; font-size: 12px; color: var(--text-muted); }
.modal-desc code { font-family: var(--font-mono); color: var(--accent-cyan); }
.tpl-editor {
  width: 100%; min-height: 240px; resize: vertical;
  background: var(--bg-input); border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
  color: var(--text-primary); padding: 12px; font-family: var(--font-mono); font-size: 12px;
  line-height: 1.6; outline: none;
}
.tpl-editor:focus { border-color: var(--accent-cyan); }
.modal-foot { display: flex; align-items: center; justify-content: flex-end; gap: 14px; margin-top: 14px; }
.editor-err { color: var(--accent-red); font-size: 12px; margin-right: auto; }

/* 小屏：标签与过滤栏纵向堆叠 */
@media (max-width: 560px) {
  .filter-bar { flex-direction: column; align-items: stretch; }
  .search-box { max-width: none; }
  .tabs { overflow-x: auto; }
}
</style>
