<template>
  <div class="tasks-page">
    <!-- 创建面板 -->
    <PanelCard title="NEW MISSION" :icon="icons.plus" padded>
      <div class="create-form">
        <div class="form-row">
          <div class="form-field">
            <label class="field-label">TYPE</label>
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
            <input v-model="form.ports" class="field-input" placeholder="80,443,8080-8090 (optional)" />
          </div>
        </div>

        <div class="form-field">
          <label class="field-label">
            TARGETS
            <span class="field-hint">One per line · IP / CIDR / Domain</span>
          </label>
          <textarea v-model="targets" class="field-textarea" rows="3" placeholder="192.168.1.0/30&#10;10.0.0.1&#10;example.com"></textarea>
        </div>

        <button class="launch-btn" :disabled="creating" @click="create">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="5,3 19,12 5,21 5,3"/>
          </svg>
          <span v-if="!creating">LAUNCH MISSION</span>
          <span v-else>DEPLOYING...</span>
        </button>
      </div>
    </PanelCard>

    <!-- 任务列表 -->
    <PanelCard title="ACTIVE MISSIONS" :icon="icons.grid" small>
      <template #actions>
        <span class="refresh-hint" @click="refresh" title="Refresh">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23,4 23,10 17,10"/>
            <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/>
          </svg>
        </span>
      </template>

      <el-table :data="tasks" v-loading="loading" stripe class="tasks-table">
        <el-table-column prop="id" label="Mission ID" width="200" show-overflow-tooltip>
          <template #default="{ row }"><span class="mono-text">{{ row.id }}</span></template>
        </el-table-column>
        <el-table-column label="Type" width="110">
          <template #default="{ row }">
            <span class="kind-tag" :class="'kind-' + row.kind">{{ row.kind === 'scan' ? 'SCAN' : 'VULN' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="Progress" min-width="240">
          <template #default="{ row }">
            <MissionProgress :done="doneCount(row)" :total="totalCount(row)" :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="Status" width="130" align="center">
          <template #default="{ row }">
            <StatusTag :text="statusText(row.status)" :tone="statusTone(row.status)" dot :pulse="row.status === 1" />
          </template>
        </el-table-column>
        <el-table-column label="" width="150" align="center">
          <template #default="{ row }">
            <div class="action-group">
              <button class="action-btn" @click="view(row.id)">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>
                <span>Info</span>
              </button>
              <button class="action-btn warn" @click="resume(row.id)">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="1,4 1,10 7,10"/><path d="M3.51 15a9 9 0 102.12-9.36L1 10"/></svg>
                <span>Retry</span>
              </button>
              <button class="action-btn danger" @click="confirmDelete(row)">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3,6 5,6 21,6"/><path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/></svg>
                <span>Delete</span>
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </PanelCard>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailVisible" width="740px" class="task-dialog">
      <template #header>
        <DialogHeader title="Mission Report" :icon="icons.grid" />
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

        <SectionLabel label="TARGET DETAILS" :count="(detail.items || []).length" />
        <el-table :data="detail.items" size="small" stripe class="sub-table" max-height="340">
          <el-table-column prop="target" label="Target" min-width="200" show-overflow-tooltip>
            <template #default="{ row }"><span class="mono-text">{{ row.target }}</span></template>
          </el-table-column>
          <el-table-column label="Status" width="110" align="center">
            <template #default="{ row }">
              <StatusTag :text="itemStatusText(row.status)" :tone="itemTone(row.status)" />
            </template>
          </el-table-column>
          <el-table-column label="Result" min-width="260" show-overflow-tooltip>
            <template #default="{ row }"><span class="mono-text small">{{ row.result ? JSON.stringify(row.result) : '—' }}</span></template>
          </el-table-column>
          <el-table-column label="Timestamp" width="180" show-overflow-tooltip>
            <template #default="{ row }"><span class="text-muted small">{{ row.updated_at || row.created_at || '—' }}</span></template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>

    <!-- 删除确认对话框 -->
    <el-dialog v-model="delVisible" width="420px" class="task-dialog">
      <template #header>
        <DialogHeader title="CONFIRM DELETION" :icon="icons.trash" accent="red" />
      </template>
      <p class="del-text">
        确定要删除任务 <span class="mono-text">{{ delTarget }}</span> 吗？<br />
        该操作将同时移除其全部子项，且不可恢复。
      </p>
      <template #footer>
        <div class="del-footer">
          <button class="action-btn" @click="delVisible = false">CANCEL</button>
          <button class="action-btn danger" :disabled="deleting" @click="doDelete">
            {{ deleting ? 'DELETING...' : 'DELETE' }}
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

const kindOpts = [
  { label: 'ASSET SCAN', value: 'scan' },
  { label: 'VULN DETECT', value: 'vuln' }
]

const detailItems = computed(() => {
  if (!detail.value || !detail.value.task) return []
  const t = detail.value.task
  return [
    { key: 'Mission ID', value: t.id, mono: true },
    { key: 'Type', value: t.kind === 'scan' ? 'Asset Scanning' : 'Vulnerability Detection' },
    { key: 'Status', value: statusText(t.status), status: true, tone: statusTone(t.status), pulse: t.status === 1 },
    { key: 'Completion', value: doneCount(t) + ' / ' + totalCount(t), highlight: true }
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

async function resume(id) {
  await api.resumeTask(id)
  await refresh()
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

function totalCount(task) { return (task.progress && task.progress.total) || 0 }
function doneCount(task) { return (task.progress && task.progress.done) || 0 }
function statusText(s) { return ['PENDING', 'RUNNING', 'COMPLETED'][s] || s }
function statusTone(s) { return ['muted', 'cyan', 'green'][s] || 'muted' }
function itemStatusText(s) { return ['PENDING', 'DONE', 'FILTERED'][s] || s }
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

/* 操作按钮 */
.action-group { display: flex; gap: 6px; justify-content: center; }
.action-btn {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 4px 10px;
  background: transparent; border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  font-family: var(--font-heading); font-size: 10px; font-weight: 600;
  letter-spacing: 0.04em;
  cursor: pointer; transition: all 0.2s ease;
}
.action-btn svg { width: 12px; height: 12px; }
.action-btn:hover { border-color: var(--accent-cyan); color: var(--accent-cyan); background: rgba(0,212,255,0.06); }
.action-btn.warn:hover { border-color: var(--accent-amber); color: var(--accent-amber); background: rgba(245,166,35,0.06); }
.action-btn.danger:hover { border-color: var(--accent-red); color: var(--accent-red); background: rgba(255,82,82,0.06); }

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
