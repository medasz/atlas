<template>
  <!-- 统一状态/徽标标签：替代原先散落在各视图中的 .status-tag / .status-pill / .item-status 等样式 -->
  <span class="status-tag" :class="['tone-' + tone, { running: pulse }]">
    <span v-if="dot" class="st-dot"></span>
    <slot>{{ text }}</slot>
  </span>
</template>

<script setup>
/**
 * 通用状态标签（胶囊样式）
 * @prop {string} text  - 标签文字
 * @prop {string} tone  - 配色：muted | cyan | amber | red | green | violet
 * @prop {boolean} dot  - 是否显示前置圆点
 * @prop {boolean} pulse- 圆点是否闪烁（用于 RUNNING 等活跃状态）
 */
defineProps({
  text: { type: String, default: '' },
  tone: { type: String, default: 'muted' },
  dot: { type: Boolean, default: false },
  pulse: { type: Boolean, default: false }
})
</script>

<style scoped>
.status-tag {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 3px 12px; border-radius: 10px;
  font-family: var(--font-heading); font-size: 10px; font-weight: 700;
  letter-spacing: 0.06em; white-space: nowrap;
}
.st-dot { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.tone-muted   { color: var(--text-muted);   background: rgba(82,96,128,0.12); }
.tone-cyan    { color: var(--accent-cyan);   background: rgba(0,212,255,0.1); }
.tone-amber   { color: var(--accent-amber);  background: rgba(245,166,35,0.1); }
.tone-red     { color: var(--accent-red);    background: rgba(255,71,87,0.12); }
.tone-green   { color: var(--accent-green);  background: rgba(0,230,118,0.08); }
.tone-violet  { color: var(--accent-violet); background: rgba(123,97,255,0.1); }
.running .st-dot { animation: pulse 1.5s ease-in-out infinite; }
</style>
