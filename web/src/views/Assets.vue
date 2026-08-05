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
            @keyup.enter="doSearch(true)"
            autofocus
          />
          <span v-if="q" class="search-clear" @click="q=''; doSearch(true)">
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
        <button class="agg-trigger" :class="{ active: isAggregated }" type="button"
          @click="toggleAggregated" title="切换 IP 聚合视图">
          <span class="st-ico" v-html="icons.agg"></span>
          <span class="st-label">IP 聚合</span>
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
        <span class="results-count" v-if="total">
          共 <span class="count-num">{{ total }}</span> 条 <span v-if="isAggregated">(已聚合)</span> · 第 {{ page }} / {{ Math.max(1, Math.ceil(total / pageSize)) }} 页
        </span>
      </div>

      <el-table :data="items" v-loading="loading" stripe class="assets-table">
        <el-table-column label="目标" width="180">
          <template #default="{ row }">
            <span class="cell-target">{{ (!row.ip && (row.domain || row.name || row.registrable_domain)) ? (row.domain || row.name || row.registrable_domain || '-') : (row.ip || '-') }}</span>
          </template>
        </el-table-column>
        <el-table-column label="域名" min-width="170" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ Array.isArray(row.domains) && row.domains.length ? row.domains.join(', ') : (row.domain || row.name || row.host || row.registrable_domain || '-') }}</span>
          </template>
        </el-table-column>
        <el-table-column label="端口" width="90" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ Array.isArray(row.open_ports) ? (row.open_ports.length ? row.open_ports.join(', ') : '-') : (row.port || '-') }}</span>
          </template>
        </el-table-column>
        <el-table-column label="协议" width="80">
          <template #default="{ row }">
            <span>{{ row.proto || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="端口状态" width="100" align="center">
          <template #default="{ row }">
            <StatusTag :text="portStatusText(row)" :tone="portStatusTone(row)" />
          </template>
        </el-table-column>
        <el-table-column label="服务" width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ Array.isArray(row.services) ? (row.services.length ? row.services.join(', ') : '-') : (row.service || '-') }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="110" />
        <el-table-column label="标题" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="cell-title">{{ Array.isArray(row.titles) ? (row.titles.length ? row.titles.join(' | ') : '-') : (row.title || '-') }}</span>
          </template>
        </el-table-column>
        <el-table-column label="服务(Web)" width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ (row.webinfo && row.webinfo.server) || row.server || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="IPv6" width="72" align="center">
          <template #default="{ row }">
            <span class="ipv6-badge" :class="{ on: row.is_ipv6 }">{{ row.is_ipv6 ? 'v6' : 'v4' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="92" align="center" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <button v-if="row.ip" class="detail-trigger" :title="detailButtonTitle(row)" :aria-label="detailButtonTitle(row)" @click="openAssetDetail(row)">
                <span>详情</span>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12,5 19,12 12,19"/></svg>
              </button>
              <button
                class="delete-trigger"
                :disabled="deletingKey === assetKey(row)"
                :title="deleteTitle(row)"
                :aria-label="deleteTitle(row)"
                @click="confirmDeleteAsset(row)"
              >
                <el-icon :class="{ 'is-loading': deletingKey === assetKey(row) }">
                  <Loading v-if="deletingKey === assetKey(row)" />
                  <DeleteIcon v-else />
                </el-icon>
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页控件 -->
      <div class="pager-wrap" v-if="total > pageSize">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next, jumper"
          :total="total"
          :page-size="pageSize"
          :current-page="page"
          :page-sizes="[10, 20, 50, 100]"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
    </div>

    <!-- 主机详情对话框 -->
    <el-dialog v-model="hostVisible" width="800px" class="host-dialog">
      <template #header>
        <DialogHeader :title="dialogTitle" :icon="icons.host" />
      </template>
      <template v-if="detail">
        <DetailGrid :items="detailItems" />

        <SectionLabel label="漏洞" :count="detailVulns.length" />
        <el-table v-if="detailVulns.length" :data="detailVulns" size="small" stripe class="sub-table">
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
import { ref, computed, onMounted } from 'vue'
import { Delete as DeleteIcon, Loading } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'
import DialogHeader from '../components/DialogHeader.vue'
import DetailGrid from '../components/DetailGrid.vue'
import SectionLabel from '../components/SectionLabel.vue'
import DorkCheatSheet from '../components/DorkCheatSheet.vue'
import StatusTag from '../components/StatusTag.vue'

const icons = {
  host: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="3"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></svg>',
  book: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>',
  agg: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="8" cy="6" r="2" fill="currentColor"/><circle cx="16" cy="12" r="2" fill="currentColor"/></svg>'
}

const q = ref('')
const items = ref([])
const loading = ref(false)
const hostVisible = ref(false)
const detail = ref(null)
const selectedAsset = ref(null)
const syntaxOpen = ref(false)
const isAggregated = ref(false)
const deletingKey = ref('')

// 分页状态
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const hasDomain = computed(() => items.value.some(r => !r.ip && (r.domain || r.name || r.registrable_domain)))

const dialogTitle = computed(() => selectedAsset.value?.port ? '端口详情' : '主机详情')

const detailVulns = computed(() => {
  const vulns = detail.value?.vulns || []
  const selected = selectedAsset.value
  if (!selected?.port) return vulns
  const assetRef = `${selected.ip}:${selected.port}`
  return vulns.filter(v => v.asset_ref === assetRef)
})

const detailItems = computed(() => {
  if (!detail.value || !detail.value.host) return []
  const h = detail.value.host
  const ports = detail.value.ports || []
  const selected = selectedAsset.value

  if (selected?.port) {
    const port = ports.find(p => Number(p.port) === Number(selected.port)) || selected
    return [
      { key: 'IP', value: port.ip || selected.ip },
      { key: '端口', value: port.port || selected.port, highlight: true },
      { key: '协议', value: port.proto || '-' },
      { key: '状态', value: portStatusText(port) },
      { key: '服务', value: port.service || '-' },
      { key: '版本', value: port.version || '-' },
      { key: '标题', value: port.title || '-' },
      { key: 'Web 服务', value: (port.webinfo && port.webinfo.server) || port.server || '-' },
      { key: 'Banner', value: port.banner || '-', mono: true, multiline: true }
    ]
  }

  // 汇总主机及端口下的技术栈/指纹
  const techSet = new Set()
  if (Array.isArray(h.tech)) h.tech.forEach(t => t && techSet.add(t))
  if (h.webinfo && Array.isArray(h.webinfo.tech)) h.webinfo.tech.forEach(t => t && techSet.add(t))
  ports.forEach(p => {
    techList(p).forEach(t => t && techSet.add(t))
  })
  const techStr = techSet.size > 0 ? Array.from(techSet).join(', ') : '-'
  const bannerStr = ports
    .filter(p => p.banner)
    .map(p => `${p.port}: ${p.banner}`)
    .join('\n') || '-'

  return [
    { key: 'IP', value: h.ip },
    { key: 'IPv6', value: h.is_ipv6 ? '是' : '否' },
    { key: '所属组织', value: h.org || '—' },
    { key: 'ASN', value: h.asn ? h.asn : '—' },
    { key: '操作系统', value: h.os || '-' },
    { key: '技术栈 / 指纹', value: techStr },
    { key: 'Banner', value: bannerStr, mono: true, multiline: true },
    { key: '开放端口', value: Array.isArray(h.open_ports) ? h.open_ports.length : (ports.length || (h.port ? 1 : 0)), highlight: true }
  ]
})

function toggleAggregated() {
  isAggregated.value = !isAggregated.value
  doSearch(true)
}

// reset=true 时回到首页（检索条件变化或清空）
async function doSearch(reset) {
  if (reset) page.value = 1
  loading.value = true
  try {
    const r = await api.searchAssets(q.value, '', page.value, pageSize.value, isAggregated.value)
    items.value = r.items || []
    total.value = r.total || 0
  } finally {
    loading.value = false
  }
}

function onPageChange(p) {
  page.value = p
  doSearch(false)
}

function onSizeChange(s) {
  pageSize.value = s
  page.value = 1
  doSearch(false)
}

// 语法手册示例 → 填入检索框并立即检索
function onApplyDork(query) {
  q.value = query
  doSearch()
}

function techOf(row) {
  const t = (row.webinfo && row.webinfo.tech) || row.tech
  if (Array.isArray(t)) return t.length ? t.join(', ') : '-'
  return t || '-'
}
function techList(row) {
  const t = (row.webinfo && row.webinfo.tech) || row.tech
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

function portStatusText(row) {
  if (row.aggregated) {
    return '开放'
  }
  const st = (row.state || row.status || (row.port ? 'open' : '')).toLowerCase()
  switch (st) {
    case 'open':
      return '开放'
    case 'closed':
      return '关闭'
    case 'filtered':
      return '被过滤'
    case 'timeout':
      return '超时'
    case 'open|filtered':
      return '开放/过滤'
    case 'unfiltered':
      return '未过滤'
    default:
      return st || '未知'
  }
}

function portStatusTone(row) {
  if (row.aggregated) {
    return 'green'
  }
  const st = (row.state || row.status || (row.port ? 'open' : '')).toLowerCase()
  switch (st) {
    case 'open':
      return 'green'
    case 'closed':
      return 'red'
    case 'filtered':
    case 'timeout':
    case 'open|filtered':
    case 'unfiltered':
      return 'amber'
    default:
      return 'muted'
  }
}

function detailButtonTitle(row) {
  return row.port ? '查看端口详情' : '查看主机详情'
}

async function openAssetDetail(row) {
  const r = await api.getHostDetail(row.ip)
  selectedAsset.value = row
  detail.value = r
  hostVisible.value = true
}

function assetKey(row) {
  if (row.aggregated || (row.ip && !row.port)) return `host:${row.ip}`
  if (row.ip && row.port) return `port:${row.ip}:${row.port}`
  return `domain:${row.domain || row.name || row.registrable_domain || ''}`
}

function deleteTitle(row) {
  return row.aggregated || (row.ip && !row.port) ? '删除主机及全部端口资产' : '删除资产'
}

async function confirmDeleteAsset(row) {
  const hostDelete = row.aggregated || (row.ip && !row.port)
  const domain = row.domain || row.name || row.registrable_domain || ''
  const target = hostDelete
    ? `主机 ${row.ip} 及其全部端口资产`
    : row.ip && row.port
      ? `端口资产 ${row.ip}:${row.port}`
      : `域名资产 ${domain}`

  try {
    await ElMessageBox.confirm(
      `确定删除${target}吗？此操作不会删除历史扫描任务。`,
      '删除资产',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'confirm-danger' }
    )
  } catch {
    return
  }

  const key = assetKey(row)
  deletingKey.value = key
  try {
    if (hostDelete) {
      await api.deleteHostAssets(row.ip)
    } else {
      await api.deleteAsset({ ip: row.ip, port: row.port, domain: row.ip ? '' : domain })
    }
    if (hostVisible.value && detail.value?.host?.ip === row.ip) {
      hostVisible.value = false
      detail.value = null
      selectedAsset.value = null
    }
    if (items.value.length === 1 && page.value > 1) page.value--
    await doSearch(false)
    ElMessage.success('资产已删除')
  } catch (err) {
    ElMessage.error(`删除失败：${err.message}`)
  } finally {
    deletingKey.value = ''
  }
}

onMounted(() => doSearch(true))
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

/* ===== 语法/聚合 按钮 ===== */
.agg-trigger,
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
.agg-trigger svg,
.syntax-trigger svg { width: 15px; height: 15px; }
.agg-trigger:hover,
.syntax-trigger:hover { border-color: rgba(0, 212, 255, 0.4); color: var(--accent-cyan); }
.agg-trigger.active,
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

/* ===== 分页控件（贴合暗色赛博主题） ===== */
.pager-wrap {
  display: flex; justify-content: flex-end;
  margin-top: 16px; padding: 12px 4px;
}
.pager-wrap :deep(.el-pagination) {
  color: var(--text-secondary);
  font-family: var(--font-body);
}
.pager-wrap :deep(.el-pagination__total),
.pager-wrap :deep(.el-pagination__jump) {
  color: var(--text-muted);
}
.pager-wrap :deep(.el-pagination button),
.pager-wrap :deep(.el-pager li) {
  background: var(--bg-input);
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  transition: all 0.2s ease;
}
.pager-wrap :deep(.el-pagination button:hover),
.pager-wrap :deep(.el-pager li:hover) {
  color: var(--accent-cyan);
  border-color: rgba(0, 212, 255, 0.3);
}
.pager-wrap :deep(.el-pager li.is-active) {
  background: linear-gradient(135deg, rgba(0,212,255,0.18), rgba(0,212,255,0.06));
  color: var(--accent-cyan);
  border-color: rgba(0,212,255,0.45);
  box-shadow: 0 0 14px rgba(0,212,255,0.18);
  font-weight: 600;
}
.pager-wrap :deep(.el-pagination .el-select .el-input__wrapper),
.pager-wrap :deep(.el-pagination button:disabled) {
  background: var(--bg-input);
}
.pager-wrap :deep(.el-pagination .el-select .el-input__inner) { color: var(--text-secondary); }

.cell-target { font-family: var(--font-mono); font-size: 13px; color: var(--text-primary); }
.cell-title { color: var(--text-secondary); }

.ipv6-badge {
  display: inline-block; padding: 2px 8px; border-radius: 3px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 700; letter-spacing: 0.06em;
  color: var(--text-muted); background: rgba(82,96,128,0.15); border: 1px solid var(--border-subtle);
}
.ipv6-badge.on { color: var(--accent-green); background: rgba(0,230,118,0.08); border-color: rgba(0,230,118,0.2); }

.detail-trigger {
  display: inline-flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; padding: 0; background: transparent; border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm); color: var(--text-muted);
  cursor: pointer; transition: all 0.2s ease;
}
.detail-trigger span { display: none; }
.detail-trigger svg { width: 12px; height: 12px; }
.detail-trigger:hover { border-color: var(--accent-cyan); color: var(--accent-cyan); background: rgba(0,212,255,0.06); }

.row-actions { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-width: 62px; }
.delete-trigger {
  display: inline-grid; place-items: center; width: 28px; height: 28px; padding: 0;
  background: transparent; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm);
  color: var(--text-muted); cursor: pointer; transition: all 0.2s ease;
}
.delete-trigger .el-icon { width: 14px; height: 14px; font-size: 14px; }
.delete-trigger:hover:not(:disabled) {
  color: var(--accent-red); border-color: rgba(255,71,87,0.45); background: rgba(255,71,87,0.08);
}
.delete-trigger:disabled { cursor: wait; opacity: 0.65; }

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
