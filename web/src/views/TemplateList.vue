<template>
  <div class="template-list">
    <t-card title="模板管理" :bordered="false">
      <template #actions>
        <t-space>
          <t-button theme="success" variant="outline" @click="showImportDialog = true">
            <template #icon><import-icon /></template>
            导入
          </t-button>
          <t-button theme="default" variant="outline" @click="exportAll">
            <template #icon><export-icon /></template>
            导出
          </t-button>
          <t-button theme="primary" @click="openCreateDialog">
            <template #icon><add-icon /></template>
            新建模板
          </t-button>
        </t-space>
      </template>

      <t-tabs v-model="activeTab" @change="onTabChange">
        <t-tab-panel value="all" label="全部模板">
          <t-table
            :data="filteredTemplates"
            :columns="columns"
            :loading="loading"
            row-key="id"
            hover
            :pagination="pagination"
            @page-change="onPageChange"
          >
            <template #name="{ row }">
              <t-link theme="primary" @click="goToDetail(row.name)">
                {{ row.displayName || row.name }}
              </t-link>
              <div class="template-tags" v-if="row.tags && row.tags.length">
                <t-tag v-for="tag in row.tags" :key="tag" size="small" variant="light">
                  {{ tag }}
                </t-tag>
              </div>
            </template>
            <template #image="{ row }">
              <span class="image-text">{{ row.spec?.image || '-' }}</span>
            </template>
            <template #resources="{ row }">
              {{ row.spec?.resources?.cpu || '-' }} / {{ row.spec?.resources?.memory || '-' }}
            </template>
            <template #version="{ row }">
              <t-tag theme="primary" variant="light">v{{ row.latestVersion }}</t-tag>
            </template>
            <template #createdAt="{ row }">
              {{ formatTime(row.createdAt) }}
            </template>
            <template #operation="{ row }">
              <t-space>
                <t-dropdown
                  :options="getDropdownOptions(row)"
                  @click="(ctx: any) => onActionClick(ctx.value, row)"
                >
                  <t-link theme="primary">
                    <template #suffix>
                      <chevron-down-icon />
                    </template>
                    操作
                  </t-link>
                </t-dropdown>
              </t-space>
            </template>
          </t-table>
        </t-tab-panel>
        <t-tab-panel value="prepull" label="镜像预拉取">
          <PrepullPanel />
        </t-tab-panel>
      </t-tabs>
    </t-card>

    <!-- Create/Edit Dialog -->
    <t-dialog
      v-model:visible="showCreateDialog"
      :header="isEdit ? '编辑模板' : '新建模板'"
      width="640px"
      :confirm-btn="{ content: isEdit ? '更新' : '创建', loading: saving }"
      @confirm="saveTemplate"
    >
      <TemplateForm ref="templateFormRef" :is-edit="isEdit" :initial-data="formData" />
    </t-dialog>

    <!-- Import Dialog -->
    <t-dialog
      v-model:visible="showImportDialog"
      header="导入模板"
      width="500px"
      :confirm-btn="{ content: '导入', loading: importing }"
      @confirm="importTemplates"
    >
      <t-form :data="importForm" label-width="100px">
        <t-form-item label="YAML 文件">
          <t-upload
            v-model="importForm.files"
            theme="file-input"
            accept=".yaml,.yml"
            :auto-upload="false"
          />
        </t-form-item>
        <t-form-item label="导入策略">
          <t-radio-group v-model="importForm.strategy">
            <t-radio value="create-or-update">创建或更新</t-radio>
            <t-radio value="create-only">仅创建</t-radio>
            <t-radio value="update-only">仅更新</t-radio>
          </t-radio-group>
        </t-form-item>
        <t-form-item label="自动预拉取">
          <t-switch v-model="importForm.prepull" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- Export Dialog -->
    <t-dialog
      v-model:visible="showExportDialog"
      header="导出模板"
      width="400px"
      @confirm="doExport"
    >
      <t-form :data="exportForm" label-width="100px">
        <t-form-item label="标签过滤">
          <t-input v-model="exportForm.tag" placeholder="按标签过滤" />
        </t-form-item>
        <t-form-item label="模板名称">
          <t-input v-model="exportForm.names" placeholder="逗号分隔，如: python,node" />
        </t-form-item>
      </t-form>
      <template #footer>
        <t-button @click="showExportDialog = false">取消</t-button>
        <t-button theme="primary" @click="doExport">导出</t-button>
      </template>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { AddIcon, ImportIcon, ExportIcon, ChevronDownIcon } from 'tdesign-icons-vue-next'
