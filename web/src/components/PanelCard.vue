<template>
  <!-- 卡片容器：统一「标题栏 + 内容区」结构，消除各视图中重复出现的 .panel-heading 样式 -->
  <section class="panel-card">
    <header class="panel-head" :class="[ small ? 'is-small' : '', 'accent-' + accent ]">
      <span v-if="icon" class="panel-icon" v-html="icon"></span>
      <span class="panel-title">{{ title }}</span>
      <!-- 右侧操作区（如刷新按钮、计数徽标），自动推到最右 -->
      <span class="panel-actions"><slot name="actions" /></span>
    </header>
    <div class="panel-body" :class="{ padded }">
      <slot />
    </div>
  </section>
</template>

<script setup>
/**
 * 通用面板卡片
 * @prop {string} title   - 标题栏文字
 * @prop {string} icon    - 标题栏图标（内联 SVG 字符串）
 * @prop {boolean} small  - 使用紧凑标题栏
 * @prop {string} accent  - 图标强调色：cyan | red | amber
 * @prop {boolean} padded - 内容区是否带内边距（表单用 true，表格用 false）
 */
defineProps({
  title: { type: String, required: true },
  icon: { type: String, default: '' },
  small: { type: Boolean, default: false },
  accent: { type: String, default: 'cyan' },
  padded: { type: Boolean, default: false }
})
</script>

<style scoped>
.panel-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
}
.panel-head {
  display: flex; align-items: center; gap: 10px;
  padding: 14px 20px;
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  font-family: var(--font-heading); font-size: 13px; font-weight: 700;
  letter-spacing: 0.1em; color: var(--text-secondary);
}
.panel-head.is-small { font-size: 11px; padding: 12px 20px; }
.panel-icon { display: inline-flex; }
.panel-icon :deep(svg) { width: 18px; height: 18px; color: var(--accent-cyan); }
.panel-head.is-small .panel-icon :deep(svg) { width: 16px; height: 16px; }
.accent-red .panel-icon :deep(svg) { color: var(--accent-red); }
.accent-amber .panel-icon :deep(svg) { color: var(--accent-amber); }
.panel-title { white-space: nowrap; }
.panel-actions { margin-left: auto; display: flex; align-items: center; gap: 10px; }
.panel-body.padded { padding: 20px; }

/* 小屏：标题栏字号与内边距收紧 */
@media (max-width: 560px) {
  .panel-head { padding: 12px 14px; gap: 8px; }
  .panel-head.is-small { padding: 10px 14px; }
  .panel-body.padded { padding: 16px 14px; }
}
</style>
