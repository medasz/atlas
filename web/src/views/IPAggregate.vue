<template>
  <div class="ip-aggregate-page">
    <div class="page-toolbar">
      <button class="back-button" type="button" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>
        返回资产检索
      </button>
      <div class="toolbar-actions">
        <button class="refresh-button" type="button" :disabled="loading || deleting" @click="load">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23,4 23,10 17,10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
          刷新
        </button>
        <button class="delete-button" type="button" :disabled="loading || deleting" @click="confirmDelete">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3,6 5,6 21,6"/><path d="M19,6v14a2,2 0 0 1-2,2H7a2,2 0 0 1-2-2V6m3,0V4a2,2 0 0 1 2-2h4a2,2 0 0 1 2,2v2m-6,5v6m4-6v6"/></svg>
          {{ deleting ? '删除中…' : '删除当前 IP 资产' }}
        </button>
      </div>
    </div>

    <section class="hero-panel">
      <div>
        <p class="eyebrow">IP 聚合视图</p>
        <h1>{{ ip }}</h1>
        <p class="subtitle">端口列表按页加载；指纹、Banner 和证书仅在打开端口详情时获取。</p>
      </div>
      <span class="ipv6-badge" :class="{ on: aggregate?.host?.is_ipv6 }">{{ aggregate?.host?.is_ipv6 ? 'IPv6' : 'IPv4' }}</span>
    </section>

    <div v-if="error" class="error-panel">{{ error }}</div>

    <template v-else>
      <section class="metric-grid" v-loading="summaryLoading">
        <div class="metric-card"><span>已观测端口</span><strong>{{ aggregate?.total || 0 }}</strong></div>
        <div class="metric-card open"><span>实际开放</span><strong>{{ stateCounts.open || 0 }}</strong></div>
        <div class="metric-card"><span>未过滤（ACK）</span><strong>{{ stateCounts.unfiltered || 0 }}</strong></div>
        <div class="metric-card"><span>过滤 / 超时</span><strong>{{ filteredOrTimeout }}</strong></div>
      </section>

      <section class="ports-panel" v-loading="portsLoading">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">端口观测</p>
            <h2>端口与服务</h2>
          </div>
          <span class="state-summary" v-if="stateSummary">{{ stateSummary }}</span>
        </div>
        <div class="filters">
          <el-select v-model="stateFilter" class="state-filter" clearable placeholder="全部状态" @change="reloadFirstPage">
            <el-option v-for="item in stateOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="sortOrder" class="sort-filter" @change="reloadFirstPage">
            <el-option label="端口：从小到大" value="port_asc" />
            <el-option label="端口：从大到小" value="port_desc" />
          </el-select>
          <span class="page-total">当前筛选 {{ portPage.total || 0 }} 条</span>
        </div>
        <div class="table-scroll">
          <el-table :data="portPage.items" stripe class="ports-table" empty-text="该 IP 暂无端口观测记录">
            <el-table-column prop="port" label="端口" width="90">
              <template #default="{ row }"><span class="port-number">{{ row.port }}</span></template>
            </el-table-column>
            <el-table-column label="协议" width="90"><template #default="{ row }">{{ row.proto || '-' }}</template></el-table-column>
            <el-table-column label="状态" width="150" align="center">
              <template #default="{ row }"><StatusTag :text="stateText(row.state)" :tone="stateTone(row.state)" /></template>
            </el-table-column>
            <el-table-column label="服务" min-width="150"><template #default="{ row }">{{ row.service || '-' }}</template></el-table-column>
            <el-table-column label="版本" min-width="170"><template #default="{ row }">{{ row.version || '-' }}</template></el-table-column>
            <el-table-column label="最近观测" min-width="180"><template #default="{ row }">{{ formatTime(row.last_seen) }}</template></el-table-column>
            <el-table-column label="操作" width="100" fixed="right">
              <template #default="{ row }"><el-button link type="primary" @click="openPortDetail(row)">详情</el-button></template>
            </el-table-column>
          </el-table>
        </div>
        <div class="pager">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="portPage.total || 0"
            :page-sizes="[50, 100, 200]"
            layout="total, sizes, prev, pager, next"
            @current-change="loadPorts"
            @size-change="reloadFirstPage"
          />
        </div>
      </section>
    </template>

    <el-dialog v-model="detailVisible" title="端口详情" width="min(760px, calc(100vw - 32px))" destroy-on-close>
      <div v-loading="portDetailLoading" class="port-detail">
        <template v-if="selectedPort">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="端口">{{ selectedPort.port }}</el-descriptions-item>
            <el-descriptions-item label="协议">{{ selectedPort.proto || '-' }}</el-descriptions-item>
            <el-descriptions-item label="状态">{{ stateText(selectedPort.state) }}</el-descriptions-item>
            <el-descriptions-item label="服务">{{ selectedPort.service || '-' }}</el-descriptions-item>
            <el-descriptions-item label="版本" :span="2">{{ selectedPort.version || '-' }}</el-descriptions-item>
            <el-descriptions-item label="Server" :span="2">{{ selectedPort.server || '-' }}</el-descriptions-item>
            <el-descriptions-item label="页面标题" :span="2">{{ selectedPort.title || '-' }}</el-descriptions-item>
            <el-descriptions-item label="最近观测" :span="2">{{ formatTime(selectedPort.last_seen) }}</el-descriptions-item>
          </el-descriptions>
          <div class="detail-section"><h3>Banner</h3><pre>{{ selectedPort.banner || '-' }}</pre></div>
          <div v-if="selectedPort.cert" class="detail-section"><h3>证书</h3><pre>{{ pretty(selectedPort.cert) }}</pre></div>
          <div v-if="selectedPort.webinfo" class="detail-section"><h3>Web 信息</h3><pre>{{ pretty(selectedPort.webinfo) }}</pre></div>
        </template>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '../api'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ ip: { type: String, required: true } })
