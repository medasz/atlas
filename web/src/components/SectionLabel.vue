<template>
  <!-- 区块标题：统一 Tasks / Assets 详情中的 .section-label -->
  <div class="section-label">
    <slot>{{ label }}</slot>
    <span v-if="count !== null && count !== undefined" class="section-count">{{ count }}</span>
  </div>
</template>

<script setup>
/**
 * 区块标题（带可选计数徽标）
 * @prop {string} label - 标题文字
 * @prop {number|string} count - 计数徽标内容
 */
defineProps({
  label: { type: String, default: '' },
  count: { type: [Number, String], default: null }
})
</script>

<style scoped>
.section-label {
  display: flex; align-items: center; gap: 8px;
  margin: 20px 0 10px;
  font-family: var(--font-heading); font-size: 11px; font-weight: 700;
  letter-spacing: 0.1em; color: var(--text-muted);
  padding-bottom: 8px; border-bottom: 1px solid var(--border-subtle);
  text-transform: uppercase;
}
.section-count {
  font-family: var(--font-mono); font-size: 13px;
  color: var(--accent-cyan); background: rgba(0,212,255,0.08);
  padding: 1px 8px; border-radius: 10px;
}
</style>
