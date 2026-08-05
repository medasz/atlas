<template>
  <div class="ip-aggregate-page">
    <div class="page-toolbar">
      <button class="back-button" type="button" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>
        返回资产检索
      </button>
      <button class="refresh-button" type="button" :disabled="loading" @click="load">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23,4 23,10 17,10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
        刷新
      </button>
    </div>

    <section class="hero-panel">
      <div>
        <p class="eyebrow">IP 聚合视图</p>
        <h1>{{ ip }}</h1>
        <p class="subtitle">按端口展示真实的最近观测状态，不将 ACK 未过滤误判为开放。</p>
      </div>
      <span class="ipv6-badge" :class="{ on: detail?.host?.is_ipv6 }">{{ detail?.host?.is_ipv6 ? 'IPv6' : 'IPv4' }}</span>
    </section>

    <div v-if="error" class="error-panel">{{ error }}</div>

    <template v-else>
      <section class="metric-grid" v-loading="loading">
        <div class="metric-card"><span>已观测端口</span><strong>{{ ports.length }}</strong></div>
        <div class="metric-card open"><span>实际开放</span><strong>{{ stateCounts.open || 0 }}</strong></div>
        <div class="metric-card"><span>未过滤（ACK）</span><strong>{{ stateCounts.unfiltered || 0 }}</strong></div>
        <div class="metric-card"><span>过滤 / 超时</span><strong>{{ filteredOrTimeout }}</strong></div>
      </section>

      <section class="ports-panel" v-loading="loading">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">端口观测</p>
            <h2>端口与服务</h2>
          </div>
          <span class="state-summary" v-if="stateSummary">{{ stateSummary }}</span>
        </div>
        <div class="table-scroll">
          <el-table :data="ports" stripe class="ports-table" empty-text="该 IP 暂无端口观测记录">
            <el-table-column prop="port" label="端口" width="90">
              <template #default="{ row }"><span class="port-number">{{ row.port }}</span></template>
            </el-table-column>
            <el-table-column label="协议" width="90"><template #default="{ row }">{{ row.proto || '-' }}</template></el-table-column>
            <el-table-column label="状态" width="150" align="center">
              <template #default="{ row }"><StatusTag :text="stateText(row.state)" :tone="stateTone(row.state)" /></template>
            </el-table-column>
            <el-table-column label="服务" min-width="140"><template #default="{ row }">{{ row.service || '-' }}</template></el-table-column>
            <el-table-column label="版本" min-width="130"><template #default="{ row }">{{ row.version || '-' }}</template></el-table-column>
            <el-table-column label="Banner" min-width="260" show-overflow-tooltip>
              <template #default="{ row }"><span class="banner-cell">{{ row.banner || '-' }}</span></template>
            </el-table-column>
            <el-table-column label="最近观测" min-width="180"><template #default="{ row }">{{ formatTime(row.last_seen) }}</template></el-table-column>
          </el-table>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import StatusTag from '../components/StatusTag.vue'

const props = defineProps({ ip: { type: String, required: true } })
const router = useRouter()
const detail = ref(null)
const loading = ref(false)
const error = ref('')

const ports = computed(() => [...(detail.value?.ports || [])].sort((a, b) => Number(a.port) - Number(b.port)))
const stateCounts = computed(() => ports.value.reduce((counts, port) => {
  const state = String(port.state || 'unknown').toLowerCase()
  counts[state] = (counts[state] || 0) + 1
  return counts
}, {}))
const filteredOrTimeout = computed(() => (stateCounts.value.filtered || 0) + (stateCounts.value.timeout || 0) + (stateCounts.value['open|filtered'] || 0))
const stateSummary = computed(() => Object.entries(stateCounts.value)
  .sort(([left], [right]) => left.localeCompare(right))
  .map(([state, count]) => `${stateText(state)}: ${count}`)
  .join(' · '))

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

async function load() {
  loading.value = true
  error.value = ''
  try {
    detail.value = await api.getHostDetail(props.ip)
  } catch (err) {
    detail.value = null
    error.value = `加载 IP 聚合信息失败：${err.message}`
  } finally {
    loading.value = false
  }
}

function goBack() {
  router.push('/assets')
}

watch(() => props.ip, load, { immediate: true })
</script>

<style scoped>
.ip-aggregate-page { display: flex; flex-direction: column; gap: 20px; }
.page-toolbar { display: flex; align-items: center; justify-content: space-between; }
.back-button, .refresh-button { display: inline-flex; align-items: center; gap: 7px; padding: 9px 13px; border-radius: var(--radius-md); font-family: var(--font-heading); font-size: 12px; cursor: pointer; transition: all .2s ease; }
.back-button { background: transparent; border: 1px solid var(--border-subtle); color: var(--text-secondary); }
.back-button:hover { border-color: var(--accent-cyan); color: var(--accent-cyan); }
.refresh-button { background: rgba(0,212,255,.08); border: 1px solid rgba(0,212,255,.28); color: var(--accent-cyan); }
.refresh-button:disabled { opacity: .55; cursor: wait; }
.page-toolbar svg { width: 15px; height: 15px; }
.hero-panel, .ports-panel, .metric-card, .error-panel { border: 1px solid var(--border-subtle); background: var(--bg-surface); border-radius: var(--radius-lg); }
.hero-panel { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 26px 28px; background: linear-gradient(120deg, rgba(0,212,255,.08), rgba(123,97,255,.04)), var(--bg-surface); }
.eyebrow { margin: 0 0 6px; color: var(--accent-cyan); font-family: var(--font-heading); font-size: 11px; letter-spacing: .12em; text-transform: uppercase; }
h1, h2 { margin: 0; font-family: var(--font-heading); color: var(--text-primary); }
h1 { font-size: 28px; letter-spacing: .03em; }
h2 { font-size: 18px; }
.subtitle { margin: 9px 0 0; color: var(--text-secondary); font-size: 13px; }
.ipv6-badge { padding: 4px 9px; border-radius: 4px; border: 1px solid var(--border-subtle); color: var(--text-muted); font-family: var(--font-heading); font-size: 11px; }
.ipv6-badge.on { color: var(--accent-green); border-color: rgba(0,230,118,.25); background: rgba(0,230,118,.07); }
.metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.metric-card { padding: 18px; display: flex; flex-direction: column; gap: 7px; }
.metric-card span { color: var(--text-muted); font-size: 12px; }
.metric-card strong { color: var(--text-primary); font-family: var(--font-heading); font-size: 28px; }
.metric-card.open strong { color: var(--accent-green); }
.ports-panel { padding: 20px; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; gap: 16px; }
.state-summary { color: var(--text-secondary); font-size: 12px; text-align: right; }
.ports-table { border-radius: var(--radius-md); overflow: hidden; }
.port-number, .banner-cell { font-family: var(--font-mono); }
.port-number { color: var(--accent-cyan); font-weight: 600; }
.banner-cell { font-size: 11px; color: var(--text-secondary); }
.error-panel { padding: 18px; border-color: rgba(255,71,87,.35); color: var(--accent-red); }
@media (max-width: 780px) { .metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .hero-panel { padding: 20px; } .state-summary { display: none; } }
</style>