const router = useRouter()
const aggregate = ref(null)
const portPage = ref({ items: [], total: 0, state_counts: {} })
const loading = ref(false)
const summaryLoading = ref(false)
const portsLoading = ref(false)
const deleting = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = ref(50)
const stateFilter = ref('')
const sortOrder = ref('port_asc')
const detailVisible = ref(false)
const portDetailLoading = ref(false)
const selectedPort = ref(null)

const stateCounts = computed(() => aggregate.value?.state_counts || {})
const filteredOrTimeout = computed(() => (stateCounts.value.filtered || 0) + (stateCounts.value.timeout || 0) + (stateCounts.value['open|filtered'] || 0))
const stateSummary = computed(() => Object.entries(stateCounts.value)
  .sort(([left], [right]) => left.localeCompare(right))
  .map(([state, count]) => `${stateText(state)}: ${count}`)
  .join(' · '))
const stateOptions = computed(() => Object.keys(stateCounts.value)
  .sort()
  .map(value => ({ value, label: `${stateText(value)} (${stateCounts.value[value]})` })))

function stateText(value) {
  switch (String(value || '').toLowerCase()) {
    case 'open': return '开放'
    case 'closed': return '关闭'
    case 'filtered': return '被过滤'
    case 'timeout': return '超时'
    case 'open|filtered': return '开放或过滤'
    case 'unfiltered': return '未过滤（ACK）'
    default: return value || '未知'
  }
}

