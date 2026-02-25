<template>
  <div class="metadata-sandbox-list">
    <t-card title="沙箱元数据记录" :bordered="false">
      <t-space direction="vertical" style="width: 100%">
        <t-row :gutter="[12, 12]">
          <t-col :span="3">
            <t-input v-model="filters.id" placeholder="按 ID 前缀过滤" clearable @enter="reload" />
          </t-col>
          <t-col :span="3">
            <t-input v-model="filters.template" placeholder="模板名" clearable @enter="reload" />
          </t-col>
          <t-col :span="2">
            <t-select v-model="filters.desired_state" clearable placeholder="期望状态">
              <t-option value="active" label="active" />
              <t-option value="deleted" label="deleted" />
            </t-select>
          </t-col>
          <t-col :span="2">
            <t-input
              v-model="filters.lifecycle_status"
              placeholder="生命周期状态"
              clearable
              @enter="reload"
            />
          </t-col>
          <t-col :span="2">
            <t-button theme="primary" @click="reload">查询</t-button>
          </t-col>
          <t-col :span="2">
            <t-button variant="outline" @click="resetFilters">重置</t-button>
          </t-col>
        </t-row>
        <t-row :gutter="[12, 12]">
          <t-col :span="3">
            <label class="filter-label">创建起始</label>
            <input v-model="filters.created_from" class="time-input" type="datetime-local" />
          </t-col>
          <t-col :span="3">
            <label class="filter-label">创建结束</label>
            <input v-model="filters.created_to" class="time-input" type="datetime-local" />
          </t-col>
          <t-col :span="3">
            <label class="filter-label">删除起始</label>
            <input v-model="filters.deleted_from" class="time-input" type="datetime-local" />
          </t-col>
          <t-col :span="3">
            <label class="filter-label">删除结束</label>
            <input v-model="filters.deleted_to" class="time-input" type="datetime-local" />
          </t-col>
        </t-row>

        <t-table :data="rows" :columns="columns" row-key="id" :loading="loading" hover>
          <template #id="{ row }">
            <t-link theme="primary" @click="goDetail(row.id)">{{ row.id }}</t-link>
          </template>
          <template #status="{ row }">
            <t-tag :theme="statusTheme(row.lifecycle_status || row.status)">
              {{ row.lifecycle_status || row.status || '-' }}
            </t-tag>
          </template>
          <template #created_at="{ row }">{{ fmt(row.created_at) }}</template>
          <template #last_seen_at="{ row }">{{ fmt(row.last_seen_at) }}</template>
          <template #deleted_at="{ row }">{{ fmt(row.deleted_at) }}</template>
        </t-table>

        <t-pagination
          v-model="pagination.page"
          v-model:page-size="pagination.page_size"
          :total="pagination.total"
          :show-page-size="true"
          :page-size-options="[20, 50, 100]"
          @change="reload"
        />
      </t-space>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { sandboxApi, type Sandbox } from '../api/sandbox'

const router = useRouter()
const loading = ref(false)
const rows = ref<Sandbox[]>([])

const filters = reactive({
  id: '',
  template: '',
  desired_state: '',
  lifecycle_status: '',
  created_from: '',
  created_to: '',
  deleted_from: '',
  deleted_to: '',
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
})

const columns = [
  { colKey: 'id', title: 'ID', width: 140 },
  { colKey: 'template', title: '模板', width: 160 },
  { colKey: 'templateVersion', title: '版本', width: 80 },
  { colKey: 'desired_state', title: '期望状态', width: 100 },
  { colKey: 'status', title: '生命周期状态', width: 140 },
  { colKey: 'pod_phase', title: 'PodPhase', width: 120 },
  { colKey: 'status_reason', title: '原因', ellipsis: true },
  { colKey: 'created_at', title: '创建时间', width: 170 },
  { colKey: 'last_seen_at', title: '最近观测', width: 170 },
  { colKey: 'deleted_at', title: '删除时间', width: 170 },
]

const fmt = (v?: string) => (v ? new Date(v).toLocaleString() : '-')

const statusTheme = (status?: string) => {
  switch (status) {
    case 'running':
      return 'success'
    case 'failed':
    case 'lost':
      return 'danger'
    case 'pending':
    case 'creating':
    case 'terminating':
      return 'warning'
    case 'deleted':
      return 'default'
    default:
      return 'default'
  }
}

const reload = async () => {
  loading.value = true
  try {
    const resp = await sandboxApi.listMetadata({
      id: filters.id || undefined,
      template: filters.template || undefined,
      desired_state: filters.desired_state || undefined,
      lifecycle_status: filters.lifecycle_status || undefined,
      created_from: toRFC3339(filters.created_from),
      created_to: toRFC3339(filters.created_to),
      deleted_from: toRFC3339(filters.deleted_from),
      deleted_to: toRFC3339(filters.deleted_to),
      page: pagination.page,
      page_size: pagination.page_size,
    })
    rows.value = resp.data.items || []
    pagination.total = resp.data.total || 0
  } catch (err: any) {
    MessagePlugin.error('加载元数据失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const resetFilters = () => {
  filters.id = ''
  filters.template = ''
  filters.desired_state = ''
  filters.lifecycle_status = ''
  filters.created_from = ''
  filters.created_to = ''
  filters.deleted_from = ''
  filters.deleted_to = ''
  pagination.page = 1
  reload()
}

const toRFC3339 = (localValue: string): string | undefined => {
  if (!localValue) return undefined
  const t = new Date(localValue)
  if (Number.isNaN(t.getTime())) return undefined
  return t.toISOString()
}

const goDetail = (id: string) => {
  router.push(`/metadata/sandboxes/${id}`)
}

onMounted(() => {
  reload()
})
</script>

<style scoped>
.metadata-sandbox-list {
  padding: 24px;
}

.filter-label {
  display: block;
  margin-bottom: 6px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.time-input {
  box-sizing: border-box;
  width: 100%;
  height: 32px;
  padding: 0 8px;
  border: 1px solid var(--td-component-border);
  border-radius: 4px;
}
</style>
