<template>
  <div class="sandbox-list">
    <t-card title="Sandboxes" :bordered="false">
      <template #actions>
        <t-space>
          <t-button theme="default" variant="outline" @click="showTemplateSelect = true">
            从模板创建
          </t-button>
          <t-button theme="primary" @click="showCreateDialog = true"> 自定义创建 </t-button>
        </t-space>
      </template>

      <t-table :data="sandboxes" :columns="columns" :loading="loading" row-key="id" hover>
        <template #id="{ row }">
          <t-link theme="primary" @click="goToDetail(row.id)">
            {{ row.id }}
          </t-link>
        </template>
        <template #image="{ row }">
          <div>
            <span class="image-text">{{ row.image }}</span>
            <t-tag v-if="row.template" size="small" variant="light" style="margin-left: 8px">
              {{ row.template }}
            </t-tag>
          </div>
        </template>
        <template #status="{ row }">
          <t-tag :theme="getStatusTheme(row.status)">
            {{ getStatusText(row.status) }}
          </t-tag>
        </template>
        <template #created_at="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
        <template #expires_at="{ row }">
          {{ formatTime(row.expires_at) }}
        </template>
        <template #operation="{ row }">
          <t-space>
            <t-link theme="primary" @click="goToDetail(row.id)">详情</t-link>
            <t-popconfirm content="确定要删除该 Sandbox 吗？" @confirm="deleteSandbox(row.id)">
              <t-link theme="danger">删除</t-link>
            </t-popconfirm>
          </t-space>
        </template>
      </t-table>
    </t-card>

    <!-- Custom Create Dialog -->
    <t-dialog
      v-model:visible="showCreateDialog"
      header="创建 Sandbox"
      :confirm-btn="{ content: '创建', loading: creating }"
      @confirm="createSandbox"
    >
      <t-form :data="createForm" :rules="formRules" ref="formRef">
        <t-form-item label="镜像" name="image">
          <t-input v-model="createForm.image" placeholder="如：python:3.11-slim" />
        </t-form-item>
        <t-form-item label="CPU">
          <t-input v-model="createForm.cpu" placeholder="如：500m" />
        </t-form-item>
        <t-form-item label="内存">
          <t-input v-model="createForm.memory" placeholder="如：512Mi" />
        </t-form-item>
        <t-form-item label="TTL (秒)">
          <t-input-number v-model="createForm.ttl" :min="60" :max="86400" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- Template Select Dialog -->
    <t-dialog
      v-model:visible="showTemplateSelect"
      header="从模板创建 Sandbox"
      width="700px"
      :confirm-btn="{ content: '创建', loading: creating }"
      @confirm="createFromTemplate"
    >
      <t-input
        v-model="templateSearch"
        placeholder="搜索模板名称（实时搜索）..."
        :style="{ marginBottom: '16px' }"
        clearable
        @keyup.enter="onSearchEnter"
      >
        <template #suffix-icon>
          <search-icon />
        </template>
      </t-input>
      <t-alert
        v-if="templateSearch && filteredTemplates.length === 0 && !templatesLoading"
        theme="warning"
        message="没有匹配的模板"
        style="margin-bottom: 16px"
      />
      <t-alert
        v-if="templatesLoading"
        theme="info"
        message="搜索中..."
        style="margin-bottom: 16px"
      />

      <div class="template-list" :style="{ maxHeight: '400px', overflow: 'auto' }">
        <div
          v-for="item in filteredTemplates"
          :key="item.name"
          class="template-item"
          :class="{ 'is-selected': selectedTemplate === item.name }"
          @click="selectTemplate(item.name)"
        >
          <div class="template-icon">{{ item.name.charAt(0).toUpperCase() }}</div>
          <div class="template-info">
            <div class="template-name">{{ item.displayName || item.name }}</div>
            <div class="template-desc">
              {{ item.description || '无描述' }} · 镜像: {{ item.spec?.image || 'N/A' }}
            </div>
          </div>
          <t-radio :value="item.name" v-model="selectedTemplate" @click.stop />
        </div>
        <t-empty
          v-if="filteredTemplates.length === 0 && !templatesLoading"
          description="暂无模板，请先到模板管理页面创建"
        >
          <template #action>
            <t-button size="small" @click="goToTemplates">创建模板</t-button>
          </template>
        </t-empty>
      </div>

      <t-divider />

      <t-form v-if="selectedTemplate" :data="templateOverrides" label-width="80px">
        <t-form-item label="覆盖 CPU">
          <t-input v-model="templateOverrides.cpu" placeholder="留空使用模板默认值" />
        </t-form-item>
        <t-form-item label="覆盖内存">
          <t-input v-model="templateOverrides.memory" placeholder="留空使用模板默认值" />
        </t-form-item>
        <t-form-item label="覆盖 TTL">
          <t-input-number v-model="templateOverrides.ttl" :min="60" :max="86400" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { SearchIcon } from 'tdesign-icons-vue-next'
