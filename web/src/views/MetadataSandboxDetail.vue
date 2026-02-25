<template>
  <div class="metadata-sandbox-detail">
    <t-card :bordered="false">
      <template #title>
        <t-space align="center">
          <t-link @click="$router.push('/metadata/sandboxes')">
            <t-icon name="chevron-left" />
          </t-link>
          <span>元数据详情: {{ sandboxId }}</span>
        </t-space>
      </template>

      <t-loading :loading="loading">
        <t-descriptions v-if="sandbox" :column="2">
          <t-descriptions-item label="ID">{{ sandbox.id }}</t-descriptions-item>
          <t-descriptions-item label="模板">{{ sandbox.template || '-' }}</t-descriptions-item>
          <t-descriptions-item label="模板版本">{{
            sandbox.templateVersion || '-'
          }}</t-descriptions-item>
          <t-descriptions-item label="期望状态">{{
            sandbox.desired_state || '-'
          }}</t-descriptions-item>
          <t-descriptions-item label="生命周期">{{
            sandbox.lifecycle_status || sandbox.status
          }}</t-descriptions-item>
          <t-descriptions-item label="Pod Phase">{{
            sandbox.pod_phase || '-'
          }}</t-descriptions-item>
          <t-descriptions-item label="Pod IP">{{ sandbox.pod_ip || '-' }}</t-descriptions-item>
          <t-descriptions-item label="状态原因">{{
            sandbox.status_reason || '-'
          }}</t-descriptions-item>
          <t-descriptions-item label="创建时间">{{ fmt(sandbox.created_at) }}</t-descriptions-item>
          <t-descriptions-item label="更新时间">{{ fmt(sandbox.updated_at) }}</t-descriptions-item>
          <t-descriptions-item label="最近观测">{{
            fmt(sandbox.last_seen_at)
          }}</t-descriptions-item>
          <t-descriptions-item label="删除时间">{{ fmt(sandbox.deleted_at) }}</t-descriptions-item>
        </t-descriptions>
      </t-loading>
    </t-card>

    <t-card title="状态流水" :bordered="false" class="history-card">
      <template #actions>
        <t-space>
          <t-input-number v-model="historyLimit" theme="normal" :min="10" :max="200" />
          <t-button variant="outline" @click="loadHistory">刷新</t-button>
        </t-space>
      </template>
      <t-table
        :data="historyItems"
        :columns="historyColumns"
        row-key="id"
        :loading="historyLoading"
        hover
      >
        <template #created_at="{ row }">{{ fmt(row.created_at) }}</template>
        <template #payload_json="{ row }">
          <div class="payload">{{ row.payload_json || '{}' }}</div>
        </template>
      </t-table>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { sandboxApi, type Sandbox, type SandboxStatusHistoryItem } from '../api/sandbox'

const route = useRoute()
const sandboxId = route.params.id as string

const loading = ref(false)
const historyLoading = ref(false)
const sandbox = ref<Sandbox | null>(null)
const historyItems = ref<SandboxStatusHistoryItem[]>([])
const historyLimit = ref(50)

const historyColumns = [
  { colKey: 'id', title: 'ID', width: 80 },
  { colKey: 'created_at', title: '时间', width: 180 },
  { colKey: 'source', title: '来源', width: 120 },
  { colKey: 'from_status', title: 'from', width: 120 },
  { colKey: 'to_status', title: 'to', width: 120 },
  { colKey: 'reason', title: '原因', width: 220, ellipsis: true },
  { colKey: 'payload_json', title: 'payload', ellipsis: true },
]

const fmt = (v?: string) => (v ? new Date(v).toLocaleString() : '-')

const loadSandbox = async () => {
  loading.value = true
  try {
    const resp = await sandboxApi.listMetadata({
      id: sandboxId,
      page: 1,
      page_size: 1,
    })
    sandbox.value = (resp.data.items || [])[0] || null
    if (!sandbox.value) {
      MessagePlugin.warning('未找到该元数据记录')
    }
  } catch (err: any) {
    MessagePlugin.error('加载元数据失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const loadHistory = async () => {
  historyLoading.value = true
  try {
    const resp = await sandboxApi.getStatusHistory(sandboxId, { limit: historyLimit.value })
    historyItems.value = resp.data.items || []
  } catch (err: any) {
    MessagePlugin.error('加载状态流水失败: ' + (err.response?.data?.error || err.message))
  } finally {
    historyLoading.value = false
  }
}

onMounted(async () => {
  await loadSandbox()
  await loadHistory()
})
</script>

<style scoped>
.metadata-sandbox-detail {
  padding: 24px;
}

.history-card {
  margin-top: 16px;
}

.payload {
  max-width: 520px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: monospace;
}
</style>
