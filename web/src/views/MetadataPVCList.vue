<template>
  <div class="metadata-pvc-list">
    <t-card title="PVC 管理" :bordered="false">
      <t-space direction="vertical" style="width: 100%">
        <t-row :gutter="[12, 12]">
          <t-col :span="3">
            <t-input
              v-model="filters.sandbox_id"
              placeholder="按 sandbox_id 过滤"
              clearable
              @enter="reload"
            />
          </t-col>
          <t-col :span="3">
            <t-input
              v-model="filters.storage_class"
              placeholder="按 storageClass 过滤"
              clearable
              @enter="reload"
            />
          </t-col>
          <t-col :span="3">
            <t-select v-model="filters.state" clearable placeholder="状态">
              <t-option value="bound" label="bound" />
              <t-option value="orphan_pvc" label="orphan_pvc" />
              <t-option value="dangling_metadata" label="dangling_metadata" />
            </t-select>
          </t-col>
          <t-col :span="2">
            <t-button theme="primary" @click="reload">查询</t-button>
          </t-col>
          <t-col :span="2">
            <t-button variant="outline" @click="resetFilters">重置</t-button>
          </t-col>
        </t-row>

        <t-table :data="rows" :columns="columns" row-key="pvcName" :loading="loading" hover>
          <template #state="{ row }">
            <t-tag :theme="stateTheme(row.state)">{{ row.state }}</t-tag>
          </template>
          <template #sandboxId="{ row }">
            <t-link
              v-if="row.sandboxId"
              theme="primary"
              @click="$router.push(`/metadata/sandboxes/${row.sandboxId}`)"
            >
              {{ row.sandboxId }}
            </t-link>
            <span v-else>-</span>
          </template>
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
import { MessagePlugin } from 'tdesign-vue-next'
import { sandboxApi, type PVCMapping } from '../api/sandbox'

const loading = ref(false)
const rows = ref<PVCMapping[]>([])

const filters = reactive({
  sandbox_id: '',
  storage_class: '',
  state: '' as '' | 'bound' | 'orphan_pvc' | 'dangling_metadata',
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
})

const columns = [
  { colKey: 'pvcName', title: 'PVC', width: 220 },
  { colKey: 'namespace', title: '命名空间', width: 180 },
  { colKey: 'storageClassName', title: 'StorageClass', width: 160 },
  { colKey: 'requestedSize', title: '容量', width: 100 },
  { colKey: 'phase', title: 'Phase', width: 100 },
  { colKey: 'pvName', title: 'PV', ellipsis: true },
  { colKey: 'sandboxId', title: 'Sandbox', width: 140 },
  { colKey: 'sandboxLifecycleStatus', title: 'Sandbox 状态', width: 130 },
  { colKey: 'reclaimPolicy', title: '回收策略', width: 100 },
  { colKey: 'state', title: '映射状态', width: 150 },
  { colKey: 'source', title: '来源', width: 90 },
]

const stateTheme = (state: string) => {
  switch (state) {
    case 'bound':
      return 'success'
    case 'orphan_pvc':
      return 'warning'
    case 'dangling_metadata':
      return 'danger'
    default:
      return 'default'
  }
}

const reload = async () => {
  loading.value = true
  try {
    const resp = await sandboxApi.listPVCMappings({
      sandbox_id: filters.sandbox_id || undefined,
      storage_class: filters.storage_class || undefined,
      state: filters.state || undefined,
      page: pagination.page,
      page_size: pagination.page_size,
    })
    rows.value = resp.data.items || []
    pagination.total = resp.data.total || 0
  } catch (err: any) {
    MessagePlugin.error('加载 PVC 列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const resetFilters = () => {
  filters.sandbox_id = ''
  filters.storage_class = ''
  filters.state = ''
  pagination.page = 1
  reload()
}

onMounted(() => {
  reload()
})
</script>

<style scoped>
.metadata-pvc-list {
  padding: 24px;
}
</style>
