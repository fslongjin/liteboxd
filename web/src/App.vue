<template>
  <t-layout>
    <t-header class="app-header">
      <div class="logo">
        <h1>LiteBoxd</h1>
      </div>
      <div class="nav-links">
        <t-link
          :class="['nav-link', { active: route.path === '/sandboxes' }]"
          theme="light"
          hover="color"
          @click="navigateTo('/sandboxes')"
        >
          Sandboxes
        </t-link>
        <t-link
          :class="['nav-link', { active: route.path.startsWith('/templates') }]"
          theme="light"
          hover="color"
          @click="navigateTo('/templates')"
        >
          模板管理
        </t-link>
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
import { useRouter, useRoute } from 'vue-router'

const router = useRouter()
const route = useRoute()

const navigateTo = (path: string) => {
  router.push(path)
}
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
}

.nav-link {
  color: rgba(255, 255, 255, 0.8);
  font-size: 14px;
  cursor: pointer;
  text-decoration: none;
  padding: 8px 12px;
  border-radius: 4px;
  transition: all 0.2s;
}

.nav-link:hover {
  background: rgba(255, 255, 255, 0.15);
  color: white;
}

.nav-link.active {
  background: rgba(255, 255, 255, 0.2);
  color: white;
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
