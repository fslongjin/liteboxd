<template>
  <div class="change-password-page">
    <div class="page-header">
      <h2 class="page-title">修改密码</h2>
      <p class="page-description">
        为了您的账户安全，建议定期更换密码。新密码必须包含至少 8 个字符。
      </p>
    </div>

    <t-card :bordered="false" class="password-card">
      <t-form
        ref="form"
        :data="formData"
        :rules="rules"
        label-align="top"
        @submit="onSubmit"
        class="password-form"
      >
        <t-form-item label="当前密码" name="old_password">
          <t-input
            v-model="formData.old_password"
            type="password"
            placeholder="请输入当前使用的密码"
            size="large"
          >
            <template #prefix-icon>
              <lock-on-icon />
            </template>
          </t-input>
        </t-form-item>

        <t-form-item label="新密码" name="new_password">
          <div class="input-wrapper">
            <t-input
              v-model="formData.new_password"
              type="password"
              placeholder="设置新密码"
              size="large"
            >
              <template #prefix-icon>
                <lock-on-icon />
              </template>
            </t-input>
            <div v-if="formData.new_password" class="password-strength">
              <div class="strength-bar-container">
                <div
                  class="strength-bar"
                  :class="strengthClass"
                  :style="{ width: strengthPercent + '%' }"
                ></div>
              </div>
              <span class="strength-text">{{ strengthText }}</span>
            </div>
          </div>
        </t-form-item>

        <t-form-item label="确认新密码" name="confirm_password">
          <t-input
            v-model="formData.confirm_password"
            type="password"
            placeholder="请再次输入新密码"
            size="large"
          >
            <template #prefix-icon>
              <lock-on-icon />
            </template>
          </t-input>
        </t-form-item>

        <div class="form-actions">
          <t-button
            theme="default"
            variant="base"
            size="large"
            @click="router.back()"
            class="cancel-btn"
          >
            取消
          </t-button>
          <t-button
            theme="primary"
            type="submit"
            :loading="loading"
            size="large"
            class="submit-btn"
          >
            确认修改
          </t-button>
        </div>
      </t-form>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { LockOnIcon } from 'tdesign-icons-vue-next'
import { authApi } from '../api/auth'
import { useRouter } from 'vue-router'

const router = useRouter()
const loading = ref(false)
const strengthPercent = ref(0)
const strengthClass = ref('')
const strengthText = ref('')

const formData = reactive({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

watch(
  () => formData.new_password,
  (val) => {
    checkPasswordStrength(val)
  }
)

const rules = {
  old_password: [{ required: true, message: '请输入旧密码', type: 'error' }],
  new_password: [
    { required: true, message: '请输入新密码', type: 'error' },
    { min: 8, message: '密码长度至少 8 位', type: 'warning' },
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码', type: 'error' },
    {
      validator: (val: string) => val === formData.new_password,
      message: '两次输入的密码不一致',
      type: 'error',
    },
  ],
}

const checkPasswordStrength = (val: string) => {
  if (!val) {
    strengthPercent.value = 0
    strengthText.value = ''
    return
  }

  let score = 0
  if (val.length >= 8) score += 1
  if (/[A-Z]/.test(val)) score += 1
  if (/[a-z]/.test(val)) score += 1
  if (/[0-9]/.test(val)) score += 1
  if (/[^A-Za-z0-9]/.test(val)) score += 1

  if (score < 2) {
    strengthPercent.value = 33
    strengthClass.value = 'weak'
    strengthText.value = '弱'
  } else if (score < 4) {
    strengthPercent.value = 66
    strengthClass.value = 'medium'
    strengthText.value = '中'
  } else {
    strengthPercent.value = 100
    strengthClass.value = 'strong'
    strengthText.value = '强'
  }
}

const onSubmit = async ({ validateResult }: any) => {
  if (validateResult === true) {
    loading.value = true
    try {
      await authApi.changePassword({
        old_password: formData.old_password,
        new_password: formData.new_password,
      })
      MessagePlugin.success('密码修改成功，请重新登录')
      await authApi.logout()
      router.push('/login')
    } catch (err: any) {
      console.error('修改密码失败:', err)
      MessagePlugin.error(err.response?.data?.error || '修改密码失败')
    } finally {
      loading.value = false
    }
  }
}
</script>

<style scoped>
.change-password-page {
  max-width: 540px;
  margin: 48px auto;
  padding: 0 24px;
}

.page-header {
  text-align: center;
  margin-bottom: 32px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin: 0 0 8px;
}

.page-description {
  color: var(--td-text-color-secondary);
  font-size: 14px;
  line-height: 1.5;
}

.password-card {
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.04);
  overflow: hidden;
}

.input-wrapper {
  width: 100%;
}

.password-strength {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.strength-bar-container {
  flex: 1;
  height: 4px;
  background-color: var(--td-bg-color-component);
  border-radius: 2px;
  overflow: hidden;
}

.strength-bar {
  height: 100%;
  border-radius: 2px;
  transition: all 0.3s ease;
}

.strength-bar.weak {
  background-color: var(--td-error-color);
}

.strength-bar.medium {
  background-color: var(--td-warning-color);
}

.strength-bar.strong {
  background-color: var(--td-success-color);
}

.strength-text {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  min-width: 24px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 16px;
  margin-top: 24px;
  padding-top: 8px;
}

.submit-btn {
  min-width: 120px;
}

:deep(.t-card__body) {
  padding: 32px;
}

@media (max-width: 640px) {
  .change-password-page {
    margin: 24px auto;
    padding: 0 16px;
  }

  :deep(.t-card__body) {
    padding: 24px;
  }
}
</style>
