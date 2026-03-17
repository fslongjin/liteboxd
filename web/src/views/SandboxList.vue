<template>
  <div class="sandbox-list">
    <t-card title="Sandboxes" :bordered="false">
      <template #actions>
        <t-button theme="primary" @click="showTemplateSelect = true">
          <template #icon><add-icon /></template>
          创建沙箱
        </t-button>
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
        <template #ttl="{ row }">
          {{ formatTTL(row.ttl) }}
        </template>
        <template #persistence="{ row }">
          {{ formatSandboxPersistence(row) }}
        </template>
        <template #expires_at="{ row }">
          {{ row.ttl === 0 ? '永久' : formatTime(row.expires_at) }}
        </template>
        <template #operation="{ row }">
          <t-space>
            <t-link theme="primary" @click="goToDetail(row.id)">详情</t-link>
            <t-popconfirm
              v-if="canStopSandbox(row)"
              content="确定要停止该 Sandbox 吗？停止后可重新启动。"
              @confirm="stopSandbox(row.id)"
            >
              <t-link theme="warning">停止</t-link>
            </t-popconfirm>
            <t-popconfirm
              v-if="canStartSandbox(row)"
              content="确定要启动该 Sandbox 吗？"
              @confirm="startSandbox(row.id)"
            >
              <t-link theme="success">启动</t-link>
            </t-popconfirm>
            <t-popconfirm
              v-if="canRestartSandbox(row)"
              content="确定要重启该持久化 Sandbox 吗？"
              @confirm="restartSandbox(row.id)"
            >
              <t-link theme="warning">重启</t-link>
            </t-popconfirm>
            <span
              v-else-if="row.persistence?.enabled && row.status === 'stopped'"
              class="op-disabled"
              >重启</span
            >
            <span v-else class="op-disabled">重启</span>
            <t-popconfirm content="确定要删除该 Sandbox 吗？" @confirm="deleteSandbox(row.id)">
              <t-link theme="danger">删除</t-link>
            </t-popconfirm>
          </t-space>
        </template>
      </t-table>
    </t-card>

    <!-- Template Select Dialog -->
    <t-dialog
      v-model:visible="showTemplateSelect"
      header="创建沙箱"
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
        <t-form-item label="持久化">
          <span class="template-desc-inline">{{ selectedTemplatePersistenceLabel || '关闭' }}</span>
        </t-form-item>
        <t-form-item label="覆盖 CPU">
          <t-input v-model="templateOverrides.cpu" placeholder="留空使用模板默认值" />
        </t-form-item>
        <t-form-item label="覆盖内存">
          <t-input v-model="templateOverrides.memory" placeholder="留空使用模板默认值" />
        </t-form-item>
        <t-form-item label="覆盖 TTL">
          <t-input-number v-model="templateOverrides.ttl" :min="0" :max="86400" />
        </t-form-item>
        <t-form-item v-if="selectedTemplateHasPersistence" label="覆盖磁盘">
          <t-input
            v-model="templateOverrides.persistenceSize"
            placeholder="如: 40Gi（留空使用模板默认值）"
          />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { SearchIcon, AddIcon } from 'tdesign-icons-vue-next'
import { debounce } from 'lodash-es'
import { sandboxApi, type Sandbox, type CreateSandboxRequest } from '../api/sandbox'
import { templateApi, type Template } from '../api/template'

const router = useRouter()
const route = useRoute()
const sandboxes = ref<Sandbox[]>([])
const templates = ref<Template[]>([])
const loading = ref(false)
const showTemplateSelect = ref(false)
const creating = ref(false)
const templateSearch = ref('')
const selectedTemplate = ref('')
const templatesLoading = ref(false)

const templateOverrides = ref({
  cpu: '',
  memory: '',
  ttl: null as number | null,
  persistenceSize: '',
})

const columns = [
  { colKey: 'id', title: 'ID', width: 120 },
  { colKey: 'image', title: '镜像', ellipsis: true },
  { colKey: 'cpu', title: 'CPU', width: 100 },
  { colKey: 'memory', title: '内存', width: 100 },
  { colKey: 'ttl', title: 'TTL', width: 90 },
  { colKey: 'persistence', title: '持久化', ellipsis: true },
  { colKey: 'status', title: '状态', width: 100 },
  { colKey: 'created_at', title: '创建时间', width: 180 },
  { colKey: 'expires_at', title: '过期时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 260 },
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
    case 'stopped':
      return 'default'
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
    case 'stopped':
      return '已停止'
    default:
      return status
  }
}

