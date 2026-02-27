<template>
  <div class="template-detail">
    <!-- Header Area -->
    <div class="detail-header">
      <t-breadcrumb :max-item-width="'150px'">
        <t-breadcrumb-item to="/templates">模板列表</t-breadcrumb-item>
        <t-breadcrumb-item>{{ template?.displayName || template?.name }}</t-breadcrumb-item>
      </t-breadcrumb>

      <div class="header-content">
        <div class="header-left">
          <t-button variant="text" shape="square" @click="goBack">
            <template #icon><chevron-left-icon /></template>
          </t-button>
          <div class="header-info">
            <div class="title-row">
              <h1>{{ template?.displayName || template?.name }}</h1>
              <t-tag v-if="template?.latestVersion" theme="primary" variant="light">
                v{{ template.latestVersion }}
              </t-tag>
              <t-tag v-if="template?.isPublic" theme="success" variant="light">Public</t-tag>
              <t-tag v-else theme="warning" variant="light">Private</t-tag>
            </div>
            <div class="desc-row" v-if="template?.description">
              {{ template.description }}
            </div>
          </div>
        </div>
        <div class="header-actions">
          <t-button theme="default" @click="exportTemplate">
            <template #icon><download-icon /></template>
            导出
          </t-button>
          <t-button theme="default" @click="openEditDialog">
            <template #icon><edit-icon /></template>
            编辑
          </t-button>
          <t-button theme="primary" @click="createSandbox">
            <template #icon><add-icon /></template>
            创建 Sandbox
          </t-button>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <t-row :gutter="16" class="detail-main">
      <!-- Left Column (Main Info) -->
      <t-col :span="8">
        <t-space direction="vertical" size="large" style="width: 100%">
          <!-- Environment Variables -->
          <t-card title="环境变量" :bordered="false" header-bordered>
            <template #actions>
              <t-button
                v-if="envVars.length"
                theme="default"
                variant="text"
                size="small"
                @click="copyEnvVars"
              >
                <template #icon><copy-icon /></template>
                复制全部
              </t-button>
            </template>
            <div v-if="envVars.length" class="env-list">
              <t-row :gutter="16" v-for="[k, v] in envVars" :key="k" class="env-item">
                <t-col :span="4" class="env-key" title="Key">{{ k }}</t-col>
                <t-col :span="8" class="env-value" title="Value">{{ v }}</t-col>
              </t-row>
            </div>
            <div v-else class="empty-state">未配置环境变量</div>
          </t-card>

          <!-- Startup Script -->
          <t-card title="启动脚本" :bordered="false" header-bordered>
            <div v-if="template?.spec?.startupScript" class="script-container">
              <pre class="code-block">{{ template.spec.startupScript }}</pre>
            </div>
            <div v-else class="empty-state">未配置启动脚本</div>
          </t-card>

          <!-- Files -->
          <t-card title="预置文件" :bordered="false" header-bordered>
            <div v-if="template?.spec?.files?.length">
              <t-tabs theme="card">
                <t-tab-panel
                  v-for="(file, index) in template.spec.files"
                  :key="index"
                  :value="index"
                  :label="file.destination"
                >
                  <div class="file-content">
                    <pre class="code-block">{{ file.content }}</pre>
                  </div>
                </t-tab-panel>
              </t-tabs>
            </div>
            <div v-else class="empty-state">未配置预置文件</div>
          </t-card>

          <!-- Version History -->
          <t-card title="版本历史" :bordered="false" header-bordered>
            <t-table
              :data="versions"
              :columns="versionColumns"
              :loading="versionsLoading"
              size="small"
              row-key="id"
              :pagination="{ pageSize: 5 }"
            >
              <template #version="{ row }">
                <t-tag theme="primary" variant="light">v{{ row.version }}</t-tag>
              </template>
              <template #createdAt="{ row }">
                {{ formatTime(row.createdAt) }}
              </template>
              <template #operation="{ row }">
                <t-space size="small">
                  <t-link theme="primary" @click="viewVersion(row)">详情</t-link>
                  <t-popconfirm
                    content="确定要回滚到此版本吗？"
                    v-if="row.version !== template?.latestVersion"
                    @confirm="rollback(row.version)"
                  >
                    <t-link theme="warning">回滚</t-link>
                  </t-popconfirm>
                </t-space>
              </template>
            </t-table>
          </t-card>
        </t-space>
      </t-col>

      <!-- Right Column (Sidebar) -->
      <t-col :span="4">
        <t-space direction="vertical" size="large" style="width: 100%">
          <!-- Metadata -->
          <t-card title="基本信息" :bordered="false" header-bordered>
            <t-descriptions :column="1" layout="vertical">
              <t-descriptions-item label="ID">
                <span class="mono">{{ template?.id }}</span>
              </t-descriptions-item>
              <t-descriptions-item label="英文名称">
                <span class="mono">{{ template?.name }}</span>
              </t-descriptions-item>
              <t-descriptions-item label="作者">{{ template?.author || '-' }}</t-descriptions-item>
              <t-descriptions-item label="创建于">{{
                formatTime(template?.createdAt)
              }}</t-descriptions-item>
              <t-descriptions-item label="更新于">{{
                formatTime(template?.updatedAt)
              }}</t-descriptions-item>
              <t-descriptions-item label="标签">
                <t-space size="small" break-line>
                  <t-tag v-for="tag in template?.tags" :key="tag" variant="outline" size="small">{{
                    tag
                  }}</t-tag>
                </t-space>
              </t-descriptions-item>
            </t-descriptions>
          </t-card>

          <!-- Resource Spec -->
          <t-card title="资源配置" :bordered="false" header-bordered>
            <t-descriptions :column="1" layout="vertical">
              <t-descriptions-item label="镜像">
                <div class="image-box">
                  <span class="mono">{{ template?.spec?.image }}</span>
                  <t-button
                    variant="text"
                    size="small"
                    shape="square"
                    @click="copyText(template?.spec?.image)"
                  >
                    <template #icon><copy-icon /></template>
                  </t-button>
                </div>
              </t-descriptions-item>
              <t-descriptions-item label="Command" v-if="template?.spec?.command?.length">
                <span class="mono">{{ template.spec.command.join(' ') }}</span>
              </t-descriptions-item>
              <t-descriptions-item label="Args" v-if="template?.spec?.args?.length">
                <span class="mono">{{ template.spec.args.join(' ') }}</span>
              </t-descriptions-item>
            </t-descriptions>
            <t-divider dashed />
            <t-row :gutter="8">
              <t-col :span="4">
                <div class="stat-item">
                  <div class="stat-label">CPU</div>
                  <div class="stat-value">{{ template?.spec?.resources?.cpu }}</div>
                </div>
              </t-col>
              <t-col :span="4">
                <div class="stat-item">
                  <div class="stat-label">Memory</div>
                  <div class="stat-value">{{ template?.spec?.resources?.memory }}</div>
                </div>
              </t-col>
              <t-col :span="4">
                <div class="stat-item">
                  <div class="stat-label">TTL</div>
                  <div class="stat-value">{{ template?.spec?.ttl }}s</div>
                </div>
              </t-col>
            </t-row>
          </t-card>

          <!-- Network -->
          <t-card title="网络配置" :bordered="false" header-bordered>
            <t-list :split="false">
              <t-list-item>
                <t-list-item-meta title="公网访问" />
                <template #action>
                  <t-tag
                    v-if="template?.spec?.network?.allowInternetAccess"
                    theme="success"
                    variant="light"
                    >允许</t-tag
                  >
                  <t-tag v-else theme="default" variant="light">禁止</t-tag>
                </template>
              </t-list-item>
              <t-list-item v-if="template?.spec?.network?.allowedDomains?.length">
                <t-list-item-meta title="域名白名单" />
              </t-list-item>
            </t-list>
            <div v-if="template?.spec?.network?.allowedDomains?.length" class="domain-list">
              <t-tag v-for="d in template.spec.network.allowedDomains" :key="d" size="small">{{
                d
              }}</t-tag>
            </div>
          </t-card>

          <!-- Readiness Probe -->
          <t-card
            title="就绪探针"
            :bordered="false"
            header-bordered
            v-if="template?.spec?.readinessProbe"
          >
            <div class="probe-cmd mono">
              {{ template.spec.readinessProbe.exec.command.join(' ') }}
            </div>
            <div class="probe-stats">
              <span>Delay: {{ template.spec.readinessProbe.initialDelaySeconds }}s</span>
              <t-divider layout="vertical" />
              <span>Period: {{ template.spec.readinessProbe.periodSeconds }}s</span>
              <t-divider layout="vertical" />
              <span>Fail: {{ template.spec.readinessProbe.failureThreshold }}</span>
            </div>
          </t-card>
        </t-space>
      </t-col>
    </t-row>

    <!-- Edit Dialog -->
    <t-dialog
      v-model:visible="showEditDialog"
      header="编辑模板"
      width="640px"
      :confirm-btn="{ content: '更新', loading: saving }"
      @confirm="saveTemplate"
    >
      <TemplateForm ref="templateFormRef" :is-edit="true" :initial-data="editFormData" />
    </t-dialog>

    <!-- Version Detail Dialog -->
    <t-dialog v-model:visible="showVersionDetail" header="版本详情" width="800px" :footer="false">
      <div v-if="selectedVersion" class="version-detail-content">
        <div class="version-meta">
          <t-descriptions :column="3" size="small">
            <t-descriptions-item label="版本">v{{ selectedVersion.version }}</t-descriptions-item>
            <t-descriptions-item label="创建者">{{
              selectedVersion.createdBy
            }}</t-descriptions-item>
            <t-descriptions-item label="创建时间">{{
              formatTime(selectedVersion.createdAt)
            }}</t-descriptions-item>
          </t-descriptions>
          <div class="version-changelog">
            <span class="label">变更日志:</span>
            <span class="text">{{ selectedVersion.changelog }}</span>
          </div>
        </div>
        <t-divider />

        <h4>规格配置快照</h4>
        <div class="code-container">
          <pre class="code-block">{{ JSON.stringify(selectedVersion.spec, null, 2) }}</pre>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { ChevronLeftIcon, AddIcon, DownloadIcon, EditIcon, CopyIcon } from 'tdesign-icons-vue-next'
