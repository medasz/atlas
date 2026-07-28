<template>
  <div class="assets-page">
    <!-- 搜索栏 -->
    <div class="search-panel">
      <div class="search-bar">
        <div class="search-input-wrap">
          <svg class="search-prefix" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
          </svg>
          <input
            v-model="q"
            placeholder="检索语法 · ip=&quot;1.1.1.1&quot; && port=&quot;443&quot;"
            @keyup.enter="doSearch"
            autofocus
          />
          <span v-if="q" class="search-clear" @click="q=''; doSearch()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </span>
        </div>
        <button class="search-btn" :class="{ loading: loading }" @click="doSearch">
          <svg v-if="!loading" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="12"/>
          </svg>
          <span>检索</span>
        </button>
        <button class="syntax-trigger" :class="{ active: syntaxOpen }" type="button"
          @click="syntaxOpen = !syntaxOpen" title="检索语法手册">
          <span class="st-ico" v-html="icons.book"></span>
          <span class="st-label">语法</span>
        </button>
      </div>

      <!-- DORK 语法手册：入口在搜索框右侧，内嵌为弹层 -->
      <div class="syntax-pop" v-show="syntaxOpen">
        <DorkCheatSheet embedded @close="syntaxOpen = false" @apply="onApplyDork" />
      </div>
      <div class="syntax-backdrop" v-if="syntaxOpen" @click="syntaxOpen = false"></div>
    </div>

    <!-- 结果 -->
    <div class="results-section">
      <div class="results-meta">
        <span class="results-count" v-if="items.length">
          <span class="count-num">{{ items.length }}</span> 条结果
        </span>
      </div>

      <el-table :data="items" v-loading="loading" stripe class="assets-table">
        <el-table-column prop="doc_type" label="类型" width="90" />
        <el-table-column label="目标" width="220">
          <template #default="{ row }"><span class="cell-target">{{ row.name || row.ip || '-' }}</span></template>
        </el-table-column>
        <el-table-column prop="port" label="端口" width="80" />
        <el-table-column prop="service" label="服务" width="120" />
        <el-table-column prop="version" label="版本" width="130" />
        <el-table-column prop="title" label="标题" min-width="170" show-overflow-tooltip>
          <template #default="{ row }"><span class="cell-title">{{ row.title || '-' }}</span></template>
        </el-table-column>
        <el-table-column prop="server" label="服务" width="150" show-overflow-tooltip />
        <el-table-column v-if="hasDomain" label="根域名" prop="registrable_domain" width="170" show-overflow-tooltip />
        <el-table-column prop="org" label="组织" width="150" show-overflow-tooltip />
        <el-table-column prop="asn" label="ASN" width="90" />
        <el-table-column label="IPv6" width="72" align="center">
          <template #default="{ row }">
            <span class="ipv6-badge" :class="{ on: row.is_ipv6 }">{{ row.is_ipv6 ? 'v6' : 'v4' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="80" align="center">
          <template #default="{ row }">
            <button v-if="row.doc_type==='host'" class="detail-trigger" @click="openHost(row.ip)">
              <span>详情</span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12,5 19,12 12,19"/></svg>
            </button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 主机详情对话框 -->
    <el-dialog v-model="hostVisible" width="800px" class="host-dialog">
      <template #header>
        <DialogHeader title="主机详情" :icon="icons.host" />
      </template>
      <template v-if="detail">
        <DetailGrid :items="hostItems" />

        <SectionLabel label="端口 · 服务 · 指纹" />
        <el-table :data="detail.ports" size="small" stripe class="sub-table">
          <el-table-column prop="port" label="端口" width="72" />
          <el-table-column prop="proto" label="协议" width="70" />
          <el-table-column prop="service" label="服务" width="110" />
          <el-table-column prop="version" label="版本" width="120" />
          <el-table-column prop="title" label="标题" min-width="150" show-overflow-tooltip />
          <el-table-column label="服务" width="140" show-overflow-tooltip>
            <template #default="{ row }">{{ (row.webinfo && row.webinfo.server) || row.server || '—' }}</template>
          </el-table-column>
          <el-table-column label="技术栈" min-width="170" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="techOf(row) !== '-'" class="tech-tags">
                <span v-for="t in techList(row)" :key="t" class="tech-tag">{{ t }}</span>
              </span>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>
          <el-table-column label="IPv6" width="64" align="center">
            <template #default="{ row }">
              <span class="ipv6-badge" :class="{ on: row.is_ipv6 }">{{ row.is_ipv6 ? 'v6' : 'v4' }}</span>
            </template>
          </el-table-column>
        </el-table>

        <SectionLabel label="漏洞" :count="(detail.vulns || []).length" />
        <el-table v-if="detail.vulns && detail.vulns.length" :data="detail.vulns" size="small" stripe class="sub-table">
          <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip />
          <el-table-column prop="cve" label="CVE" width="150" />
          <el-table-column label="严重等级" width="96" align="center">
            <template #default="{ row }">
              <span class="sev-badge" :class="'sev-' + sevClass(row.level)">{{ sevLabel(row.level) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="96" align="center" />
          <el-table-column prop="asset_ref" label="引用" width="170" show-overflow-tooltip />
        </el-table>
        <el-empty v-else description="暂无漏洞" :image-size="48" />
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { api } from '../api'
import DialogHeader from '../components/DialogHeader.vue'
import DetailGrid from '../components/DetailGrid.vue'
import SectionLabel from '../components/SectionLabel.vue'
import DorkCheatSheet from '../components/DorkCheatSheet.vue'

const icons = {
  host: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="3"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></svg>',
  book: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>'
}

const q = ref('')
const items = ref([])
const loading = ref(false)
const hostVisible = ref(false)
const detail = ref(null)
const syntaxOpen = ref(false)

const hasDomain = computed(() => items.value.some(r => r.doc_type === 'domain'))

const hostItems = computed(() => {
  if (!detail.value || !detail.value.host) return []
  const h = detail.value.host
  return [
    { key: 'IP', value: h.ip },
    { key: 'IPv6', value: h.is_ipv6 ? '是' : '否' },
    { key: '所属组织', value: h.org || '—' },
    { key: '操作系统', value: h.os || '—' },
    { key: '开放端口', value: (h.open_ports || []).length, highlight: true }
  ]
})

async function doSearch() {
  loading.value = true
  try {
    const r = await api.searchAssets(q.value, '')
    items.value = r.items || []
  } finally {
    loading.value = false
  }
}

// 语法手册示例 → 填入检索框并立即检索
function onApplyDork(query) {
  q.value = query
  doSearch()
}

function techOf(row) {
  const t = row.webinfo && row.webinfo.tech
  if (Array.isArray(t)) return t.join(', ')
  return t || '-'
}
function techList(row) {
  const t = row.webinfo && row.webinfo.tech
  return Array.isArray(t) ? t : t ? [t] : []
}
function sevClass(lv) {
  if (lv >= 3) return 'high'
  if (lv === 2) return 'mid'
  return 'low'
}
function sevLabel(lv) {
  return ['信息', '低危', '中危', '高危', '严重'][lv] || lv
}

async function openHost(ip) {
  const r = await api.getHostDetail(ip)
  detail.value = r
  hostVisible.value = true
}
</script>

<style scoped>
.assets-page { display: flex; flex-direction: column; gap: 20px; }

/* ===== 搜索面板 ===== */
.search-panel {
  position: relative;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
}
.search-bar { display: flex; align-items: center; gap: 10px; padding: 16px; }

.search-input-wrap { flex: 1; position: relative; }
.search-input-wrap input {
  width: 100%; padding: 11px 40px 11px 42px;
  background: var(--bg-input);
  border: 1px solid rgba(26,42,74,0.6);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-family: var(--font-mono); font-size: 13px;
  outline: none; transition: border-color 0.25s, box-shadow 0.25s;
  caret-color: var(--accent-cyan);
}
.search-input-wrap input::placeholder { color: var(--text-muted); font-family: var(--font-body); }
.search-input-wrap input:focus {
  border-color: var(--accent-cyan);
  box-shadow: 0 0 0 3px rgba(0,212,255,0.07), inset 0 0 16px rgba(0,212,255,0.03);
}
.search-prefix { position: absolute; left: 13px; top: 50%; transform: translateY(-50%); width: 18px; height: 18px; color: var(--text-muted); transition: color 0.25s; }
.search-input-wrap input:focus ~ .search-prefix { color: var(--accent-cyan); }
.search-clear { position: absolute; right: 12px; top: 50%; transform: translateY(-50%); width: 16px; height: 16px; color: var(--text-muted); cursor: pointer; opacity: 0.5; transition: opacity 0.2s; }
.search-clear:hover { opacity: 1; color: var(--accent-red); }
.search-clear svg { width: 16px; height: 16px; }

.search-btn {
  display: flex; align-items: center; gap: 6px;
  padding: 11px 20px;
  background: linear-gradient(135deg, rgba(0,212,255,0.15) 0%, rgba(0,212,255,0.05) 100%);
  border: 1px solid rgba(0,212,255,0.28);
  border-radius: var(--radius-md);
  color: var(--accent-cyan);
  font-family: var(--font-heading); font-size: 13px; font-weight: 600;
  letter-spacing: 0.06em; cursor: pointer; transition: all 0.25s ease; white-space: nowrap;
}
.search-btn svg { width: 16px; height: 16px; }
.search-btn:hover { border-color: rgba(0,212,255,0.5); box-shadow: 0 0 20px rgba(0,212,255,0.18), inset 0 0 18px rgba(0,212,255,0.04); }
.search-btn.loading svg { animation: spin 1s linear infinite; }

/* ===== 语法弹层入口（搜索框右侧） ===== */
.syntax-trigger {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 11px 16px;
  background: var(--bg-input);
  border: 1px solid rgba(26, 42, 74, 0.6);
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  font-family: var(--font-heading); font-size: 12px; font-weight: 600; letter-spacing: 0.06em;
  cursor: pointer; transition: all 0.22s ease; white-space: nowrap;
}
.syntax-trigger svg { width: 15px; height: 15px; }
.syntax-trigger:hover { border-color: rgba(0, 212, 255, 0.4); color: var(--accent-cyan); }
.syntax-trigger.active {
  color: var(--accent-cyan);
  background: rgba(0, 212, 255, 0.08);
  border-color: rgba(0, 212, 255, 0.45);
  box-shadow: 0 0 16px rgba(0, 212, 255, 0.16);
}

/* ===== 语法弹层 ===== */
.syntax-pop {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  width: 440px;
  max-width: calc(100vw - 32px);
  z-index: 30;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.45), 0 0 0 1px rgba(0, 212, 255, 0.04);
  overflow: hidden;
  animation: pop-in 0.24s cubic-bezier(0.4, 0, 0.2, 1);
}
@keyframes pop-in {
  from { opacity: 0; transform: translateY(-6px); }
  to { opacity: 1; transform: translateY(0); }
}
.syntax-backdrop { position: fixed; inset: 0; z-index: 20; background: transparent; }

/* ===== 结果 ===== */
.results-meta { margin-bottom: 12px; }
.results-count { font-family: var(--font-heading); font-size: 11px; font-weight: 600; letter-spacing: 0.08em; color: var(--text-muted); text-transform: uppercase; }
.count-num { color: var(--accent-cyan); font-size: 14px; margin-right: 2px; }

.assets-table { border-radius: var(--radius-md); overflow: hidden; }
.cell-target { font-family: var(--font-mono); font-size: 13px; color: var(--text-primary); }
.cell-title { color: var(--text-secondary); }

.ipv6-badge {
  display: inline-block; padding: 2px 8px; border-radius: 3px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 700; letter-spacing: 0.06em;
  color: var(--text-muted); background: rgba(82,96,128,0.15); border: 1px solid var(--border-subtle);
}
.ipv6-badge.on { color: var(--accent-green); background: rgba(0,230,118,0.08); border-color: rgba(0,230,118,0.2); }

.detail-trigger {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 4px 10px; background: transparent; border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm); color: var(--text-muted);
  font-family: var(--font-heading); font-size: 10px; font-weight: 600; letter-spacing: 0.06em;
  cursor: pointer; transition: all 0.2s ease;
}
.detail-trigger svg { width: 12px; height: 12px; }
.detail-trigger:hover { border-color: var(--accent-cyan); color: var(--accent-cyan); background: rgba(0,212,255,0.06); }

/* ===== 主机详情对话框 ===== */
.host-dialog :deep(.el-dialog) { border: 1px solid var(--border-subtle); }
.sub-table { border-radius: var(--radius-md); overflow: hidden; margin-bottom: 4px; }

.tech-tags { display: flex; flex-wrap: wrap; gap: 4px; }
.tech-tag {
  padding: 1px 7px; border-radius: 3px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 600;
  color: var(--accent-violet); background: rgba(123,97,255,0.1); border: 1px solid rgba(123,97,255,0.15);
}
.sev-badge {
  display: inline-block; padding: 2px 10px; border-radius: 3px;
  font-family: var(--font-heading); font-size: 11px; font-weight: 700; letter-spacing: 0.04em;
}
.sev-high { color: var(--accent-red); background: rgba(255,71,87,0.12); border: 1px solid rgba(255,71,87,0.25); }
.sev-mid { color: var(--accent-amber); background: rgba(245,166,35,0.1); border: 1px solid rgba(245,166,35,0.2); }
.sev-low { color: var(--accent-green); background: rgba(0,230,118,0.08); border: 1px solid rgba(0,230,118,0.15); }

.text-muted { color: var(--text-muted); }

/* 小屏：搜索栏纵向堆叠 */
@media (max-width: 640px) {
  .search-bar { flex-wrap: wrap; padding: 12px; }
  .search-input-wrap { flex: 1 1 100%; }
  .search-btn { flex: 1 1 100%; }
  .syntax-trigger { flex: 1 1 100%; justify-content: center; }
  .syntax-pop { left: 12px; right: 12px; width: auto; }
}
</style>
