<template>
  <div class="tasks-page">
    <!-- 创建面板 -->
    <PanelCard title="新建任务" :icon="icons.plus" padded>
      <div class="create-form">
        <div class="form-row">
          <div class="form-field">
            <label class="field-label">类型</label>
            <div class="radio-group">
              <label v-for="opt in kindOpts" :key="opt.value" class="radio-item" :class="{ active: form.kind === opt.value }">
                <input type="radio" v-model="form.kind" :value="opt.value" />
                <span class="radio-mark"></span>
                <span>{{ opt.label }}</span>
              </label>
            </div>
          </div>
          <div class="form-field">
            <label class="field-label">PORTS</label>
            <input v-model="form.ports" class="field-input" placeholder="80,443,8080-8090（可选）" />
          </div>
        </div>

        <div class="form-field">
          <label class="field-label">
            目标
            <span class="field-hint">每行一个 · IP / CIDR / 域名</span>
          </label>
          <textarea v-model="targets" class="field-textarea" rows="3" placeholder="192.168.1.0/30&#10;10.0.0.1&#10;example.com"></textarea>
        </div>

        <button class="launch-btn" :disabled="creating" @click="create">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="5,3 19,12 5,21 5,3"/>
          </svg>
          <span v-if="!creating">发起任务</span>
          <span v-else>部署中…</span>
        </button>
      </div>
    </PanelCard>

    <!-- 任务列表 -->
    <PanelCard title="进行中的任务" :icon="icons.grid" small>
      <template #actions>
        <span class="refresh-hint" @click="refresh" title="Refresh">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23,4 23,10 17,10"/>
            <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
          </svg>
        </span>
      </template>

      <el-table :data="tasks" v-loading="loading" stripe class="tasks-table">
        <el-table-column prop="id" label="任务 ID" width="200" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono-text">{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <span class="kind-tag" :class="'kind-' + row.kind">{{ row.kind === 'scan' ? '扫描' : '漏洞' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="进度" min-width="240">
          <template #default="{ row }">
            <MissionProgress :done="doneCount(row)" :total="totalCount(row)" :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="状态" width="130" align="center">
          <template #default="{ row }">
            <StatusTag :text="statusText(row.status)" :tone="statusTone(row.status)" dot :pulse="row.status === 1" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="340" align="center">
          <template #default="{ row }">
            <div class="action-group">
              <button
                v-for="act in actionsFor(row)"
                :key="act.key"
                class="action-btn"
                :class="[act.tone, { 'is-loading': isBusy(row, act) }]"
                :disabled="rowBusy(row) || isBusy(row, act)"
                :title="act.label"
                @click="runAction(act, row)"
              >
                <svg v-if="isBusy(row, act)" class="spinner" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="12"/></svg>
                <svg v-else-if="act.icon === 'eye'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg v-else-if="act.icon === 'upload'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17,8 12,3 7,8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
                <svg v-else-if="act.icon === 'clock'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg>
                <svg v-else-if="act.icon === 'pause'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="9" y1="5" x2="9" y2="19"/><line x1="15" y1="5" x2="15" y2="19"/></svg>
                <svg v-else-if="act.icon === 'resume'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="7,5 19,12 7,19" /></svg>
                <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3,6 5,6 21,6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                <span class="action-label">{{ act.label }}</span>
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </PanelCard>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailVisible" width="740px" class="task-dialog">
      <template #header>
        <DialogHeader title="任务报告" :icon="icons.grid" />
      </template>
      <template v-if="detail && detail.task">
        <DetailGrid :items="detailItems">
          <template #value="{ item }">
            <span v-if="item.status" class="detail-val">
              <StatusTag :text="item.value" :tone="item.tone" dot :pulse="item.pulse" />
            </span>
            <span v-else class="detail-val" :class="{ mono: item.mono, highlight: item.highlight }">{{ item.value }}</span>
          </template>
        </DetailGrid>

        <MissionProgress :done="doneCount(detail.task)" :total="totalCount(detail.task)" :status="detail.task.status" size="lg" />

        <SectionLabel label="目标明细" :count="(detail.items || []).length" />
        <el-table :data="detail.items" size="small" stripe class="sub-table" max-height="340">
          <el-table-column prop="target" label="目标" min-width="200" show-overflow-tooltip>
            <template #default="{ row }"><span class="mono-text">{{ row.target }}</span></template>
          </el-table-column>
          <el-table-column label="状态" width="110" align="center">
            <template #default="{ row }">
              <StatusTag :text="itemStatusText(row.status)" :tone="itemTone(row.status)" />
            </template>
          </el-table-column>
          <el-table-column label="结果" min-width="260" show-overflow-tooltip>
            <template #default="{ row }"><span class="mono-text small">{{ row.result ? JSON.stringify(row.result) : '—' }}</span></template>
          </el-table-column>
          <el-table-column label="时间戳" width="180" show-overflow-tooltip>
            <template #default="{ row }"><span class="text-muted small">{{ row.updated_at || row.created_at || '—' }}</span></template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>

    <!-- 删除确认对话框 -->
    <el-dialog v-model="delVisible" width="420px" class="task-dialog">
      <template #header>
        <DialogHeader title="确认删除" :icon="icons.trash" accent="red" />
      </template>
      <p class="del-text">
        确定要删除任务 <span class="mono-text">{{ delTarget }}</span> 吗？<br />
        该操作将同时移除其全部子项，且不可恢复。
      </p>
      <template #footer>
        <div class="del-footer">
          <button class="action-btn" @click="delVisible = false">取消</button>
          <button class="action-btn danger" :disabled="deleting" @click="doDelete">
            {{ deleting ? '删除中…' : '删除' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api'
import PanelCard from '../components/PanelCard.vue'
import StatusTag from '../components/StatusTag.vue'
import MissionProgress from '../components/MissionProgress.vue'
import DetailGrid from '../components/DetailGrid.vue'
import SectionLabel from '../components/SectionLabel.vue'
import DialogHeader from '../components/DialogHeader.vue'
import { ElMessage } from 'element-plus'

const icons = {
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg>',
  grid: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="3"/><path d="M9 12l2 2 4-4"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3,6 5,6 21,6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>'
}

const form = ref({ kind: 'scan', ports: '' })
const targets = ref('')
const creating = ref(false)
const tasks = ref([])
const loading = ref(false)
const detailVisible = ref(false)
const detail = ref(null)
const delVisible = ref(false)
const delTarget = ref('')
const deleting = ref(false)
let pollTimer = null

// 任务状态常量（与后端 model.Task* 对齐）
const S = { PENDING: 0, RUNNING: 1, DONE: 2, PAUSED: 3 }
// 行级操作进行中标记：key = `${act.key}:${row.id}`
const pending = ref({})

const kindOpts = [
  { label: '资产扫描', value: 'scan' },
  { label: '漏洞检测', value: 'vuln' }
]

const detailItems = computed(() => {
  if (!detail.value || !detail.value.task) return []
  const t = detail.value.task
  return [
    { key: '任务 ID', value: t.id, mono: true },
    { key: '类型', value: t.kind === 'scan' ? '资产扫描' : '漏洞检测' },
    { key: '状态', value: statusText(t.status), status: true, tone: statusTone(t.status), pulse: t.status === 1 },
    { key: '完成度', value: doneCount(t) + ' / ' + totalCount(t), highlight: true }
  ]
})

async function create() {
  const list = targets.value.split('\n').map((s) => s.trim()).filter(Boolean)
  if (!list.length) return
  creating.value = true
  try {
    await api.createTask({ kind: form.value.kind, scope: { targets: list, ports: form.value.ports }, schedule: {}, rate_limit: {} })
    targets.value = ''
    await refresh()
  } finally {
    creating.value = false
  }
}

async function refresh() {
  loading.value = true
  try {
    const r = await api.listTasks()
    tasks.value = r.items || []
  } finally {
    loading.value = false
  }
}

async function view(id) {
  const r = await api.getTask(id)
  detail.value = r
  detailVisible.value = true
}

function confirmDelete(row) {
  delTarget.value = row.id
  delVisible.value = true
}

async function doDelete() {
  deleting.value = true
  try {
    await api.deleteTask(delTarget.value)
    delVisible.value = false
    if (detail.value && detail.value.task && detail.value.task.id === delTarget.value) {
      detailVisible.value = false
    }
    await refresh()
  } finally {
    deleting.value = false
  }
}

// 根据任务状态/进度动态返回最相关的快捷操作
function actionsFor(row) {
  const view = { key: 'view', label: '查看详情', icon: 'eye', tone: 'neutral' }
  const abandon = { key: 'abandon', label: '放弃任务', icon: 'trash', tone: 'danger', confirm: true }
  if (row.status === S.RUNNING) {
    // 进行中：暂停为主操作，查看详情为信息入口，放弃任务为危险项
    return [
      { key: 'pause', label: '暂停', icon: 'pause', tone: 'primary', api: 'pause' },
      view,
      abandon
    ]
  }
  if (row.status === S.PAUSED) {
    // 已暂停：恢复为主操作
    return [
      { key: 'resume', label: '恢复', icon: 'resume', tone: 'primary', api: 'resume' },
      view,
      abandon
    ]
  }
  if (row.status === S.PENDING) {
    return [
      { key: 'resume', label: '恢复', icon: 'resume', tone: 'primary', api: 'resume' },
      view,
      abandon
    ]
  }
  // 已完成
  return [view, abandon]
}

function actKey(act, row) { return act.key + ':' + row.id }
function isBusy(row, act) { return !!pending.value[actKey(act, row)] }
function rowBusy(row) {
  const p = row.id + ':'
  return Object.keys(pending.value).some((k) => k.startsWith(p) && pending.value[k])
}

async function runAction(act, row) {
  if (act.confirm) { confirmDelete(row); return }
  if (act.key === 'view') { view(row.id); return }
  pending.value = { ...pending.value, [actKey(act, row)]: true }
  try {
    if (act.key === 'resume') {
      await api.resumeTask(row.id)
    } else if (act.key === 'pause') {
      await api.pauseTask(row.id)
    }
    ElMessage.success(act.label + '成功')
    await refresh()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(act.label + '失败：' + (e.message || e))
  } finally {
    pending.value = { ...pending.value, [actKey(act, row)]: false }
  }
}

function totalCount(task) { return (task.progress && task.progress.total) || 0 }
function doneCount(task) { return (task.progress && task.progress.done) || 0 }
function statusText(s) { return ['待运行', '运行中', '已完成', '已暂停'][s] || s }
function statusTone(s) { return ['muted', 'cyan', 'green', 'amber'][s] || 'muted' }
function itemStatusText(s) { return ['待运行', '已完成', '已过滤'][s] || s }
function itemTone(s) { return ['muted', 'green', 'amber'][s] || 'muted' }

onMounted(() => {
  refresh()
  pollTimer = setInterval(refresh, 3000)
})
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.tasks-page { display: flex; flex-direction: column; gap: 20px; }

/* ===== 创建表单 ===== */
.create-form { display: flex; flex-direction: column; gap: 16px; }
.form-row { display: flex; gap: 20px; }
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-row .form-field:first-child,
.form-row .form-field:last-child { flex: 1; }

.field-label {
  font-family: var(--font-heading); font-size: 10px; font-weight: 700;
  letter-spacing: 0.1em; color: var(--text-muted);
  display: flex; align-items: center; gap: 8px;
}
.field-hint {
  font-family: var(--font-body); font-size: 11px; font-weight: 400;
  letter-spacing: 0; color: var(--text-muted); opacity: 0.6;
}
.field-input {
  padding: 10px 14px;
  background: var(--bg-input); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-primary); font-family: var(--font-mono); font-size: 13px;
  outline: none; transition: border-color 0.25s;
}
.field-input:focus { border-color: var(--accent-cyan); }
.field-textarea {
  padding: 10px 14px; resize: vertical;
  background: var(--bg-input); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-primary); font-family: var(--font-mono); font-size: 13px;
  line-height: 1.7; outline: none; transition: border-color 0.25s;
  caret-color: var(--accent-cyan);
}
.field-textarea::placeholder { color: var(--text-muted); font-family: var(--font-body); }
.field-textarea:focus { border-color: var(--accent-cyan); }

