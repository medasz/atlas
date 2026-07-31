<template>
  <div class="audit-page">
    <!-- 顶部 Summary Cards -->
    <div class="summary-cards">
      <div class="card stat-card">
        <div class="card-icon cyan">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
            <polyline points="14,2 14,8 20,8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
          </svg>
        </div>
        <div class="card-info">
          <div class="card-title">审计日志总数</div>
          <div class="card-value">{{ total }}</div>
        </div>
      </div>

      <div class="card stat-card">
        <div class="card-icon emerald">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
            <polyline points="22 4 12 14.01 9 11.01"/>
          </svg>
        </div>
        <div class="card-info">
          <div class="card-title">审计状态</div>
          <div class="card-value status-badge" :class="auditEnabled ? 'enabled' : 'disabled'">
            {{ auditEnabled ? '已开启记录' : '已关闭' }}
          </div>
        </div>
      </div>
    </div>

    <!-- 搜索与控制面板 -->
    <div class="panel toolbar-panel">
      <div class="search-box">
        <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索操作人、目标 IP / 域名、动作或任务 ID..."
          @keyup.enter="onSearch"
        />
        <button v-if="searchQuery" class="clear-btn" @click="clearSearch">×</button>
      </div>

      <div class="filter-actions">
        <button class="btn btn-secondary" @click="fetchLogs" :disabled="loading">
          <svg class="icon-spin" v-if="loading" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"/>
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23 4 23 10 17 10"/>
            <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
          </svg>
          刷新
        </button>
      </div>
    </div>

    <!-- 日志列表面板 -->
    <div class="panel table-panel">
      <div class="table-responsive">
        <table class="data-table">
          <thead>
            <tr>
              <th width="180">时间</th>
              <th width="140">操作人</th>
              <th width="160">动作类型</th>
              <th>目标资产 / 参数</th>
              <th width="200">关联 Task ID</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="5" class="text-center loading-cell">
                <div class="spinner"></div>
                加载审计日志中...
              </td>
            </tr>
            <tr v-else-if="logs.length === 0">
              <td colspan="5" class="text-center empty-cell">
                暂无审计日志记录
              </td>
            </tr>
            <tr v-for="item in logs" :key="item.id" class="log-row">
              <td class="time-col">
                <svg class="time-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="10"/>
                  <polyline points="12 6 12 12 16 14"/>
                </svg>
                <span>{{ formatTime(item.time) }}</span>
              </td>
              <td>
                <span class="operator-badge" :class="item.operator === 'admin' ? 'admin' : 'system'">
                  {{ item.operator || 'system' }}
                </span>
              </td>
              <td>
                <span class="action-badge" :class="getActionClass(item.action)">
                  {{ item.action }}
                </span>
              </td>
              <td class="target-col">
                <code v-if="item.target" class="target-code">{{ item.target }}</code>
                <span v-else class="text-muted">-</span>
              </td>
              <td class="task-col">
                <code v-if="item.task_id" class="task-code">{{ item.task_id }}</code>
                <span v-else class="text-muted">-</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页控制 -->
      <div class="pagination-bar" v-if="totalPages > 1">
        <div class="page-info">共 {{ total }} 条记录，第 {{ page }} / {{ totalPages }} 页</div>
        <div class="page-buttons">
          <button class="btn btn-sm btn-secondary" :disabled="page <= 1" @click="changePage(page - 1)">
            上一页
          </button>
          <span class="current-page">{{ page }}</span>
          <button class="btn btn-sm btn-secondary" :disabled="page >= totalPages" @click="changePage(page + 1)">
            下一页
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const logs = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const totalPages = ref(1)
const searchQuery = ref('')
const loading = ref(false)
const auditEnabled = ref(true)

async function fetchLogs() {
  loading.value = true
  try {
    const res = await api.getAuditLogs(searchQuery.value, page.value, pageSize.value)
    logs.value = res.items || []
    total.value = res.total || 0
    totalPages.value = res.total_pages || 1
  } catch (err) {
    console.error('加载审计日志失败:', err)
  } finally {
    loading.value = false
  }
}

async function fetchAuditStatus() {
  try {
    const cfg = await api.getConfig()
    if (cfg && cfg.audit) {
      auditEnabled.value = !!cfg.audit.enabled
    }
  } catch (e) {
    // 忽略错误
  }
}

function onSearch() {
  page.value = 1
  fetchLogs()
}

function clearSearch() {
  searchQuery.value = ''
  onSearch()
}

function changePage(p) {
  if (p < 1 || p > totalPages.value) return
  page.value = p
  fetchLogs()
}

