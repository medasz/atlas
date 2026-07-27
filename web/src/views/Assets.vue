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
            placeholder="Dork syntax · ip=&quot;1.1.1.1&quot; && port=&quot;443&quot;"
            @keyup.enter="doSearch"
            autofocus
          />
          <span v-if="q" class="search-clear" @click="q=''; doSearch()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </span>
        </div>
        <div class="type-select">
          <button v-for="opt in typeOpts" :key="opt.value"
            :class="{ active: type === opt.value }" @click="type = opt.value">
            {{ opt.label }}
          </button>
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
      </div>

      <!-- Dork 语法提示 -->
      <div class="hint-panel">
        <div class="hint-row">
          <span class="hint-label">FIELDS</span>
          <code>ip</code> <code>port</code> <code>protocol</code> <code>server</code>
          <code>banner</code> <code>title</code> <code>os</code> <code>org</code>
          <code>asn</code> <code>host</code> <code>domain</code> <code>app</code>
          <code>product</code> <code>body</code> <code>header</code> <code>country</code>
          <code>region</code> <code>city</code> <code>cert</code> <code>is_ipv6</code>
        </div>
        <div class="hint-row">
          <span class="hint-label">OPS</span>
          <code>=</code>包含 <code>==</code>精确 <code>!=</code>排除 <code>*=</code>模糊
          <span class="hint-sep"></span>
          <span class="hint-label">LOGIC</span>
          <code>&amp;&amp;</code>与 <code>||</code>或 <code>()</code>分组
          <span class="hint-sep"></span>
          <code>server="nginx" && (port="80" || port="443")</code>
        </div>
      </div>
    </div>

    <!-- 结果 -->
    <div class="results-section">
      <div class="results-meta">
        <span class="results-count" v-if="items.length">
          <span class="count-num">{{ items.length }}</span> results found
        </span>
      </div>

      <el-table :data="items" v-loading="loading" stripe class="assets-table">
        <el-table-column prop="doc_type" label="Type" width="90" />
        <el-table-column label="Target" width="220">
          <template #default="{ row }"><span class="cell-target">{{ row.name || row.ip || '-' }}</span></template>
        </el-table-column>
        <el-table-column prop="port" label="Port" width="80" />
        <el-table-column prop="service" label="Service" width="120" />
        <el-table-column prop="version" label="Version" width="130" />
        <el-table-column prop="title" label="Title" min-width="170" show-overflow-tooltip>
          <template #default="{ row }"><span class="cell-title">{{ row.title || '-' }}</span></template>
        </el-table-column>
        <el-table-column prop="server" label="Server" width="150" show-overflow-tooltip />
        <el-table-column v-if="type==='domain' || hasDomain" label="Root Domain" prop="registrable_domain" width="170" show-overflow-tooltip />
        <el-table-column prop="org" label="Org" width="150" show-overflow-tooltip />
        <el-table-column prop="asn" label="ASN" width="90" />
        <el-table-column label="IPv6" width="72" align="center">
          <template #default="{ row }">
            <span class="ipv6-badge" :class="{ on: row.is_ipv6 }">{{ row.is_ipv6 ? 'v6' : 'v4' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="80" align="center">
          <template #default="{ row }">
            <button v-if="row.doc_type==='host'" class="detail-trigger" @click="openHost(row.ip)">
              <span>Details</span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12,5 19,12 12,19"/></svg>
            </button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 主机详情对话框 -->
    <el-dialog v-model="hostVisible" width="800px" class="host-dialog">
      <template #header>
        <DialogHeader title="Host Detail" :icon="icons.host" />
      </template>
      <template v-if="detail">
        <DetailGrid :items="hostItems" />

        <SectionLabel label="PORTS · SERVICES · FINGERPRINTS" />
        <el-table :data="detail.ports" size="small" stripe class="sub-table">
          <el-table-column prop="port" label="Port" width="72" />
          <el-table-column prop="proto" label="Proto" width="70" />
          <el-table-column prop="service" label="Service" width="110" />
          <el-table-column prop="version" label="Version" width="120" />
          <el-table-column prop="title" label="Title" min-width="150" show-overflow-tooltip />
          <el-table-column label="Server" width="140" show-overflow-tooltip>
            <template #default="{ row }">{{ (row.webinfo && row.webinfo.server) || row.server || '—' }}</template>
          </el-table-column>
          <el-table-column label="Tech Stack" min-width="170" show-overflow-tooltip>
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

        <SectionLabel label="VULNERABILITIES" :count="(detail.vulns || []).length" />
        <el-table v-if="detail.vulns && detail.vulns.length" :data="detail.vulns" size="small" stripe class="sub-table">
          <el-table-column prop="name" label="Name" min-width="200" show-overflow-tooltip />
          <el-table-column prop="cve" label="CVE" width="150" />
          <el-table-column label="Severity" width="96" align="center">
            <template #default="{ row }">
              <span class="sev-badge" :class="'sev-' + sevClass(row.level)">{{ row.level }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="Status" width="96" align="center" />
          <el-table-column prop="asset_ref" label="Ref" width="170" show-overflow-tooltip />
        </el-table>
        <el-empty v-else description="No vulnerabilities" :image-size="48" />
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

const icons = {
  host: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="3"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></svg>'
}

const q = ref('')
const type = ref('')
const items = ref([])
const loading = ref(false)
const hostVisible = ref(false)
const detail = ref(null)

const typeOpts = [
  { label: 'ALL', value: '' },
  { label: 'HOST', value: 'host' },
  { label: 'PORT', value: 'port' },
  { label: 'DOMAIN', value: 'domain' }
]

const hasDomain = computed(() => items.value.some(r => r.doc_type === 'domain'))

const hostItems = computed(() => {
  if (!detail.value || !detail.value.host) return []
  const h = detail.value.host
  return [
    { key: 'IP', value: h.ip },
    { key: 'IPv6', value: h.is_ipv6 ? 'YES' : 'NO' },
    { key: 'Organization', value: h.org || '—' },
    { key: 'Operating System', value: h.os || '—' },
    { key: 'Open Ports', value: (h.open_ports || []).length, highlight: true }
  ]
})

async function doSearch() {
  loading.value = true
  try {
    const r = await api.searchAssets(q.value, type.value)
    items.value = r.items || []
  } finally {
    loading.value = false
  }
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
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
}
.search-bar { display: flex; align-items: center; gap: 0; padding: 16px 16px 0; }

.search-input-wrap { flex: 1; position: relative; }
.search-input-wrap input {
  width: 100%; padding: 11px 40px 11px 42px;
  background: var(--bg-input);
  border: 1px solid rgba(26,42,74,0.6);
  border-radius: var(--radius-md) 0 0 var(--radius-md);
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

.type-select { display: flex; border-top: 1px solid rgba(26,42,74,0.5); border-bottom: 1px solid rgba(26,42,74,0.5); background: var(--bg-input); }
.type-select button {
  padding: 11px 14px; background: transparent; border: none;
  color: var(--text-muted); font-family: var(--font-heading); font-size: 11px; font-weight: 600;
  letter-spacing: 0.08em; cursor: pointer; transition: all 0.2s ease; border-right: 1px solid rgba(26,42,74,0.5);
}
.type-select button:last-child { border-right: none; }
.type-select button:hover { color: var(--text-primary); background: var(--bg-hover); }
.type-select button.active { color: var(--accent-cyan); background: rgba(0,212,255,0.08); box-shadow: inset 0 -2px 0 var(--accent-cyan); }

.search-btn {
  display: flex; align-items: center; gap: 6px;
  padding: 11px 20px;
  background: linear-gradient(135deg, rgba(0,212,255,0.15) 0%, rgba(0,212,255,0.05) 100%);
  border: 1px solid rgba(0,212,255,0.28); border-left: none;
  border-radius: 0 var(--radius-md) var(--radius-md) 0;
  color: var(--accent-cyan);
  font-family: var(--font-heading); font-size: 13px; font-weight: 600;
  letter-spacing: 0.06em; cursor: pointer; transition: all 0.25s ease; white-space: nowrap;
}
.search-btn svg { width: 16px; height: 16px; }
.search-btn:hover { border-color: rgba(0,212,255,0.5); box-shadow: 0 0 20px rgba(0,212,255,0.18), inset 0 0 18px rgba(0,212,255,0.04); }
.search-btn.loading svg { animation: spin 1s linear infinite; }

/* ===== 语法提示 ===== */
.hint-panel { padding: 10px 16px 12px; border-top: 1px solid var(--border-subtle); background: rgba(6,11,20,0.5); display: flex; flex-direction: column; gap: 6px; }
.hint-row { display: flex; align-items: center; flex-wrap: wrap; gap: 4px; font-size: 11px; color: var(--text-secondary); }
.hint-label { font-family: var(--font-heading); font-size: 10px; font-weight: 700; letter-spacing: 0.1em; color: var(--text-muted); margin-right: 4px; user-select: none; }
.hint-row code { font-family: var(--font-mono); font-size: 11px; background: rgba(0,212,255,0.06); color: var(--accent-cyan); padding: 1px 6px; border-radius: 3px; border: 1px solid rgba(0,212,255,0.12); white-space: nowrap; }
.hint-sep { width: 1px; height: 12px; background: var(--border-subtle); margin: 0 8px; }

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
  .search-input-wrap input { border-radius: var(--radius-md); }
  .type-select { flex: 1 1 100%; border-radius: 0; }
  .search-btn { flex: 1 1 100%; border-radius: var(--radius-md); border-left: 1px solid rgba(0,212,255,0.28); }
}
</style>
