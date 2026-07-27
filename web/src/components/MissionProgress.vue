<template>
  <!-- 任务进度条：替代 Tasks 中自实现的 .progress-cell / .big-progress，支持列表单元格与对话框大尺寸两种形态 -->
  <div class="mprogress" :class="'size-' + size">
    <div class="mp-track">
      <div class="mp-fill" :class="{ done: status === 2, active: status === 1 }"
           :style="{ width: pct + '%' }">
        <div v-if="status === 1" class="mp-scan"></div>
      </div>
    </div>
    <span class="mp-text" :class="{ done: status === 2 }">{{ done }} / {{ total }} · {{ pct }}%</span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

/**
 * 任务进度条
 * @prop {number} done   - 已完成数量
 * @prop {number} total  - 总数量
 * @prop {number} status - 任务状态：0 待运行 / 1 运行中 / 2 已完成
 * @prop {string} size   - 尺寸：sm（表格单元格）| lg（对话框大进度）
 */
const props = defineProps({
  done: { type: Number, default: 0 },
  total: { type: Number, default: 0 },
  status: { type: Number, default: 0 },
  size: { type: String, default: 'sm' }
})

const pct = computed(() =>
  props.total ? Math.min(100, Math.round((props.done / props.total) * 100)) : 0
)
</script>

<style scoped>
.mprogress { display: flex; align-items: center; gap: 10px; }
.mp-track { background: var(--bg-hover); border-radius: 999px; overflow: hidden; }
.mp-fill {
  height: 100%; border-radius: 999px; background: var(--accent-cyan);
  position: relative; overflow: hidden; transition: width 0.45s ease;
}
.mp-fill.active { background: linear-gradient(90deg, var(--accent-cyan), #0090ff); }
.mp-fill.done { background: var(--accent-green); }
.mp-scan {
  position: absolute; inset: 0;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.3), transparent);
  animation: scanline 1.5s linear infinite;
}
.mp-text.done { color: var(--accent-green); }

/* 小尺寸：用于表格单元格 */
.size-sm .mp-track { flex: 1; height: 6px; }
.size-sm .mp-text {
  font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary);
  white-space: nowrap; min-width: 84px; text-align: right;
}

/* 大尺寸：用于任务详情对话框 */
.size-lg { gap: 14px; }
.size-lg .mp-track { flex: 1; height: 10px; }
.size-lg .mp-fill.done { background: linear-gradient(90deg, var(--accent-green), #00c853); }
.size-lg .mp-text {
  font-family: var(--font-heading); font-size: 22px; font-weight: 700;
  color: var(--accent-cyan); min-width: 50px; text-align: right;
}
</style>