function formatTime(isoStr) {
  if (!isoStr) return '-'
  try {
    const d = new Date(isoStr)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch (e) {
    return isoStr
  }
}

function getActionClass(action) {
  if (!action) return 'default'
  const act = action.toLowerCase()
  if (act.includes('create') || act.includes('add')) return 'success'
  if (act.includes('delete') || act.includes('remove') || act.includes('del')) return 'danger'
  if (act.includes('update') || act.includes('pause') || act.includes('resume')) return 'warning'
  return 'info'
}

onMounted(() => {
  fetchLogs()
  fetchAuditStatus()
})
</script>

<style scoped>
.audit-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 顶部统计卡片 */
.summary-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.card {
  background: var(--bg-surface, #1e2029);
  border: 1px solid var(--border-color, #2a2d3d);
  border-radius: 12px;
  padding: 16px 20px;
  display: flex;
  align-items: center;
  gap: 16px;
}

.card-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.card-icon.cyan {
  background: rgba(6, 182, 212, 0.12);
  color: #06b6d4;
}

.card-icon.emerald {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}

.card-icon svg {
  width: 22px;
  height: 22px;
}

.card-title {
  font-size: 13px;
  color: var(--text-muted, #8b8d9b);
  margin-bottom: 4px;
}

.card-value {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-heading, #f3f4f6);
}

.status-badge.enabled {
  color: #10b981;
  font-size: 15px;
}
.status-badge.disabled {
  color: #ef4444;
  font-size: 15px;
}

/* 工具栏面板 */
.panel {
  background: var(--bg-surface, #1e2029);
  border: 1px solid var(--border-color, #2a2d3d);
  border-radius: 12px;
}

.toolbar-panel {
  padding: 14px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.search-box {
  position: relative;
  flex: 1;
  min-width: 280px;
}

.search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  width: 16px;
  height: 16px;
  color: var(--text-muted, #8b8d9b);
}

.search-box input {
  width: 100%;
  padding: 9px 36px 9px 36px;
  background: var(--bg-dark, #14151b);
  border: 1px solid var(--border-color, #2a2d3d);
  border-radius: 8px;
  color: var(--text-primary, #e2e8f0);
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s;
}

.search-box input:focus {
  border-color: var(--color-primary, #6366f1);
}

.clear-btn {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--text-muted, #8b8d9b);
  font-size: 18px;
  cursor: pointer;
}

/* 数据表格 */
.table-panel {
  padding: 0;
  overflow: hidden;
}

.table-responsive {
  width: 100%;
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
  font-size: 14px;
}

.data-table th {
  background: rgba(255, 255, 255, 0.02);
  color: var(--text-muted, #8b8d9b);
  font-weight: 600;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-color, #2a2d3d);
}

.data-table td {
  padding: 13px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  color: var(--text-primary, #e2e8f0);
}

.log-row:hover td {
  background: rgba(255, 255, 255, 0.015);
}

.time-col {
  display: flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-muted, #9ca3af);
}

.time-icon {
  width: 14px;
  height: 14px;
  opacity: 0.6;
}

.operator-badge {
  display: inline-flex;
  padding: 2px 10px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
}

.operator-badge.admin {
  background: rgba(99, 102, 241, 0.15);
  color: #818cf8;
  border: 1px solid rgba(99, 102, 241, 0.3);
}

.operator-badge.system {
  background: rgba(156, 163, 175, 0.15);
  color: #d1d5db;
  border: 1px solid rgba(156, 163, 175, 0.3);
}

.action-badge {
  display: inline-flex;
  padding: 2px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.2px;
}

.action-badge.success {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}

.action-badge.warning {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
}

.action-badge.danger {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.action-badge.info {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
}

.action-badge.default {
  background: rgba(255, 255, 255, 0.08);
  color: #9ca3af;
}

.target-code, .task-code {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  background: rgba(0, 0, 0, 0.25);
  padding: 3px 8px;
  border-radius: 4px;
  color: #a7f3d0;
}

.task-code {
  color: #bae6fd;
}

.text-muted {
  color: var(--text-muted, #6b7280);
}

.loading-cell, .empty-cell {
  padding: 40px 0;
  color: var(--text-muted, #8b8d9b);
}

/* 分页条 */
.pagination-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-top: 1px solid var(--border-color, #2a2d3d);
}

.page-info {
  font-size: 13px;
  color: var(--text-muted, #8b8d9b);
}

.page-buttons {
  display: flex;
  align-items: center;
  gap: 12px;
}

.current-page {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #e2e8f0);
}

/* 按钮通用 */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all 0.2s;
}

.btn-secondary {
  background: var(--bg-dark, #14151b);
  color: var(--text-primary, #e2e8f0);
  border: 1px solid var(--border-color, #2a2d3d);
}

.btn-secondary:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.05);
  border-color: #4b5563;
}

.btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-sm {
  padding: 5px 10px;
  font-size: 12px;
}

.icon-spin {
  width: 14px;
  height: 14px;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  100% { transform: rotate(360deg); }
}
</style>