import {
  templateApi,
  type Template,
  type CreateTemplateRequest,
  type UpdateTemplateRequest,
} from '../api/template'
import PrepullPanel from '../components/PrepullPanel.vue'
import TemplateForm from '../components/TemplateForm.vue'

const router = useRouter()
const route = useRoute()

const templates = ref<Template[]>([])
const loading = ref(false)
const saving = ref(false)
const importing = ref(false)
const showCreateDialog = ref(false)
const showImportDialog = ref(false)
const showExportDialog = ref(false)
const isEdit = ref(false)
const editingTemplate = ref<Template | null>(null)
const templateFormRef = ref()
const activeTab = ref('all')

// Initial data for the form
const formData = ref<(CreateTemplateRequest & { autoPrepull?: boolean }) | null>(null)

const defaultFormData: CreateTemplateRequest & { autoPrepull?: boolean } = {
  name: '',
  displayName: '',
  description: '',
  tags: [],
  spec: {
    image: 'python:3.11-slim',
    command: [],
    args: [],
    resources: { cpu: '500m', memory: '512Mi' },
    ttl: 3600,
    network: { allowInternetAccess: false, allowedDomains: [] },
    env: {},
    startupTimeout: 300,
  },
  autoPrepull: false,
}

const importForm = ref({
  files: [] as any[],
  strategy: 'create-or-update',
  prepull: false,
})

const exportForm = ref({
  tag: '',
  names: '',
})

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
})