.radio-group { display: flex; gap: 12px; }
.radio-item {
  display: flex; align-items: center; gap: 8px; cursor: pointer;
  font-family: var(--font-heading); font-size: 12px; font-weight: 600;
  letter-spacing: 0.06em; color: var(--text-muted);
  transition: color 0.2s;
}
.radio-item input { display: none; }
.radio-mark {
  width: 14px; height: 14px; border-radius: 50%;
  border: 1.5px solid var(--border-active);
  display: flex; align-items: center; justify-content: center;
  transition: all 0.2s;
}
.radio-mark::after {
  content: ''; width: 6px; height: 6px; border-radius: 50%;
  background: var(--accent-cyan);
  transform: scale(0); transition: transform 0.2s ease;
}
.radio-item.active { color: var(--accent-cyan); }
.radio-item.active .radio-mark { border-color: var(--accent-cyan); }
.radio-item.active .radio-mark::after { transform: scale(1); }

.launch-btn {
  display: flex; align-items: center; justify-content: center; gap: 10px;
  width: 100%; padding: 13px;
  background: linear-gradient(135deg, rgba(0,230,118,0.12) 0%, rgba(0,200,100,0.05) 100%);
  border: 1px solid var(--accent-green); border-radius: var(--radius-md);
  color: var(--accent-green);
  font-family: var(--font-heading); font-size: 14px; font-weight: 700;
  letter-spacing: 0.1em;
  cursor: pointer; transition: all 0.3s ease;
}
.launch-btn svg { width: 16px; height: 16px; }
.launch-btn:hover:not(:disabled) {
  box-shadow: 0 0 28px rgba(0,230,118,0.2), inset 0 0 18px rgba(0,230,118,0.04);
  transform: translateY(-1px);
}
.launch-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* ===== 任务表格 ===== */
.tasks-table { border-radius: 0; }
.mono-text { font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); }
.mono-text.small { font-size: 11px; color: var(--text-secondary); }

