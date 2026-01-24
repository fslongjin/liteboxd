<template>
  <div class="prepull-panel">
    <div class="prepull-header">
      <t-input
        v-model="imageInput"
        placeholder="输入镜像地址，如: python:3.11-slim"
        :style="{ width: '400px' }"
        @keyup.enter="createPrepull"
      />
      <t-button theme="primary" :loading="creating" @click="createPrepull">添加预拉取任务</t-button>
    </div>

    <t-table :data="prepulls" :columns="columns" :loading="loading" row-key="id" hover size="small">
      <template #image="{ row }">
        <span class="image-text">{{ row.image }}</span>
      </template>
      <template #status="{ row }">
        <t-tag :theme="getStatusTheme(row.status)">
          {{ getStatusText(row.status) }}
        </t-tag>
      </template>
      <template #progress="{ row }">
        <t-progress
          :percentage="getProgress(row)"
          :label="false"
          size="small"
          :theme="row.status === 'completed' ? 'success' : 'default'"
        />
        <span class="progress-text">{{ row.readyNodes }} / {{ row.desiredNodes }} 节点</span>
      </template>
      <template #createdAt="{ row }">
        {{ formatTime(row.createdAt) }}
      </template>
      <template #operation="{ row }">
        <t-popconfirm content="确定要删除该预拉取任务吗？" @confirm="deletePrepull(row.id)">
          <t-link theme="danger" :disabled="row.status !== 'completed'">删除</t-link>
        </t-popconfirm>
      </template>
    </t-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { templateApi, type PrepullTask } from '../api/template'

const prepulls = ref<PrepullTask[]>([])
const loading = ref(false)
const creating = ref(false)
const imageInput = ref('')

const columns = [
  { colKey: 'image', title: '镜像', ellipsis: true },
  { colKey: 'status', title: '状态', width: 100 },
  { colKey: 'progress', title: '进度', width: 200 },
  { colKey: 'createdAt', title: '创建时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 80 },
]

const getStatusTheme = (status: string) => {
  switch (status) {
    case 'completed':
      return 'success'
    case 'pending':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'default'
  }
}

const getStatusText = (status: string) => {
  switch (status) {
    case 'pending':
      return '进行中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '失败'
    default:
      return status
  }
}

const getProgress = (row: PrepullTask) => {
  if (row.desiredNodes === 0) return 0
  return Math.round((row.readyNodes / row.desiredNodes) * 100)
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const loadPrepulls = async () => {
  loading.value = true
  try {
    const res = await templateApi.listPrepulls()
    prepulls.value = res.data.items || []
  } catch (err: any) {
    console.error('Failed to load prepulls:', err)
  } finally {
    loading.value = false
  }
}

const createPrepull = async () => {
  if (!imageInput.value.trim()) {
    MessagePlugin.warning('请输入镜像地址')
    return
  }

  creating.value = true
  try {
    await templateApi.createPrepull({ image: imageInput.value })
    MessagePlugin.success('预拉取任务已创建')
    imageInput.value = ''
    loadPrepulls()
  } catch (err: any) {
    MessagePlugin.error('创建失败: ' + (err.response?.data?.error?.message || err.message))
  } finally {
    creating.value = false
  }
}

const deletePrepull = async (id: string) => {
  try {
    await templateApi.deletePrepull(id)
    MessagePlugin.success('删除成功')
    loadPrepulls()
  } catch (err: any) {
    MessagePlugin.error('删除失败: ' + (err.response?.data?.error?.message || err.message))
  }
}

let refreshInterval: number

onMounted(() => {
  loadPrepulls()
  refreshInterval = window.setInterval(loadPrepulls, 5000)
})

onUnmounted(() => {
  clearInterval(refreshInterval)
})
</script>

<style scoped>
.prepull-panel {
  padding: 16px 0;
}

.prepull-header {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.image-text {
  font-family: monospace;
  font-size: 12px;
}

.progress-text {
  margin-left: 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>
