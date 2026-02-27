<template>
  <div class="api-keys-page">
    <t-card title="API Keys" :bordered="false">
      <template #actions>
        <t-button theme="primary" @click="showCreate = true">创建 API Key</t-button>
      </template>

      <t-table :data="keys" :columns="columns" :loading="loading" row-key="id" hover>
        <template #prefix="{ row }">
          <code>lbxk_{{ row.prefix }}...</code>
        </template>
        <template #expires_at="{ row }">
          <template v-if="row.expires_at">
            <t-tag v-if="isExpired(row.expires_at)" theme="danger" size="small">已过期</t-tag>
            <span v-else>{{ formatTime(row.expires_at) }}</span>
          </template>
          <span v-else class="text-secondary">永不过期</span>
        </template>
        <template #last_used_at="{ row }">
          <span v-if="row.last_used_at">{{ formatTime(row.last_used_at) }}</span>
          <span v-else class="text-secondary">从未使用</span>
        </template>
        <template #created_at="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
        <template #operation="{ row }">
          <t-popconfirm
            content="确定要删除该 API Key 吗？删除后立即失效。"
            @confirm="deleteKey(row.id)"
          >
            <t-link theme="danger">删除</t-link>
          </t-popconfirm>
        </template>
      </t-table>
    </t-card>

    <!-- Create Dialog -->
    <t-dialog
      v-model:visible="showCreate"
      header="创建 API Key"
      :confirm-btn="{ content: '创建', loading: creating }"
      @confirm="createKey"
    >
      <t-form :data="createForm">
        <t-form-item label="名称" name="name">
          <t-input v-model="createForm.name" placeholder="例如：ci-pipeline, dev-machine" />
        </t-form-item>
        <t-form-item label="有效期（天）" name="expires_in_days">
          <t-input-number
            v-model="createForm.expires_in_days"
            :min="0"
            placeholder="留空或 0 表示永不过期"
            style="width: 100%"
          />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- Show Key Dialog -->
    <t-dialog
      v-model:visible="showKeyDialog"
      header="API Key 已创建"
      :cancel-btn="null"
      :confirm-btn="{ content: '我已复制' }"
      :close-on-overlay-click="false"
      @confirm="showKeyDialog = false"
    >
      <t-alert
        theme="warning"
        message="请立即复制此 Key，关闭后将无法再次查看！"
        style="margin-bottom: 16px"
      />
      <div class="key-display">
        <code>{{ newKey }}</code>
        <t-button size="small" variant="outline" @click="copyKey">复制</t-button>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { authApi, type APIKey } from '../api/auth'

const loading = ref(false)
const creating = ref(false)
const showCreate = ref(false)
const showKeyDialog = ref(false)
const newKey = ref('')
const keys = ref<APIKey[]>([])

const createForm = reactive({
  name: '',
  expires_in_days: undefined as number | undefined,
})

const columns = [
  { colKey: 'name', title: '名称', width: 180 },
  { colKey: 'prefix', title: 'Key 前缀', width: 180 },
  { colKey: 'expires_at', title: '过期时间', width: 180 },
  { colKey: 'last_used_at', title: '最近使用', width: 180 },
  { colKey: 'created_at', title: '创建时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 100 },
]

const formatTime = (t: string) => {
  return new Date(t).toLocaleString('zh-CN')
}

const isExpired = (t: string) => {
  return new Date(t) < new Date()
}

const loadKeys = async () => {
  loading.value = true
  try {
    const res = await authApi.listAPIKeys()
    keys.value = res.data
  } catch {
    MessagePlugin.error('加载 API Keys 失败')
  } finally {
    loading.value = false
  }
}

const createKey = async () => {
  if (!createForm.name) {
    MessagePlugin.warning('请输入名称')
    return
  }
  creating.value = true
  try {
    const res = await authApi.createAPIKey({
      name: createForm.name,
      expires_in_days:
        createForm.expires_in_days && createForm.expires_in_days > 0
          ? createForm.expires_in_days
          : undefined,
    })
    newKey.value = res.data.key || ''
    showCreate.value = false
    showKeyDialog.value = true
    createForm.name = ''
    createForm.expires_in_days = undefined
    await loadKeys()
  } catch {
    MessagePlugin.error('创建 API Key 失败')
  } finally {
    creating.value = false
  }
}

const deleteKey = async (id: string) => {
  try {
    await authApi.deleteAPIKey(id)
    MessagePlugin.success('已删除')
    await loadKeys()
  } catch {
    MessagePlugin.error('删除失败')
  }
}

const copyKey = async () => {
  try {
    await navigator.clipboard.writeText(newKey.value)
    MessagePlugin.success('已复制到剪贴板')
  } catch {
    MessagePlugin.warning('复制失败，请手动复制')
  }
}

onMounted(loadKeys)
</script>

<style scoped>
.api-keys-page {
  padding: 24px;
}

.text-secondary {
  color: var(--td-text-color-secondary);
}

.key-display {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 4px;
  word-break: break-all;
}

.key-display code {
  flex: 1;
  font-size: 13px;
}
</style>