const formatTime = (time: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const formatTTL = (ttl?: number) => {
  if (ttl === 0) return '永久'
  if (!ttl) return '-'
  return `${ttl}s`
}

const formatSandboxPersistence = (sb: Sandbox) => {
  const p = sb.persistence
  if (!p || !p.enabled) return '关闭'
  const mode = p.mode || 'rootfs-overlay'
  const size = p.size || '-'
  const sc = p.storageClassName || '-'
  return `${mode} / ${size} / ${sc}`
}

const selectedTemplateObj = computed(() => {
  if (!selectedTemplate.value) return null
  return (
    filteredTemplates.value.find((t) => t.name === selectedTemplate.value) ||
    templates.value.find((t) => t.name === selectedTemplate.value) ||
    null
  )
})

const selectedTemplateHasPersistence = computed(
  () => selectedTemplateObj.value?.spec?.persistence?.enabled === true
)

const selectedTemplatePersistenceLabel = computed(() => {
  const p = selectedTemplateObj.value?.spec?.persistence
  if (!p || !p.enabled) return ''
  const mode = p.mode || 'rootfs-overlay'
  const size = p.size || '-'
  const sc = p.storageClassName || '-'
  return `${mode} / ${size} / ${sc}`
})

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
    if (templateOverrides.value.ttl !== null) {
      data.overrides = { ...data.overrides, ttl: templateOverrides.value.ttl }
    }
    if (selectedTemplateHasPersistence.value && templateOverrides.value.persistenceSize.trim()) {
      data.overrides = {
        ...data.overrides,
        persistence: {
          size: templateOverrides.value.persistenceSize.trim(),
        },
      }
    }

    await sandboxApi.create(data)
    MessagePlugin.success('创建成功')
    showTemplateSelect.value = false
    selectedTemplate.value = ''
    templateOverrides.value = { cpu: '', memory: '', ttl: null, persistenceSize: '' }
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
    MessagePlugin.success('已提交删除，后台正在清理资源')
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

const canRestartSandbox = (sb: Sandbox) =>
  sb.persistence?.enabled === true && sb.status !== 'stopped'

const canStopSandbox = (sb: Sandbox) => sb.persistence?.enabled === true && sb.status === 'running'

const canStartSandbox = (sb: Sandbox) => sb.persistence?.enabled === true && sb.status === 'stopped'

const restartSandbox = async (id: string) => {
  try {
    await sandboxApi.restart(id)
    MessagePlugin.success('已触发重启')
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('重启失败: ' + (err.response?.data?.error || err.message))
  }
}

const stopSandbox = async (id: string) => {
  try {
    await sandboxApi.stop(id)
    MessagePlugin.success('已触发停止')
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('停止失败: ' + (err.response?.data?.error || err.message))
  }
}

const startSandbox = async (id: string) => {
  try {
    await sandboxApi.start(id)
    MessagePlugin.success('已触发启动')
    loadSandboxes()
  } catch (err: any) {
    MessagePlugin.error('启动失败: ' + (err.response?.data?.error || err.message))
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
  templateOverrides.value.persistenceSize = ''
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

// Pause sandbox refresh when dialog is open, and reload templates when opening
watch(showTemplateSelect, (isOpen) => {
  if (isOpen) {
    stopRefresh()
    loadTemplates()
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

// Check for query parameters on mount to pre-open dialog
onMounted(() => {
  loadSandboxes()
  loadTemplates().then(() => {
    // Check if we should open the template dialog
    const tmplName = route.query.template as string
    if (tmplName) {
      showTemplateSelect.value = true
      selectedTemplate.value = tmplName

      // Clean up URL so refresh doesn't keep opening it
      router.replace({ path: '/sandboxes', query: {} })
    }
  })
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

.op-disabled {
  color: var(--td-text-color-disabled);
}

.template-list {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  padding: 4px;
}

.template-item {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  padding: 16px;
  border: 1px solid var(--td-component-border);
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  background: var(--td-bg-color-container);
  position: relative;
  overflow: hidden;
}

.template-item:hover {
  border-color: var(--td-brand-color);
  transform: translateY(-2px);
  box-shadow: 0 8px 16px -4px rgba(0, 0, 0, 0.05);
}

.template-item.is-selected {
  background: var(--td-brand-color-light);
  border-color: var(--td-brand-color);
}

.template-item.is-selected::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  width: 0;
  height: 0;
  border-style: solid;
  border-width: 0 24px 24px 0;
  border-color: transparent var(--td-brand-color) transparent transparent;
}

.template-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--td-brand-color) 0%, #266eff 100%);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 20px;
  flex-shrink: 0;
  box-shadow: 0 4px 6px -1px rgba(0, 82, 217, 0.2);
}

.template-info {
  flex: 1;
  min-width: 0;
  padding-top: 2px;
}

.template-name {
  font-weight: 600;
  font-size: 16px;
  margin-bottom: 6px;
  color: var(--td-text-color-primary);
}

.template-desc {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
  white-space: normal;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.template-desc-inline {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
</style>