.kind-tag {
  display: inline-block; padding: 2px 10px; border-radius: 3px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 700; letter-spacing: 0.08em;
}
.kind-scan { color: var(--accent-cyan); background: rgba(0,212,255,0.08); border: 1px solid rgba(0,212,255,0.15); }
.kind-vuln { color: var(--accent-violet); background: rgba(123,97,255,0.08); border: 1px solid rgba(123,97,255,0.15); }

/* 操作按钮（动态状态驱动） */
.action-group {
  display: flex; flex-wrap: wrap; gap: 8px;
  justify-content: center; align-items: center;
}
.action-btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 6px;
  min-height: 32px; padding: 6px 12px;
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-family: var(--font-heading); font-size: 11px; font-weight: 600;
  letter-spacing: 0.04em; white-space: nowrap;
  cursor: pointer; transition: all 0.2s ease;
}
.action-btn svg { width: 14px; height: 14px; flex: none; }
.action-btn .action-label { line-height: 1; }
.action-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.action-btn:not(:disabled):hover {
  border-color: var(--accent-cyan); color: var(--accent-cyan); background: rgba(0,212,255,0.06);
}
/* 主操作：视觉重心集中于一处（暂停 / 恢复） */
.action-btn.primary {
  border-color: rgba(0,212,255,0.45); color: var(--accent-cyan); background: rgba(0,212,255,0.08);
}
.action-btn.primary:not(:disabled):hover {
  border-color: var(--accent-cyan); background: rgba(0,212,255,0.14); box-shadow: 0 0 16px rgba(0,212,255,0.18);
}
.action-btn.warn { color: var(--accent-amber); border-color: rgba(245,166,35,0.3); background: rgba(245,166,35,0.05); }
.action-btn.warn:not(:disabled):hover { border-color: var(--accent-amber); background: rgba(245,166,35,0.1); box-shadow: 0 0 16px rgba(245,166,35,0.15); }
.action-btn.danger { color: var(--accent-red); border-color: rgba(255,71,87,0.3); background: rgba(255,71,87,0.05); }
.action-btn.danger:not(:disabled):hover { border-color: var(--accent-red); background: rgba(255,71,87,0.1); box-shadow: 0 0 16px rgba(255,71,87,0.15); }

