<template>
  <!-- 主操作按钮：统一 Vulns / Settings 中重复的 .btn-save + .spinner 模式 -->
  <button class="primary-btn" :disabled="loading" @click="$emit('click', $event)">
    <span v-if="loading" class="pb-spinner"></span>
    <slot>{{ loading ? loadingText : text }}</slot>
  </button>
</template>

<script setup>
/**
 * 主操作按钮（带加载态）
 * @prop {string} text       - 默认文字
 * @prop {string} loadingText- 加载时文字
 * @prop {boolean} loading   - 是否加载中（禁用 + 显示转圈）
 * @emit click - 点击事件
 */
defineProps({
  text: { type: String, default: '' },
  loadingText: { type: String, default: '' },
  loading: { type: Boolean, default: false }
})
defineEmits(['click'])
</script>

<style scoped>
.primary-btn {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 11px 26px; border-radius: var(--radius-md); border: none;
  background: linear-gradient(135deg, var(--accent-cyan), #0098c7);
  color: #04121c; font-family: var(--font-heading); font-size: 13px;
  font-weight: 700; letter-spacing: 0.06em; cursor: pointer;
  transition: all 0.2s ease; box-shadow: 0 4px 16px rgba(0,212,255,0.25);
}
.primary-btn:hover:not(:disabled) { box-shadow: 0 6px 22px rgba(0,212,255,0.4); }
.primary-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.pb-spinner {
  width: 13px; height: 13px; border-radius: 50%;
  border: 2px solid rgba(4,18,28,0.3); border-top-color: #04121c;
  animation: spin 0.7s linear infinite;
}
</style>