import {
  templateApi,
  type Template,
  type TemplateVersion,
  type UpdateTemplateRequest,
  type CreateTemplateRequest,
} from '../api/template'
import TemplateForm from '../components/TemplateForm.vue'

const router = useRouter()
const route = useRoute()

const template = ref<Template | null>(null)
const versions = ref<TemplateVersion[]>([])
const versionsLoading = ref(false)

// Edit Dialog
const showEditDialog = ref(false)
const saving = ref(false)
const templateFormRef = ref()
const editFormData = ref<(CreateTemplateRequest & { autoPrepull?: boolean }) | null>(null)

// Version Detail
const showVersionDetail = ref(false)
const selectedVersion = ref<TemplateVersion | null>(null)

const versionColumns = [
  { colKey: 'version', title: '版本', width: 80 },
  { colKey: 'changelog', title: '变更日志', ellipsis: true },
  { colKey: 'createdBy', title: '创建者', width: 100 },
  { colKey: 'createdAt', title: '时间', width: 160 },
  { colKey: 'operation', title: '操作', width: 120, fixed: 'right' },
]

const envVars = computed(() => {
  if (!template.value?.spec?.env) return []
  return Object.entries(template.value.spec.env)
})

const formatTime = (time?: string) => {
  if (!time) return '-'
  return new Date(time).toLocaleString()
}