const columns = [
  { colKey: 'name', title: '名称', ellipsis: true },
  { colKey: 'image', title: '镜像', ellipsis: true },
  { colKey: 'resources', title: '资源', width: 120 },
  { colKey: 'version', title: '版本', width: 80 },
  { colKey: 'createdAt', title: '创建时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 100 },
]

const filteredTemplates = computed(() => {
  const start = (pagination.value.current - 1) * pagination.value.pageSize
  const end = start + pagination.value.pageSize
  return templates.value.slice(start, end)
})

const formatTime = (time: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const loadTemplates = async () => {
  loading.value = true
  try {
    const res = await templateApi.list()
    templates.value = res.data.items || []
    pagination.value.total = templates.value.length
  } catch (err: any) {
    MessagePlugin.error('加载失败: ' + (err.response?.data?.error?.message || err.message))
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  isEdit.value = false
  editingTemplate.value = null
  // Reset form data by creating a deep copy of default
  formData.value = JSON.parse(JSON.stringify(defaultFormData))
  showCreateDialog.value = true
}

const saveTemplate = async () => {
  const valid = await templateFormRef.value?.validate()
  if (valid !== true) return

  saving.value = true
  try {
    const data = templateFormRef.value.getData()
    // Make a copy to avoid mutating the form data directly if needed,
    // although getData returns the reactive object from component.
    const requestData = { ...data }
    delete (requestData as any).autoPrepull

    if (isEdit.value && editingTemplate.value) {
      const updateData: UpdateTemplateRequest = {
        displayName: requestData.displayName,
        description: requestData.description,
        tags: requestData.tags,
        spec: requestData.spec,
        changelog: '通过 Web UI 更新',
      }
      await templateApi.update(editingTemplate.value.name, updateData)
      MessagePlugin.success('更新成功')
    } else {
      await templateApi.create(requestData)
      MessagePlugin.success('创建成功')
    }
    showCreateDialog.value = false
    loadTemplates()
  } catch (err: any) {
    MessagePlugin.error(
      (isEdit.value ? '更新' : '创建') +
        '失败: ' +
        (err.response?.data?.error?.message || err.message)
    )
  } finally {
    saving.value = false
  }
}

const editTemplate = (tmpl: Template) => {
  isEdit.value = true
  editingTemplate.value = tmpl
  formData.value = {
    name: tmpl.name,
    displayName: tmpl.displayName,
    description: tmpl.description,
    tags: tmpl.tags || [],
    spec: {
      image: tmpl.spec?.image || '',
      command: tmpl.spec?.command ?? [],
      args: tmpl.spec?.args ?? [],
      resources: tmpl.spec?.resources || { cpu: '', memory: '' },
      ttl: tmpl.spec?.ttl || 3600,
      network: {
        allowInternetAccess: tmpl.spec?.network?.allowInternetAccess ?? false,
        allowedDomains: tmpl.spec?.network?.allowedDomains ?? [],
      },
      env: tmpl.spec?.env || {},
      startupScript: tmpl.spec?.startupScript || '',
      startupTimeout: tmpl.spec?.startupTimeout || 300,
    },
    autoPrepull: false,
  }
  showCreateDialog.value = true
}

const getDropdownOptions = (_tmpl: Template) => {
  return [
    { content: '创建 Sandbox', value: 'create' },
    { content: '查看版本', value: 'versions' },
    { content: '编辑', value: 'edit' },
    { content: '导出', value: 'export' },
    { content: '删除', value: 'delete', theme: 'error' },
  ]
}

const onActionClick = async (value: string, tmpl: Template) => {
  switch (value) {
    case 'create':
      router.push({ path: '/sandboxes', query: { template: tmpl.name } })
      break
    case 'versions':
      goToDetail(tmpl.name)
      break
    case 'edit':
      editTemplate(tmpl)
      break
    case 'export':
      exportOne(tmpl.name)
      break
    case 'delete': {
      const dialog = DialogPlugin({
        header: '确认删除',
        body: `确定要删除模板 "${tmpl.displayName || tmpl.name}" 吗？`,
        confirmBtn: {
          content: '确定',
          theme: 'danger',
        },
        onConfirm: async () => {
          try {
            await templateApi.delete(tmpl.name)
            MessagePlugin.success('删除成功')
            loadTemplates()
            dialog.hide()
          } catch (err: any) {
            MessagePlugin.error('删除失败: ' + (err.response?.data?.error?.message || err.message))
          }
        },
      })
      break
    }
  }
}

const exportAll = () => {
  showExportDialog.value = true
}

const doExport = async () => {
  try {
    const params: any = {}
    if (exportForm.value.tag) params.tag = exportForm.value.tag
    if (exportForm.value.names) params.names = exportForm.value.names
    const res = await templateApi.exportAll(params)
    downloadBlob(res.data, 'templates.yaml')
    showExportDialog.value = false
    MessagePlugin.success('导出成功')
  } catch (err: any) {
    MessagePlugin.error('导出失败: ' + (err.message || '未知错误'))
  }
}

const exportOne = async (name: string) => {
  try {
    const res = await templateApi.exportOne(name)
    downloadBlob(res.data, `${name}.yaml`)
    MessagePlugin.success('导出成功')
  } catch (err: any) {
    MessagePlugin.error('导出失败: ' + (err.message || '未知错误'))
  }
}

const downloadBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

const importTemplates = async () => {
  if (!importForm.value.files.length) {
    MessagePlugin.warning('请选择文件')
    return
  }
  const file = importForm.value.files[0]
  if (!file?.raw) return

  importing.value = true
  try {
    const res = await templateApi.import(
      file.raw,
      importForm.value.strategy,
      importForm.value.prepull
    )
    const { total, created, updated, skipped, failed } = res.data
    MessagePlugin.success(
      `导入完成: 共 ${total} 个，创建 ${created}，更新 ${updated}，跳过 ${skipped}，失败 ${failed}`
    )
    if (res.data.prepullStarted && res.data.prepullStarted.length > 0) {
      MessagePlugin.info(`已启动 ${res.data.prepullStarted.length} 个镜像预拉取任务`)
    }
    showImportDialog.value = false
    importForm.value.files = []
    loadTemplates()
  } catch (err: any) {
    MessagePlugin.error('导入失败: ' + (err.response?.data?.error?.message || err.message))
  } finally {
    importing.value = false
  }
}

const goToDetail = (name: string) => {
  router.push(`/templates/${name}`)
}

const onPageChange = (pageInfo: any) => {
  pagination.value.current = pageInfo.current
  pagination.value.pageSize = pageInfo.pageSize
}

const onTabChange = (value: string) => {
  if (value === 'prepull') {
    activeTab.value = 'prepull'
  }
}

// Watch for route query params to handle edit action from other pages
watch(
  () => route.query,
  async (query) => {
    if (query.action === 'edit' && query.name) {
      const templateName = query.name as string
      // Clean up URL params to prevent re-triggering
      router.replace({ path: '/templates', query: {} })

      // Wait for templates to load if needed
      if (templates.value.length === 0) {
        await loadTemplates()
      }

      // Find the template in the loaded list
      const template = templates.value.find((t) => t.name === templateName)
      if (template) {
        editTemplate(template)
      } else {
        // If not found in list, fetch it directly
        try {
          const res = await templateApi.get(templateName)
          editTemplate(res.data)
        } catch (err: any) {
          MessagePlugin.error(
            '加载模板失败: ' + (err.response?.data?.error?.message || err.message)
          )
        }
      }
    }
  },
  { immediate: true }
)

onMounted(() => {
  loadTemplates()
})
</script>

<style scoped>
.template-list {
  padding: 24px;
}

.template-tags {
  display: flex;
  gap: 4px;
  margin-top: 4px;
}

.image-text {
  font-family: monospace;
  font-size: 12px;
}
</style>
