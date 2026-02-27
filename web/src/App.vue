<template>
  <!-- Full-screen pages without layout -->
  <router-view v-if="route.meta.hideLayout" />

  <!-- Normal pages with layout -->
  <t-layout v-else>
    <t-header class="app-header">
      <div class="logo">
        <h1>LiteBoxd</h1>
      </div>
      <div class="nav-links">
        <t-button
          variant="text"
          :class="['nav-link', { active: route.path === '/sandboxes' }]"
          @click="navigateTo('/sandboxes')"
        >
          Sandboxes
        </t-button>
        <t-button
          variant="text"
          :class="['nav-link', { active: route.path.startsWith('/templates') }]"
          @click="navigateTo('/templates')"
        >
          模板管理
        </t-button>
        <t-button
          variant="text"
          :class="['nav-link', { active: route.path.startsWith('/metadata') }]"
          @click="navigateTo('/metadata/sandboxes')"
        >
          元数据记录
        </t-button>
      </div>
      <div class="header-right">
        <t-dropdown :options="userMenuOptions" @click="onUserMenuClick">
          <t-button variant="text" class="user-btn">
            {{ username || 'admin' }}
            <template #suffix>
              <chevron-down-icon />
            </template>
          </t-button>
        </t-dropdown>
      </div>
    </t-header>
    <t-content class="app-content">
      <router-view />
    </t-content>
    <t-footer class="app-footer">
      <a href="https://github.com/fslongjin/liteboxd" target="_blank" rel="noopener"> GitHub </a>
      <span class="separator">|</span>
      <span>LiteBoxd &copy; 2025</span>
    </t-footer>
  </t-layout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ChevronDownIcon } from 'tdesign-icons-vue-next'
import { authApi } from './api/auth'

const router = useRouter()
const route = useRoute()
const username = ref('')

const userMenuOptions = [
  { content: 'API Keys', value: 'api-keys' },
  { content: '登出', value: 'logout' },
]

const navigateTo = (path: string) => {
  router.push(path)
}

const onUserMenuClick = async (data: { value: string }) => {
  if (data.value === 'api-keys') {
    router.push('/settings/api-keys')
  } else if (data.value === 'logout') {
    try {
      await authApi.logout()
    } catch {
      // ignore
    }
    router.push('/login')
  }
}

onMounted(async () => {
  try {
    const res = await authApi.me()
    username.value = res.data.username || ''
  } catch {
    // Not logged in — router guard will handle redirect
  }
})
</script>

<style>
body {
  margin: 0;
  padding: 0;
}

.app-header {
  display: flex;
  align-items: center;
  padding: 0 24px;
  background: var(--td-brand-color);
}

.logo {
  margin-right: 48px;
}

.logo h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: white;
}

.nav-links {
  display: flex;
  gap: 24px;
  flex: 1;
}

.nav-link {
  color: rgba(255, 255, 255, 0.8) !important;
  font-size: 14px;
  border-radius: 4px;
  transition: all 0.2s;
}

.nav-link:hover {
  background: rgba(255, 255, 255, 0.15) !important;
  color: white !important;
}

.nav-link.active {
  background: rgba(255, 255, 255, 0.2) !important;
  color: white !important;
}

.header-right {
  margin-left: auto;
}

.user-btn {
  color: rgba(255, 255, 255, 0.9) !important;
}

.user-btn:hover {
  color: white !important;
  background: rgba(255, 255, 255, 0.15) !important;
}

.app-content {
  min-height: calc(100vh - 64px - 40px);
  background: var(--td-bg-color-page);
}

.app-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 0 24px;
  height: 40px;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-border);
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.app-footer a {
  color: var(--td-brand-color);
  text-decoration: none;
}

.app-footer a:hover {
  text-decoration: underline;
}

.separator {
  color: var(--td-text-color-placeholder);
}
</style>