const loadTemplate = async () => {
  const name = route.params.name as string
  try {
    const res = await templateApi.get(name)
    template.value = res.data
  } catch (err: any) {
    MessagePlugin.error('加载失败: ' + (err.response?.data?.error?.message || err.message))
    goBack()
  }
}

const loadVersions = async () => {
  const name = route.params.name as string
  versionsLoading.value = true
  try {
    const res = await templateApi.listVersions(name)
    versions.value = (res.data.items || []).sort((a, b) => b.version - a.version)
  } catch (err: any) {
    console.error('Failed to load versions:', err)
  } finally {
    versionsLoading.value = false
  }
}

const goBack = () => {
  router.push('/templates')
}

const createSandbox = () => {
  router.push({ path: '/sandboxes', query: { template: template.value?.name } })
}

const exportTemplate = async () => {
  if (!template.value) return
  try {
    const res = await templateApi.exportOne(template.value.name)
    downloadBlob(res.data, `${template.value.name}.yaml`)
    MessagePlugin.success('导出成功')
  } catch (err: any) {
    MessagePlugin.error('导出失败: ' + (err.message || '未知错误'))
  }
}

const openEditDialog = () => {
  if (!template.value) return

  const tmpl = template.value
  editFormData.value = {
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
  showEditDialog.value = true
}

const saveTemplate = async () => {
  const valid = await templateFormRef.value?.validate()
  if (valid !== true) return

  if (!template.value) return

  saving.value = true
  try {
    const requestData = templateFormRef.value.getData()
    const updateData: UpdateTemplateRequest = {
      displayName: requestData.displayName,
      description: requestData.description,
      tags: requestData.tags,
      spec: requestData.spec,
      changelog: '通过 Web UI 更新',
    }

    await templateApi.update(template.value.name, updateData)
    MessagePlugin.success('更新成功')
    showEditDialog.value = false
    loadTemplate()
    loadVersions()
  } catch (err: any) {
    MessagePlugin.error('更新失败: ' + (err.response?.data?.error?.message || err.message))
  } finally {
    saving.value = false
  }
}

const viewVersion = (version: TemplateVersion) => {
  selectedVersion.value = version
  showVersionDetail.value = true
}

const rollback = async (version: number) => {
  if (!template.value) return
  try {
    await templateApi.rollback(template.value.name, {
      targetVersion: version,
      changelog: `从 v${version} 回滚`,
    })
    MessagePlugin.success('回滚成功')
    loadTemplate()
    loadVersions()
  } catch (err: any) {
    MessagePlugin.error('回滚失败: ' + (err.response?.data?.error?.message || err.message))
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

const copyEnvVars = () => {
  if (!envVars.value.length) return
  const text = envVars.value.map(([k, v]) => `${k}=${v}`).join('\n')
  copyText(text)
}

const copyText = (text?: string) => {
  if (!text) return
  navigator.clipboard.writeText(text).then(() => {
    MessagePlugin.success('已复制')
  })
}

onMounted(() => {
  loadTemplate()
  loadVersions()
})
</script>

<style scoped>
.template-detail {
  padding: 0;
}

.detail-header {
  background: var(--td-bg-color-container);
  padding: 16px 24px;
  border-bottom: 1px solid var(--td-component-border);
  margin-bottom: 24px;
}

.header-content {
  margin-top: 16px;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.header-left {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.header-info .title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.header-info h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.desc-row {
  color: var(--td-text-color-secondary);
  font-size: 14px;
  max-width: 600px;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.detail-main {
  padding: 0 24px 24px;
}

.mono {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
  word-break: break-all;
}

/* Env Vars Styling */
.env-list {
  background: var(--td-bg-color-secondary-container);
  border-radius: 4px;
  padding: 8px 12px;
}

.env-item {
  padding: 8px 0;
  border-bottom: 1px dashed var(--td-component-border);
}

.env-item:last-child {
  border-bottom: none;
}

.env-key {
  color: var(--td-brand-color);
  font-weight: 500;
  font-family: monospace;
}

.env-value {
  color: var(--td-text-color-primary);
  font-family: monospace;
  word-break: break-all;
}

/* Code Block */
.code-block {
  margin: 0;
  padding: 12px;
  background: var(--td-bg-color-secondary-container);
  border-radius: 4px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
  color: var(--td-text-color-primary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 400px;
  overflow-y: auto;
}

.empty-state {
  color: var(--td-text-color-placeholder);
  text-align: center;
  padding: 24px;
  background: var(--td-bg-color-secondary-container);
  border-radius: 4px;
  font-size: 13px;
}

/* Files */
.file-content {
  margin-top: -1px; /* Overlap border */
}

/* Stats */
.stat-item {
  text-align: center;
  padding: 12px;
  background: var(--td-bg-color-secondary-container);
  border-radius: 4px;
}

.stat-label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-bottom: 4px;
}

.stat-value {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.image-box {
  background: var(--td-bg-color-secondary-container);
  padding: 8px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.domain-list {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.probe-cmd {
  padding: 8px;
  background: var(--td-bg-color-secondary-container);
  border-radius: 4px;
  margin-bottom: 12px;
}

.probe-stats {
  display: flex;
  gap: 12px;
  align-items: center;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.version-meta {
  margin-bottom: 16px;
}

.version-changelog {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}

.version-changelog .label {
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}

.version-changelog .text {
  color: var(--td-text-color-primary);
}

.code-container {
  max-height: 500px;
  overflow-y: auto;
}
</style>
