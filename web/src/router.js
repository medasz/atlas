import { createRouter, createWebHashHistory } from 'vue-router'
import Login from './views/Login.vue'
import Layout from './views/Layout.vue'
import Assets from './views/Assets.vue'
import IPAggregate from './views/IPAggregate.vue'
import Tasks from './views/Tasks.vue'
import Blacklist from './views/Blacklist.vue'
import Settings from './views/Settings.vue'
import Vulns from './views/Vulns.vue'
import Audit from './views/Audit.vue'

const routes = [
  { path: '/login', component: Login },
  {
    path: '/',
    component: Layout,
    children: [
      { path: '', redirect: '/assets' },
      { path: 'assets', component: Assets },
      { path: 'assets/:ip/aggregate', component: IPAggregate, props: true },
      { path: 'tasks', component: Tasks },
      { path: 'vulns', component: Vulns },
      { path: 'blacklist', component: Blacklist },
      { path: 'audit', component: Audit },
      { path: 'settings', component: Settings }
    ]
  }
]

const router = createRouter({ history: createWebHashHistory(), routes })

// 路由守卫：未登录跳转登录页
router.beforeEach((to) => {
  const authed = localStorage.getItem('atlas_authed') === '1'
  if (to.path !== '/login' && !authed) return '/login'
  if (to.path === '/login' && authed) return '/'
  return true
})

export default router