/* 加载态 */
.action-btn.is-loading { color: var(--text-muted); border-color: var(--border-subtle); background: transparent; box-shadow: none; }
.action-btn .spinner { width: 14px; height: 14px; animation: spin 0.8s linear infinite; }

/* 小屏：放大点按区域，防止误触 */
@media (max-width: 640px) {
  .action-btn { min-height: 36px; padding: 8px 14px; }
}

/* ===== 详情对话框 ===== */
.task-dialog :deep(.el-dialog) { border: 1px solid var(--border-subtle); }
.sub-table { border-radius: var(--radius-md); overflow: hidden; }

.del-text {
  font-family: var(--font-body); font-size: 13px; line-height: 1.7;
  color: var(--text-secondary); margin: 0;
}
.del-text .mono-text { color: var(--accent-red); font-size: 12px; }
.del-footer { display: flex; justify-content: flex-end; gap: 10px; }
.del-footer .action-btn.danger { border-color: var(--accent-red); color: var(--accent-red); }
.del-footer .action-btn.danger:hover { background: rgba(255,82,82,0.1); box-shadow: 0 0 18px rgba(255,82,82,0.15); }

.text-muted { color: var(--text-muted); }
.text-muted.small { font-size: 11px; }

.refresh-hint { cursor: pointer; opacity: 0.4; transition: opacity 0.2s; display: inline-flex; }
.refresh-hint:hover { opacity: 1; }
.refresh-hint svg { width: 14px; height: 14px; }

/* 小屏：创建表单纵向堆叠 */
@media (max-width: 640px) {
  .form-row { flex-direction: column; gap: 16px; }
}
</style>
