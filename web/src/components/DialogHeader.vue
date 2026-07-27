<template>
  <!-- 对话框标题：统一 Tasks / Assets 的 .dialog-header-custom -->
  <div class="dialog-header-custom" :class="'accent-' + accent">
    <span v-if="icon" class="dh-icon" v-html="icon"></span>
    <span>{{ title }}</span>
  </div>
</template>

<script setup>
/**
 * 对话框自定义标题
 * @prop {string} title  - 标题文字
 * @prop {string} icon   - 标题图标（内联 SVG 字符串）
 * @prop {string} accent - 图标强调色：cyan | red
 */
defineProps({
  title: { type: String, required: true },
  icon: { type: String, default: '' },
  accent: { type: String, default: 'cyan' }
})
</script>

<style scoped>
.dialog-header-custom {
  display: flex; align-items: center; gap: 10px;
  font-family: var(--font-heading); font-size: 16px; font-weight: 600;
  letter-spacing: 0.04em; color: var(--text-primary);
}
.dh-icon :deep(svg) { width: 22px; height: 22px; }
.accent-cyan .dh-icon :deep(svg) { color: var(--accent-cyan); }
.accent-red .dh-icon :deep(svg) { color: var(--accent-red); }
</style>