function stateTone(value) {
  switch (String(value || '').toLowerCase()) {
    case 'open': return 'green'
    case 'closed': return 'red'
    case 'filtered':
    case 'timeout':
    case 'open|filtered':
    case 'unfiltered': return 'amber'
    default: return 'muted'
  }
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

function pretty(value) {
  return JSON.stringify(value, null, 2)
}

async function loadSummary() {
  summaryLoading.value = true
  try {
    aggregate.value = await api.getHostAggregate(props.ip)
  } finally {
    summaryLoading.value = false
  }
}

async function loadPorts() {
  portsLoading.value = true
  try {
    portPage.value = await api.listHostPorts(props.ip, page.value, pageSize.value, stateFilter.value, sortOrder.value)
  } finally {
    portsLoading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    await Promise.all([loadSummary(), loadPorts()])
  } catch (err) {
    aggregate.value = null
    portPage.value = { items: [], total: 0, state_counts: {} }
    error.value = `加载 IP 聚合信息失败：${err.message}`
  } finally {
    loading.value = false
  }
}

function reloadFirstPage() {
  page.value = 1
  loadPorts()
}

async function openPortDetail(row) {
  detailVisible.value = true
  selectedPort.value = null
  portDetailLoading.value = true
  try {
    const result = await api.getHostPort(props.ip, row.port)
    selectedPort.value = result.port
  } catch (err) {
    detailVisible.value = false
    ElMessage.error(`加载端口详情失败：${err.message}`)
  } finally {
    portDetailLoading.value = false
  }
}

function goBack() {
  router.push('/assets')
}

async function confirmDelete() {
  try {
    await ElMessageBox.confirm(
      `确定删除 IP ${props.ip} 的全部关联资产吗？端口资产、关联漏洞和资产历史将被删除，此操作不可恢复。`,
      '删除 IP 资产',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning', confirmButtonClass: 'confirm-danger' }
    )
  } catch {
    return
  }
  deleting.value = true
  try {
    await api.deleteHostAssets(props.ip)
    ElMessage.success(`已删除 ${props.ip} 的关联资产`)
    router.push('/assets')
  } catch (err) {
    ElMessage.error(`删除失败：${err.message}`)
  } finally {
    deleting.value = false
  }
}

watch(() => props.ip, () => {
  page.value = 1
  stateFilter.value = ''
  load()
}, { immediate: true })
</script>

<style scoped>
.ip-aggregate-page { display: flex; flex-direction: column; gap: 20px; }
.page-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.toolbar-actions { display: inline-flex; align-items: center; gap: 8px; }
.back-button, .refresh-button, .delete-button { display: inline-flex; align-items: center; gap: 7px; padding: 9px 13px; border-radius: var(--radius-md); font-family: var(--font-heading); font-size: 12px; cursor: pointer; transition: all .2s ease; }
.back-button { background: transparent; border: 1px solid var(--border-subtle); color: var(--text-secondary); }
.back-button:hover { border-color: var(--accent-cyan); color: var(--accent-cyan); }
.refresh-button { background: rgba(0,212,255,.08); border: 1px solid rgba(0,212,255,.28); color: var(--accent-cyan); }
.delete-button { background: rgba(255,71,87,.08); border: 1px solid rgba(255,71,87,.3); color: var(--accent-red); }
.delete-button:hover:not(:disabled) { background: rgba(255,71,87,.15); border-color: rgba(255,71,87,.55); }
.refresh-button:disabled, .delete-button:disabled { opacity: .55; cursor: wait; }
.page-toolbar svg { width: 15px; height: 15px; }
.hero-panel, .ports-panel, .metric-card, .error-panel { border: 1px solid var(--border-subtle); background: var(--bg-surface); border-radius: var(--radius-lg); }
.hero-panel { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 26px 28px; background: linear-gradient(120deg, rgba(0,212,255,.08), rgba(123,97,255,.04)), var(--bg-surface); }
.eyebrow { margin: 0 0 6px; color: var(--accent-cyan); font-family: var(--font-heading); font-size: 11px; letter-spacing: .12em; text-transform: uppercase; }
h1, h2, h3 { margin: 0; font-family: var(--font-heading); color: var(--text-primary); }
h1 { font-size: 28px; letter-spacing: .03em; } h2 { font-size: 18px; } h3 { font-size: 13px; }
.subtitle { margin: 9px 0 0; color: var(--text-secondary); font-size: 13px; }
.ipv6-badge { padding: 4px 9px; border-radius: 4px; border: 1px solid var(--border-subtle); color: var(--text-muted); font-family: var(--font-heading); font-size: 11px; }
.ipv6-badge.on { color: var(--accent-green); border-color: rgba(0,230,118,.25); background: rgba(0,230,118,.07); }
.metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.metric-card { padding: 18px; display: flex; flex-direction: column; gap: 7px; }.metric-card span { color: var(--text-muted); font-size: 12px; }.metric-card strong { color: var(--text-primary); font-family: var(--font-heading); font-size: 28px; }.metric-card.open strong { color: var(--accent-green); }
.ports-panel { padding: 20px; }.panel-heading, .filters, .pager { display: flex; align-items: center; justify-content: space-between; gap: 16px; }.panel-heading { margin-bottom: 16px; }.filters { justify-content: flex-start; margin-bottom: 14px; }.state-filter, .sort-filter { width: 165px; }.page-total, .state-summary { color: var(--text-secondary); font-size: 12px; }.state-summary { text-align: right; }
.table-scroll { overflow-x: auto; }.ports-table { min-width: 820px; border-radius: var(--radius-md); overflow: hidden; }.port-number { font-family: var(--font-mono); color: var(--accent-cyan); font-weight: 600; }.pager { justify-content: flex-end; margin-top: 16px; }.error-panel { padding: 18px; border-color: rgba(255,71,87,.35); color: var(--accent-red); }
.port-detail { min-height: 100px; }.detail-section { margin-top: 18px; }.detail-section h3 { margin-bottom: 8px; }.detail-section pre { max-height: 250px; margin: 0; padding: 12px; overflow: auto; white-space: pre-wrap; word-break: break-word; border: 1px solid var(--border-subtle); border-radius: var(--radius-md); background: var(--bg-base); color: var(--text-secondary); font-family: var(--font-mono); font-size: 12px; }
@media (max-width: 780px) { .page-toolbar { align-items: flex-start; flex-direction: column; }.metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }.hero-panel { padding: 20px; }.state-summary { display: none; }.filters { align-items: stretch; flex-direction: column; }.state-filter, .sort-filter { width: 100%; }.pager { justify-content: center; } }
</style>
