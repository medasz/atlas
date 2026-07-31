<template>
  <div class="layout-shell" :class="{ 'nav-open': navOpen }">
    <!-- 移动端遮罩 -->
    <div class="sidebar-backdrop" @click="navOpen = false"></div>

    <!-- 侧边栏导航 -->
    <nav class="sidebar" :class="{ open: navOpen }">
      <router-link to="/assets" class="sidebar-brand">
        <svg viewBox="0 0 32 32" fill="none">
          <circle cx="16" cy="16" r="14" stroke="currentColor" stroke-width="1.2"/>
          <circle cx="16" cy="16" r="5" stroke="currentColor" stroke-width="1" stroke-dasharray="3 2"/>
          <line x1="16" y1="2" x2="16" y2="8" stroke="currentColor" stroke-width="1"/>
        </svg>
        <span class="brand-name">ATLAS</span>
      </router-link>

      <div class="nav-group">
        <router-link v-for="item in navItems" :key="item.path" :to="item.path"
          class="nav-item" :class="{ active: isActive(item.path) }">
          <span class="nav-icon" v-html="item.icon"></span>
          <span class="nav-label">{{ item.label }}</span>
          <span class="nav-indicator"></span>
        </router-link>
      </div>

      <div class="sidebar-footer">
        <button class="logout-btn" @click="doLogout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4"/>
            <polyline points="16,17 21,12 16,7"/>
            <line x1="21" y1="12" x2="9" y2="12"/>
          </svg>
          <span>退出</span>
        </button>
      </div>
    </nav>

    <!-- 主区域 -->
    <div class="main-area">
      <header class="topbar">
        <div class="topbar-left">
          <button class="nav-toggle" @click="navOpen = !navOpen" aria-label="切换导航">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="3" y1="6" x2="21" y2="6"/>
              <line x1="3" y1="12" x2="21" y2="12"/>
              <line x1="3" y1="18" x2="21" y2="18"/>
            </svg>
          </button>
          <div class="topbar-path">
            <span class="path-segment" v-for="(seg, i) in breadcrumbs" :key="i">
              <span v-if="i > 0" class="path-divider">/</span>
              <span :class="{ active: i === breadcrumbs.length - 1 }">{{ seg }}</span>
            </span>
          </div>
        </div>
        <div class="topbar-status">
          <span class="status-dot"></span>
          <span class="status-text">系统就绪</span>
        </div>
      </header>
      <main class="content-area">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const route = useRoute()

// 移动端侧边栏抽屉开关
const navOpen = ref(false)
// 路由切换后自动收起抽屉
watch(() => route.path, () => { navOpen.value = false })

const navItems = [
  {
    path: '/assets', label: '资产检索',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="8" y1="11" x2="14" y2="11"/><line x1="11" y1="8" x2="11" y2="14"/></svg>'
  },
  {
    path: '/tasks', label: '任务管理',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="3"/><circle cx="9" cy="9" r="2"/><path d="M21 15l-5-5L5 21"/></svg>'
  },
  {
    path: '/vulns', label: '漏洞管理',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>'
  },
  {
    path: '/blacklist', label: '黑名单',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="8" y1="10" x2="16" y2="14"/><line x1="16" y1="10" x2="8" y2="14"/></svg>'
  },
  {
    path: '/audit', label: '审计日志',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14,2 14,8 20,8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10,9 9,9 8,9"/></svg>'
  },
  {
    path: '/settings', label: '系统设置',
    icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 112.83-2.83l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 112.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z"/></svg>'
  }
]

const breadcrumbs = computed(() => {
  const p = route.path
  const map = { '/assets': '资产检索', '/tasks': '任务管理', '/vulns': '漏洞管理', '/blacklist': '黑名单', '/audit': '审计日志', '/settings': '系统设置' }
  return map[p] ? ['首页', map[p]] : ['首页']
})

function isActive(path) { return route.path === path }

async function doLogout() {
  await api.logout().catch(() => {})
  localStorage.removeItem('atlas_authed')
  router.push('/login')
}
</script>

<style scoped>
.layout-shell { display: flex; height: 100vh; overflow: hidden; }

/* ===== Sidebar ===== */
.sidebar {
  width: 220px; flex-shrink: 0;
  display: flex; flex-direction: column;
  background: var(--bg-surface);
  border-right: 1px solid var(--border-subtle);
  position: relative;
}
.sidebar::after {
  content: ''; position: absolute; right: 0; top: 0; bottom: 0; width: 1px;
  background: linear-gradient(180deg, transparent, rgba(0,212,255,0.2) 50%, transparent);
  opacity: 0.5; pointer-events: none;
}

