<template>
  <!-- 提示横幅：统一 Vulns / Settings 的 .banner 样式 -->
  <div class="banner" :class="'banner-' + type"><slot /></div>
</template>

<script setup>
/**
 * 提示横幅
 * @prop {string} type - 类型：error | warn | info
 */
defineProps({
  type: { type: String, default: 'error' }
})
</script>

<style scoped>
.banner {
  padding: 10px 14px; border-radius: var(--radius-md);
  margin-bottom: 16px; font-family: var(--font-body); font-size: 12px;
}
.banner-error { background: rgba(255,71,87,0.1); border: 1px solid rgba(255,71,87,0.3); color: var(--accent-red); }
.banner-warn  { background: rgba(245,166,35,0.1); border: 1px solid rgba(245,166,35,0.3); color: var(--accent-amber); }
.banner-info  { background: rgba(0,212,255,0.08); border: 1px solid rgba(0,212,255,0.25); color: var(--accent-cyan); }
</style>
