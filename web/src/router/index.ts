import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
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
      path: '/templates',
      name: 'templates',
      component: () => import('../views/TemplateList.vue'),
    },
    {
      path: '/templates/:name',
      name: 'template-detail',
      component: () => import('../views/TemplateDetail.vue'),
    },
  ],
})

export default router