.sidebar-brand {
  display: flex; align-items: center; gap: 10px;
  padding: 20px 18px 12px;
  text-decoration: none; color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
}
.sidebar-brand svg { width: 28px; height: 28px; color: var(--accent-cyan); }
.brand-name {
  font-family: var(--font-heading); font-size: 20px; font-weight: 700;
  letter-spacing: 0.14em; color: var(--text-primary);
}

.nav-group { flex: 1; padding: 16px 10px; display: flex; flex-direction: column; gap: 4px; }

.nav-item {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 14px; border-radius: var(--radius-md);
  text-decoration: none; cursor: pointer;
  position: relative; transition: all 0.2s ease;
  color: var(--text-secondary);
}
.nav-item:hover { background: var(--bg-hover); color: var(--text-primary); }
.nav-item.active {
  background: rgba(0,212,255,0.08);
  color: var(--accent-cyan);
}
.nav-indicator {
  position: absolute; left: 0; top: 50%; transform: translateY(-50%);
  width: 2px; height: 0; background: var(--accent-cyan);
  border-radius: 0 2px 2px 0;
  transition: height 0.25s ease;
  box-shadow: 0 0 8px var(--accent-cyan);
}
.nav-item.active .nav-indicator { height: 60%; }

.nav-icon { width: 20px; height: 20px; display: flex; align-items: center; flex-shrink: 0; }
.nav-icon :deep(svg) { width: 20px; height: 20px; }

.nav-label {
  font-family: var(--font-heading); font-size: 13px; font-weight: 600;
  letter-spacing: 0.05em;
}

.sidebar-footer { padding: 12px 10px 16px; border-top: 1px solid var(--border-subtle); }

.logout-btn {
  display: flex; align-items: center; gap: 8px;
  width: 100%; padding: 10px 14px;
  background: transparent; border: 1px solid transparent; border-radius: var(--radius-md);
  color: var(--text-muted); font-family: var(--font-heading); font-size: 12px;
  font-weight: 600; letter-spacing: 0.05em;
  cursor: pointer; transition: all 0.2s ease;
}
.logout-btn svg { width: 16px; height: 16px; }
.logout-btn:hover {
  background: rgba(255,71,87,0.08); border-color: rgba(255,71,87,0.2);
  color: var(--accent-red);
}

/* ===== Main Area ===== */
.main-area { flex: 1; display: flex; flex-direction: column; min-width: 0; }

.topbar {
  height: 52px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 24px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
}

.topbar-left { display: flex; align-items: center; gap: 12px; }
.nav-toggle {
  display: none; align-items: center; justify-content: center;
  width: 32px; height: 32px; border-radius: var(--radius-sm);
  background: transparent; border: 1px solid var(--border-subtle);
  color: var(--text-secondary); cursor: pointer; transition: all 0.2s ease;
}
.nav-toggle svg { width: 18px; height: 18px; }
.nav-toggle:hover { color: var(--accent-cyan); border-color: var(--accent-cyan); }

.topbar-path {
  display: flex; align-items: center; gap: 2px;
  font-family: var(--font-heading); font-size: 12px; font-weight: 600;
  letter-spacing: 0.06em; color: var(--text-muted);
}
.path-divider { color: var(--border-active); margin: 0 4px; }
.path-segment span.active { color: var(--accent-cyan); }

.topbar-status {
  display: flex; align-items: center; gap: 8px;
  font-family: var(--font-heading); font-size: 11px; font-weight: 600;
  letter-spacing: 0.08em; color: var(--text-muted);
}
.status-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--accent-green);
  box-shadow: 0 0 8px rgba(0,230,118,0.6);
  animation: pulse 2s ease-in-out infinite;
}

.content-area {
  flex: 1; overflow-y: auto; padding: 24px;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* ===== 响应式：平板及以下保持侧栏可见；手机切换为抽屉 ===== */
@media (max-width: 768px) {
  .sidebar {
    position: fixed; left: 0; top: 0; bottom: 0; z-index: 40;
    transform: translateX(-100%);
    transition: transform 0.28s ease;
    box-shadow: var(--shadow-card);
  }
  .sidebar.open { transform: translateX(0); }
  .sidebar-backdrop {
    display: block; position: fixed; inset: 0; z-index: 30;
    background: rgba(3,7,14,0.6); backdrop-filter: blur(2px);
  }
  .nav-toggle { display: inline-flex; }
  .content-area { padding: 16px; }
}

/* 遮罩默认隐藏（桌面端） */
.sidebar-backdrop { display: none; }
</style>
