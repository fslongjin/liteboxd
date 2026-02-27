<template>
  <div class="login-wrapper">
    <div class="background-pattern"></div>

    <div class="login-box">
      <div class="login-header">
        <div class="logo-container">
          <svg class="logo-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path
              d="M21 16V8C20.9996 7.64927 20.9048 7.30532 20.7251 7.00381C20.5455 6.70231 20.2872 6.45427 19.976 6.284L13.976 2.284C13.6736 2.08226 13.3188 1.97455 12.956 1.97455C12.5932 1.97455 12.2384 2.08226 11.936 2.284L5.936 6.284C5.62483 6.45427 5.36652 6.70231 5.2069 7.00381C5.04728 7.30532 4.99245 7.64927 5 8V16C5.00036 16.3507 5.09521 16.6947 5.27483 16.9962C5.45445 17.2977 5.71276 17.5457 6.024 17.716L12.024 21.716C12.3264 21.9177 12.6812 22.0254 13.044 22.0254C13.4068 22.0254 13.7616 21.9177 14.064 21.716L20.064 17.716C20.3752 17.5457 20.6335 17.2977 20.8131 16.9962C20.9927 16.6947 21.0876 16.3507 21 16Z"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M5.17004 6.3999L13 11.5999"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M13 11.6001V21.8001"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <path
              d="M20.83 6.3999L13 11.5999"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </div>
        <h2 class="login-title">LiteBoxd</h2>
        <p class="login-subtitle">轻量级沙箱环境管理系统</p>
      </div>

      <t-form
        ref="formRef"
        :data="formData"
        :rules="rules"
        @submit="onSubmit"
        class="login-form"
        layout="vertical"
      >
        <t-form-item name="username" label="用户名">
          <t-input v-model="formData.username" placeholder="请输入用户名" size="large" clearable>
            <template #prefix-icon>
              <user-icon />
            </template>
          </t-input>
        </t-form-item>

        <t-form-item name="password" label="密码" class="password-item">
          <t-input
            v-model="formData.password"
            type="password"
            placeholder="请输入密码"
            size="large"
            clearable
            @keyup.enter="onSubmit"
          >
            <template #prefix-icon>
              <lock-on-icon />
            </template>
          </t-input>
        </t-form-item>

        <t-form-item class="remember-item">
          <t-checkbox v-model="rememberMe">记住我</t-checkbox>
        </t-form-item>

        <div v-if="errorMsg" class="error-msg-container">
          <error-circle-icon class="error-icon" />
          <span>{{ errorMsg }}</span>
        </div>

        <t-form-item class="submit-item">
          <t-button
            theme="primary"
            type="submit"
            block
            size="large"
            :loading="loading"
            class="login-btn"
          >
            登录系统
          </t-button>
        </t-form-item>
      </t-form>

      <div class="login-footer">
        <span>LiteBoxd © 2025</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authApi } from '../api/auth'
import { UserIcon, LockOnIcon, ErrorCircleIcon } from 'tdesign-icons-vue-next'
import { MessagePlugin } from 'tdesign-vue-next'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const errorMsg = ref('')
const rememberMe = ref(false)

const formData = reactive({
  username: '',
  password: '',
})

const rules = {
  username: [{ required: true, message: '请输入用户名' }],
  password: [{ required: true, message: '请输入密码' }],
}

