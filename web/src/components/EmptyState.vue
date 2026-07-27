<template>
  <!-- 空状态：统一 Blacklist / Vulns 中的空数据占位 -->
  <div class="empty-state">
    <slot name="icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        <path d="M9 12l2 2 4-4" />
      </svg>
    </slot>
    <span>{{ text }}</span>
  </div>
</template>

<script setup>
/**
 * 空状态占位
 * @prop {string} text - 说明文字（默认空）
 */
defineProps({
  text: { type: String, default: '' }
})
</script>

<style scoped>
.empty-state {
  display: flex; flex-direction: column; align-items: center; gap: 12px;
  padding: 48px 20px; color: var(--text-muted);
}
.empty-state :deep(svg) { width: 40px; height: 40px; opacity: 0.3; }
.empty-state span {
  font-family: var(--font-heading); font-size: 13px; font-weight: 500;
  letter-spacing: 0.04em; opacity: 0.5;
}
</style>