import { debounce } from 'lodash-es'
import { sandboxApi, type Sandbox, type CreateSandboxRequest } from '../api/sandbox'
import { templateApi, type Template } from '../api/template'

const router = useRouter()
const sandboxes = ref<Sandbox[]>([])
const templates = ref<Template[]>([])
const loading = ref(false)
const showCreateDialog = ref(false)
const showTemplateSelect = ref(false)
const creating = ref(false)
const formRef = ref()
const templateSearch = ref('')
const selectedTemplate = ref('')
const templatesLoading = ref(false)

const createForm = ref<CreateSandboxRequest>({
  image: 'python:3.11-slim',
  cpu: '500m',
  memory: '512Mi',
  ttl: 3600,
})

const templateOverrides = ref({
  cpu: '',
  memory: '',
  ttl: 0,
})

const formRules = {
  image: [{ required: true, message: '请输入镜像名称' }],
}

const columns = [
  { colKey: 'id', title: 'ID', width: 120 },
  { colKey: 'image', title: '镜像', ellipsis: true },
  { colKey: 'cpu', title: 'CPU', width: 100 },
  { colKey: 'memory', title: '内存', width: 100 },
  { colKey: 'status', title: '状态', width: 100 },
  { colKey: 'created_at', title: '创建时间', width: 180 },
  { colKey: 'expires_at', title: '过期时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 120 },
]

const filteredTemplates = ref<Template[]>([])

const getStatusTheme = (status: string) => {
  switch (status) {
    case 'running':
      return 'success'
    case 'pending':
      return 'warning'
    case 'failed':
      return 'danger'
    case 'terminating':
      return 'warning'
    default:
      return 'default'
  }
}

const getStatusText = (status: string) => {
  switch (status) {
    case 'running':
      return '运行中'
    case 'pending':
      return '启动中'
    case 'failed':
      return '失败'
    case 'succeeded':
      return '已完成'
    case 'terminating':
      return '正在销毁'
    default:
      return status
  }
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const loadSandboxes = async () => {
  loading.value = true
  try {
    const res = await sandboxApi.list()
    sandboxes.value = res.data.items || []
  } catch (err: any) {
    MessagePlugin.error('加载失败: ' + (err.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

const loadTemplates = async () => {
  templatesLoading.value = true
  try {
    const res = await templateApi.list()
    templates.value = res.data.items || []
    filteredTemplates.value = templates.value
    console.log(
      'Loaded templates:',
      templates.value.length,
      templates.value.map((t) => t.name)
    )
  } catch (err: any) {
    console.error('Failed to load templates:', err)
    MessagePlugin.warning('无法加载模板列表，请检查后端服务')
  } finally {
    templatesLoading.value = false
  }
}

const searchTemplates = async () => {
  const search = templateSearch.value.trim()
  if (!search) {
    filteredTemplates.value = templates.value
    return
  }

  templatesLoading.value = true
  try {
    // Use backend search API
    const res = await templateApi.list({ search: search, pageSize: 100 })
    filteredTemplates.value = res.data.items || []
    console.log('Search result:', filteredTemplates.value.length, 'templates')
  } catch (err: any) {
    console.error('Search failed:', err)
    // Fallback to client-side filtering
    filteredTemplates.value = templates.value.filter(
      (t) =>
        t.name.toLowerCase().includes(search.toLowerCase()) ||
        (t.displayName && t.displayName.toLowerCase().includes(search.toLowerCase())) ||
        (t.description && t.description.toLowerCase().includes(search.toLowerCase()))
    )
  } finally {
    templatesLoading.value = false
  }
}

const createSandbox = async () => {
  const valid = await formRef.value?.validate()
  if (valid !== true) return

  creating.value = true
  try {
    await sandboxApi.create(createForm.value)
    MessagePlugin.success('创建成功')
    showCreateDialog.value = false
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('创建失败: ' + (err.response?.data?.error || err.message))
  } finally {
    creating.value = false
  }
}

const createFromTemplate = async () => {
  if (!selectedTemplate.value) {
    MessagePlugin.warning('请选择模板')
    return
  }

  creating.value = true
  try {
    const data: CreateSandboxRequest = {
      template: selectedTemplate.value,
    }
    if (templateOverrides.value.cpu) {
      data.overrides = { cpu: templateOverrides.value.cpu }
    }
    if (templateOverrides.value.memory) {
      data.overrides = { ...data.overrides, memory: templateOverrides.value.memory }
    }
    if (templateOverrides.value.ttl > 0) {
      data.overrides = { ...data.overrides, ttl: templateOverrides.value.ttl }
    }

    await sandboxApi.create(data)
    MessagePlugin.success('创建成功')
    showTemplateSelect.value = false
    selectedTemplate.value = ''
    templateOverrides.value = { cpu: '', memory: '', ttl: 0 }
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('创建失败: ' + (err.response?.data?.error || err.message))
  } finally {
    creating.value = false
  }
}

const deleteSandbox = async (id: string) => {
  try {
    await sandboxApi.delete(id)
    MessagePlugin.success('删除成功')
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

const goToDetail = (id: string) => {
  router.push(`/sandboxes/${id}`)
}

const goToTemplates = () => {
  showTemplateSelect.value = false
  router.push('/templates')
}

const selectTemplate = (name: string) => {
  selectedTemplate.value = name
}

const onSearchEnter = () => {
  if (filteredTemplates.value.length === 1) {
    const first = filteredTemplates.value[0]
    if (first) {
      selectedTemplate.value = first.name
    }
  }
}

let refreshInterval: number

const startRefresh = () => {
  if (refreshInterval) clearInterval(refreshInterval)
  refreshInterval = window.setInterval(loadSandboxes, 5000)
}

const stopRefresh = () => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
    refreshInterval = 0 as unknown as number
  }
}

// Pause sandbox refresh when dialogs are open, and reload templates when opening template select
watch([showCreateDialog, showTemplateSelect], ([create, template]) => {
  if (create || template) {
    stopRefresh()
    // Reload templates when opening template selection dialog
    if (template) {
      loadTemplates()
    }
  } else {
    startRefresh()
  }
})

// Debounced search using backend API
const debouncedSearch = debounce(() => {
  searchTemplates()
}, 300)

watch(templateSearch, () => {
  debouncedSearch()
})

onMounted(() => {
  loadSandboxes()
  loadTemplates()
  startRefresh()
})

onUnmounted(() => {
  stopRefresh()
})
</script>

<style scoped>
.sandbox-list {
  padding: 24px;
}

.image-text {
  font-family: monospace;
  font-size: 12px;
}

.template-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.template-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--td-component-border);
  border-radius: 8px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.template-item:hover {
  background: var(--td-bg-color-container-hover);
}

.template-item.is-selected {
  background: var(--td-brand-color-light);
  border-color: var(--td-brand-color);
}

.template-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: var(--td-brand-color);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: bold;
  font-size: 18px;
  flex-shrink: 0;
}

.template-info {
  flex: 1;
  min-width: 0;
}

.template-name {
  font-weight: 500;
  margin-bottom: 4px;
}

.template-desc {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