const onSubmit = async () => {
  if (!formData.username || !formData.password) return

  loading.value = true
  errorMsg.value = ''

  try {
    await authApi.login({ username: formData.username, password: formData.password })
    MessagePlugin.success('登录成功')
    const redirect = (route.query.redirect as string) || '/'
    router.push(redirect)
  } catch (err: any) {
    errorMsg.value = err?.response?.data?.error || '登录失败，请检查用户名和密码'
    // Shake effect could be added here
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* Modern Reset & Base */
.login-wrapper {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  width: 100%;
  background: #0f172a; /* Fallback */
  background: radial-gradient(circle at 50% 0%, #1e293b 0%, #0f172a 100%);
  overflow: hidden;
  font-family:
    'Inter',
    -apple-system,
    BlinkMacSystemFont,
    'Segoe UI',
    Roboto,
    sans-serif;
}

/* Background Pattern */
.background-pattern {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 30px 30px;
  mask-image: radial-gradient(circle at center, black 40%, transparent 100%);
  z-index: 0;
}

.login-box {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 420px;
  padding: 48px 40px;
  background: #ffffff;
  border-radius: 16px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.25),
    0 0 0 1px rgba(255, 255, 255, 0.1);
  transition: transform 0.3s ease;
  border-top: 4px solid var(--td-brand-color);
}

/* Dark mode adjustments if needed, though we enforce a specific look here */
@media (prefers-color-scheme: dark) {
  .login-box {
    background: #1e293b;
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-top: 4px solid var(--td-brand-color);
  }

  .login-title {
    color: #f8fafc !important;
  }

  .login-subtitle {
    color: #94a3b8 !important;
  }

  .login-footer {
    color: #64748b !important;
  }
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.logo-container {
  display: flex;
  justify-content: center;
  margin-bottom: 24px;
}

.logo-icon {
  width: 56px;
  height: 56px;
  color: var(--td-brand-color);
  filter: drop-shadow(0 4px 6px rgba(0, 82, 217, 0.3));
}

.login-title {
  margin: 0;
  font-size: 28px;
  font-weight: 700;
  color: #0f172a;
  letter-spacing: -0.02em;
}

.login-subtitle {
  margin: 8px 0 0;
  font-size: 14px;
  color: #64748b;
  font-weight: 500;
}

.login-form {
  margin-bottom: 24px;
}

:deep(.remember-item) {
  margin-bottom: 24px;
}

:deep(.password-item) {
  margin-bottom: 12px;
}

/* TDesign Overrides for "Pro" feel */
:deep(.t-form__label) {
  color: #334155;
  font-weight: 600;
  font-size: 14px;
  padding-bottom: 6px;
}

@media (prefers-color-scheme: dark) {
  :deep(.t-form__label) {
    color: #cbd5e1;
  }
}

:deep(.t-input) {
  border-radius: 8px;
  background-color: #f1f5f9;
  border: 1px solid #e2e8f0;
  transition: all 0.2s;
  padding-left: 12px;
}

@media (prefers-color-scheme: dark) {
  :deep(.t-input) {
    background-color: #334155;
    border-color: #475569;
    color: #f8fafc;
  }
}

:deep(.t-input:hover) {
  background-color: #e2e8f0;
  border-color: #94a3b8;
}

@media (prefers-color-scheme: dark) {
  :deep(.t-input:hover) {
    background-color: #475569;
    border-color: #64748b;
  }
}

:deep(.t-input__inner) {
  font-size: 15px;
}

@media (prefers-color-scheme: dark) {
  :deep(.t-input__inner) {
    color: #f8fafc;
  }
}

:deep(.t-input--focused) {
  background-color: #ffffff;
  box-shadow: 0 0 0 3px rgba(0, 82, 217, 0.15);
  border-color: var(--td-brand-color);
}

@media (prefers-color-scheme: dark) {
  :deep(.t-input--focused) {
    background-color: #1e293b;
    box-shadow: 0 0 0 3px rgba(0, 82, 217, 0.25);
  }
}

:deep(.t-form-item) {
  margin-bottom: 24px;
}

.login-btn {
  height: 48px;
  font-size: 16px;
  font-weight: 600;
  border-radius: 8px;
  letter-spacing: 0.02em;
  background: linear-gradient(to right, var(--td-brand-color), #266eff);
  border: none;
  box-shadow: 0 4px 12px rgba(0, 82, 217, 0.2);
  transition: all 0.3s ease;
}

.login-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px rgba(0, 82, 217, 0.3);
}

.login-btn:active {
  transform: translateY(0);
}

.error-msg-container {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px;
  background: rgba(213, 73, 65, 0.1);
  border-radius: 6px;
  color: var(--td-error-color);
  font-size: 13px;
  margin-bottom: 20px;
  animation: slideDown 0.3s ease;
}

.error-icon {
  font-size: 16px;
}

.login-footer {
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
  margin-top: 32px;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
