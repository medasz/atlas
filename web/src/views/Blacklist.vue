<template>
  <div class="blacklist-page">
    <!-- 添加面板 -->
    <PanelCard title="加入黑名单" :icon="icons.shield" accent="red" padded>
      <div class="add-form">
        <div class="type-pills">
          <button v-for="opt in typeOpts" :key="opt.value"
            class="pill" :class="{ active: form.type === opt.value }"
            @click="form.type = opt.value">
            {{ opt.label }}
          </button>
        </div>
        <div class="value-row">
          <input v-model="form.value" class="value-input" :placeholder="placeholder" @keyup.enter="add" />
          <button class="add-btn" :disabled="adding" @click="add">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            <span>{{ adding ? '添加中…' : '屏蔽' }}</span>
          </button>
        </div>
      </div>
    </PanelCard>

    <!-- 黑名单列表 -->
    <PanelCard title="已屏蔽条目" :icon="icons.list" accent="amber" small>
      <template #actions>
        <span class="entry-count">{{ items.length }}</span>
      </template>

      <el-table :data="items" v-loading="loading" stripe class="blacklist-table">
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <StatusTag :text="row.type.toUpperCase()" :tone="typeTone(row.type)" />
          </template>
        </el-table-column>
        <el-table-column label="值" min-width="260">
          <template #default="{ row }"><span class="mono-value">{{ row.value }}</span></template>
        </el-table-column>
        <el-table-column prop="operator" label="操作方" width="140" align="center">
          <template #default="{ row }"><span class="op-tag">{{ row.operator || '—' }}</span></template>
        </el-table-column>
        <el-table-column prop="created_at" label="添加时间" width="200" show-overflow-tooltip>
          <template #default="{ row }"><span class="time-text">{{ row.created_at || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="" width="100" align="center">
          <template #default="{ row }">
            <button class="remove-btn" @click="remove(row.type, row.value)">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="3,6 5,6 21,6"/>
                <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/>
              </svg>
              <span>移除</span>
            </button>
          </template>
        </el-table-column>
      </el-table>

      <EmptyState v-if="!items.length && !loading" text="黑名单为空" />
    </PanelCard>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api'
import { ElMessage } from 'element-plus'
import PanelCard from '../components/PanelCard.vue'
import StatusTag from '../components/StatusTag.vue'
import EmptyState from '../components/EmptyState.vue'

const icons = {
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  list: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>'
}

const form = ref({ type: 'ip', value: '' })
const adding = ref(false)
const items = ref([])
const loading = ref(false)

const typeOpts = [
  { label: 'IP', value: 'ip' },
  { label: 'CIDR', value: 'cidr' },
  { label: '域名', value: 'domain' }
]

const placeholder = computed(() => {
  const map = { ip: '例如：10.0.0.5', cidr: '例如：10.0.0.0/8', domain: '例如：example.com' }
  return map[form.value.type] || ''
})

function typeTone(t) { return { ip: 'cyan', cidr: 'violet', domain: 'amber' }[t] || 'muted' }

async function add() {
  if (!form.value.value) return
  adding.value = true
  try {
    await api.addBlacklist({ type: form.value.type, value: form.value.value, operator: 'web' })
    form.value.value = ''
    await refresh()
  } catch (e) {
    ElMessage.error('添加失败：' + e.message)
  } finally {
    adding.value = false
  }
}

async function remove(type, value) {
  await api.deleteBlacklist(type, value)
  await refresh()
}

async function refresh() {
  loading.value = true
  try {
    const r = await api.listBlacklist()
    items.value = r.items || []
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<style scoped>
.blacklist-page { display: flex; flex-direction: column; gap: 20px; }

/* ===== 添加表单 ===== */
.add-form { display: flex; flex-direction: column; gap: 14px; }
.type-pills { display: flex; gap: 6px; }
.pill {
  padding: 7px 20px; background: transparent; border: 1px solid var(--border-subtle);
  border-radius: 20px; color: var(--text-muted);
  font-family: var(--font-heading); font-size: 11px; font-weight: 700;
  letter-spacing: 0.08em; cursor: pointer; transition: all 0.2s ease;
}
.pill:hover { border-color: var(--text-muted); color: var(--text-secondary); }
.pill.active { border-color: var(--accent-red); color: var(--accent-red); background: rgba(255,71,87,0.08); }

.value-row { display: flex; gap: 10px; }
.value-input {
  flex: 1; padding: 11px 14px; background: var(--bg-input); border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md); color: var(--text-primary); font-family: var(--font-mono); font-size: 13px;
  outline: none; transition: border-color 0.25s;
}
.value-input::placeholder { color: var(--text-muted); font-family: var(--font-body); }
.value-input:focus { border-color: var(--accent-red); }
.add-btn {
  display: flex; align-items: center; gap: 8px; padding: 11px 24px;
  background: rgba(255,71,87,0.1); border: 1px solid var(--accent-red); border-radius: var(--radius-md);
  color: var(--accent-red); font-family: var(--font-heading); font-size: 13px; font-weight: 700;
  letter-spacing: 0.06em; cursor: pointer; transition: all 0.25s ease; white-space: nowrap;
}
.add-btn svg { width: 16px; height: 16px; }
.add-btn:hover:not(:disabled) { box-shadow: 0 0 20px rgba(255,71,87,0.2), inset 0 0 16px rgba(255,71,87,0.04); transform: translateY(-1px); }
.add-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* ===== 列表 ===== */
.blacklist-table { border-radius: 0; }
.mono-value { font-family: var(--font-mono); font-size: 13px; color: var(--text-primary); }
.op-tag { color: var(--text-muted); font-size: 12px; }
.time-text { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); }

.remove-btn {
  display: inline-flex; align-items: center; gap: 4px; padding: 4px 10px;
  background: transparent; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm);
  color: var(--text-muted); font-family: var(--font-heading); font-size: 10px; font-weight: 600;
  letter-spacing: 0.04em; cursor: pointer; transition: all 0.2s ease;
}
.remove-btn svg { width: 12px; height: 12px; }
.remove-btn:hover { border-color: var(--accent-red); color: var(--accent-red); background: rgba(255,71,87,0.06); }

.entry-count {
  font-family: var(--font-mono); font-size: 13px; color: var(--accent-cyan);
  background: rgba(0,212,255,0.08); padding: 1px 10px; border-radius: 10px;
}

/* 小屏：添加表单纵向堆叠 */
@media (max-width: 640px) {
  .value-row { flex-direction: column; }
  .add-btn { justify-content: center; }
}
</style>
