<template>
  <!-- 页面标题区：统一 Vulns / Settings 等视图的 .view-head 结构 -->
  <header class="view-head">
    <div class="view-head-text">
      <h1 class="view-title">{{ title }}</h1>
      <p v-if="sub" class="view-sub">{{ sub }}</p>
    </div>
    <div class="view-head-actions"><slot name="actions" /></div>
  </header>
</template>

<script setup>
/**
 * 页面标题头部
 * @prop {string} title - 主标题
 * @prop {string} sub   - 副标题 / 说明
 */
defineProps({
  title: { type: String, default: '' },
  sub: { type: String, default: '' }
})
</script>

<style scoped>
.view-head {
  display: flex; align-items: flex-end; justify-content: space-between;
  gap: 16px; margin-bottom: 20px; flex-wrap: wrap;
}
.view-title {
  font-family: var(--font-heading); font-size: 22px; font-weight: 700;
  letter-spacing: 0.08em; color: var(--text-primary); margin: 0 0 4px;
}
.view-sub { margin: 0; font-size: 12px; color: var(--text-muted); }
.view-head-actions { display: flex; align-items: center; gap: 12px; }

/* 小屏：标题与操作区纵向堆叠，操作区占满宽度 */
@media (max-width: 560px) {
  .view-head { flex-direction: column; align-items: stretch; }
  .view-head-actions { width: 100%; }
  .view-head-actions :deep(button) { flex: 1; justify-content: center; }
}
</style>
