<template>
  <div class="metadata-reconcile">
    <t-card title="对账记录中心" :bordered="false">
      <template #actions>
        <t-space>
          <t-button theme="primary" :loading="triggerLoading" @click="triggerRun"
            >手动触发对账</t-button
          >
          <t-button variant="outline" @click="loadRuns">刷新</t-button>
        </t-space>
      </template>

      <t-table :data="runs" :columns="runColumns" row-key="id" :loading="runsLoading" hover>
        <template #id="{ row }">
          <t-link theme="primary" @click="selectRun(row.id)">{{ row.id }}</t-link>
        </template>
        <template #started_at="{ row }">{{ fmt(row.started_at) }}</template>
        <template #finished_at="{ row }">{{ fmt(row.finished_at) }}</template>
        <template #status="{ row }">
          <t-tag
            :theme="
              row.status === 'completed'
                ? 'success'
                : row.status === 'failed'
                  ? 'danger'
                  : 'warning'
            "
          >
            {{ row.status }}
          </t-tag>
        </template>
      </t-table>
    </t-card>

    <t-card
      :title="`对账明细 ${selectedRunId ? '(' + selectedRunId + ')' : ''}`"
      :bordered="false"
      class="detail-card"
    >
      <t-row :gutter="[12, 12]" class="filter-row">
        <t-col :span="3">
          <t-select v-model="filters.drift_type" clearable placeholder="过滤 drift_type">
            <t-option value="missing_in_k8s" label="missing_in_k8s" />
            <t-option value="missing_in_db" label="missing_in_db" />
            <t-option value="status_mismatch" label="status_mismatch" />
            <t-option value="spec_mismatch" label="spec_mismatch" />
          </t-select>
        </t-col>
        <t-col :span="3">
          <t-select v-model="filters.action" clearable placeholder="过滤 action">
            <t-option value="mark_lost" label="mark_lost" />
            <t-option value="mark_deleted" label="mark_deleted" />
            <t-option value="alert_only" label="alert_only" />
            <t-option value="none" label="none" />
          </t-select>
        </t-col>
        <t-col :span="3">
          <t-input v-model="filters.sandbox_id" placeholder="过滤 sandbox_id" clearable />
        </t-col>
        <t-col :span="3">
          <label class="filter-label">明细起始时间</label>
          <input v-model="filters.created_from" class="time-input" type="datetime-local" />
        </t-col>
        <t-col :span="3">
          <label class="filter-label">明细结束时间</label>
          <input v-model="filters.created_to" class="time-input" type="datetime-local" />
        </t-col>
        <t-col :span="3">
          <t-input v-model="quickJumpSandboxID" placeholder="快速跳转 sandbox_id" clearable />
        </t-col>
        <t-col :span="2">
          <t-button variant="outline" @click="jumpToSandbox">跳转</t-button>
        </t-col>
      </t-row>
      <t-loading :loading="detailLoading">
        <t-table :data="filteredItems" :columns="itemColumns" row-key="id" hover>
          <template #created_at="{ row }">{{ fmt(row.created_at) }}</template>
          <template #sandbox_id="{ row }">
            <t-link theme="primary" @click="$router.push(`/metadata/sandboxes/${row.sandbox_id}`)">
              {{ row.sandbox_id }}
            </t-link>
          </template>
        </t-table>
      </t-loading>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { sandboxApi, type ReconcileItem, type ReconcileRun } from '../api/sandbox'

const router = useRouter()
const runsLoading = ref(false)
const detailLoading = ref(false)
const triggerLoading = ref(false)
const runs = ref<ReconcileRun[]>([])
const items = ref<ReconcileItem[]>([])
const selectedRunId = ref('')
const quickJumpSandboxID = ref('')
const filters = reactive({
  drift_type: '',
  action: '',
  sandbox_id: '',
  created_from: '',
  created_to: '',
})

const runColumns = [
  { colKey: 'id', title: 'Run ID', width: 140 },
  { colKey: 'trigger_type', title: '触发来源', width: 120 },
  { colKey: 'started_at', title: '开始时间', width: 180 },
  { colKey: 'finished_at', title: '结束时间', width: 180 },
  { colKey: 'total_db', title: 'DB数', width: 90 },
  { colKey: 'total_k8s', title: 'K8s数', width: 90 },
  { colKey: 'drift_count', title: '漂移数', width: 90 },
  { colKey: 'fixed_count', title: '修复数', width: 90 },
  { colKey: 'status', title: '状态', width: 100 },
  { colKey: 'error', title: '错误', ellipsis: true },
]

const itemColumns = [
  { colKey: 'id', title: 'ID', width: 80 },
  { colKey: 'created_at', title: '时间', width: 180 },
  { colKey: 'sandbox_id', title: 'Sandbox', width: 140 },
  { colKey: 'drift_type', title: '漂移类型', width: 140 },
  { colKey: 'action', title: '动作', width: 120 },
  { colKey: 'detail', title: '详情', ellipsis: true },
]

const fmt = (v?: string) => (v ? new Date(v).toLocaleString() : '-')

const filteredItems = computed(() => {
  const createdFrom = toTime(filters.created_from)
  const createdTo = toTime(filters.created_to)
  return items.value.filter((item) => {
    if (filters.drift_type && item.drift_type !== filters.drift_type) return false
    if (filters.action && item.action !== filters.action) return false
    if (filters.sandbox_id && !item.sandbox_id.includes(filters.sandbox_id)) return false
    const createdAt = new Date(item.created_at).getTime()
    if (createdFrom !== null && createdAt < createdFrom) return false
    if (createdTo !== null && createdAt > createdTo) return false
    return true
  })
})

const toTime = (v: string): number | null => {
  if (!v) return null
  const t = new Date(v).getTime()
  if (Number.isNaN(t)) return null
  return t
}

const loadRuns = async () => {
  runsLoading.value = true
  try {
    const resp = await sandboxApi.listReconcileRuns(50)
    runs.value = resp.data.items || []
    if (!selectedRunId.value && runs.value.length > 0) {
      const first = runs.value[0]
      if (first) {
        await selectRun(first.id)
      }
    }
  } catch (err: any) {
    MessagePlugin.error('加载对账列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    runsLoading.value = false
  }
}

const selectRun = async (id: string) => {
  selectedRunId.value = id
  detailLoading.value = true
  try {
    const resp = await sandboxApi.getReconcileRun(id)
    items.value = resp.data.items || []
  } catch (err: any) {
    MessagePlugin.error('加载对账明细失败: ' + (err.response?.data?.error || err.message))
  } finally {
    detailLoading.value = false
  }
}

const triggerRun = async () => {
  triggerLoading.value = true
  try {
    const resp = await sandboxApi.triggerReconcile()
    MessagePlugin.success(`对账已触发: ${resp.data.run.id}`)
    selectedRunId.value = resp.data.run.id
    items.value = resp.data.items || []
    await loadRuns()
  } catch (err: any) {
    MessagePlugin.error('触发对账失败: ' + (err.response?.data?.error || err.message))
  } finally {
    triggerLoading.value = false
  }
}

const jumpToSandbox = () => {
  const id = quickJumpSandboxID.value.trim()
  if (!id) {
    MessagePlugin.warning('请输入 sandbox_id')
    return
  }
  router.push(`/metadata/sandboxes/${id}`)
}

onMounted(async () => {
  await loadRuns()
})
</script>

<style scoped>
.metadata-reconcile {
  padding: 24px;
}

.detail-card {
  margin-top: 16px;
}

.filter-row {
  margin-bottom: 12px;
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
