import { createRouter, createWebHistory } from 'vue-router'
import { authApi } from '../api/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
      meta: { public: true, hideLayout: true },
    },
    {
      path: '/',
      name: 'home',
      redirect: '/sandboxes',
    },
    {
      path: '/sandboxes',
      name: 'sandboxes',
      component: () => import('../views/SandboxList.vue'),
    },
    {
      path: '/sandboxes/:id',
      name: 'sandbox-detail',
      component: () => import('../views/SandboxDetail.vue'),
    },
    {
      path: '/sandboxes/:id/terminal',
      name: 'sandbox-terminal',
      component: () => import('../views/SandboxTerminal.vue'),
      meta: { hideLayout: true },
    },
    {
      path: '/templates',
      name: 'templates',
      component: () => import('../views/TemplateList.vue'),
    },
    {
      path: '/templates/:name',
      name: 'template-detail',
      component: () => import('../views/TemplateDetail.vue'),
    },
    {
      path: '/metadata/sandboxes',
      name: 'metadata-sandboxes',
      component: () => import('../views/MetadataSandboxList.vue'),
    },
    {
      path: '/metadata/sandboxes/:id',
      name: 'metadata-sandbox-detail',
      component: () => import('../views/MetadataSandboxDetail.vue'),
    },
    {
      path: '/metadata/reconcile',
      name: 'metadata-reconcile',
      component: () => import('../views/MetadataReconcile.vue'),
    },
    {
      path: '/settings/api-keys',
      name: 'api-keys',
      component: () => import('../views/APIKeys.vue'),
    },
  ],
})

// Navigation guard: redirect to login if not authenticated
router.beforeEach(async (to, _from, next) => {
  if (to.meta.public) {
    next()
    return
  }
  try {
    await authApi.me()
    next()
  } catch {
    next({ name: 'login', query: { redirect: to.fullPath } })
  }
})

export default router
