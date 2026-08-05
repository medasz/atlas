<template>
  <!-- 键值详情网格：统一 Tasks / Assets 详情的 .detail-grid -->
  <div class="detail-grid">
    <div class="detail-item" v-for="(it, i) in items" :key="i">
      <span class="detail-key">{{ it.key }}</span>
      <!-- 允许通过 #value 插槽自定义某些单元格（如内嵌状态标签） -->
      <slot name="value" :item="it" :index="i">
        <span class="detail-val" :class="{ mono: it.mono, highlight: it.highlight, multiline: it.multiline }">{{ it.value }}</span>
      </slot>
    </div>
  </div>
</template>

<script setup>
/**
 * 键值详情网格
 * @prop {Array} items - 项数组，每项 { key, value, mono?, highlight?, status?, tone?, pulse? }
 *   - status:true 时配合 #value 插槽渲染 StatusTag
 */
defineProps({
  items: { type: Array, default: () => [] }
})
</script>

<style scoped>
.detail-grid {
  display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 20px;
}
.detail-item {
  display: flex; flex-direction: column; gap: 4px;
  padding: 12px 16px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
}
.detail-key {
  font-family: var(--font-heading); font-size: 10px; font-weight: 700;
  letter-spacing: 0.1em; color: var(--text-muted); text-transform: uppercase;
}
.detail-val {
  font-family: var(--font-mono); font-size: 14px; color: var(--text-primary);
  word-break: break-all;
}
.detail-val.mono { font-size: 11px; }
.detail-val.multiline { white-space: pre-wrap; max-height: 120px; overflow: auto; overflow-wrap: anywhere; }
.detail-val.highlight { color: var(--accent-cyan); font-size: 22px; font-weight: 600; }

/* 小屏：单列堆叠 */
@media (max-width: 520px) {
  .detail-grid { grid-template-columns: 1fr; }
}
</style>
