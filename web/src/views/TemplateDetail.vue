<template>
  <div class="template-detail">
    <t-space direction="vertical" size="large" style="width: 100%">
      <!-- Header -->
      <t-card :bordered="false">
        <t-space align="center">
          <t-button theme="default" variant="text" @click="goBack">
            <template #icon><chevron-left-icon /></template>
            返回
          </t-button>
          <t-divider layout="vertical" />
          <h2 style="margin: 0">{{ template?.displayName || template?.name }}</h2>
          <t-tag v-if="template?.tags" v-for="tag in template.tags" :key="tag" variant="light">
            {{ tag }}
          </t-tag>
        </t-space>
      </t-card>

      <!-- Template Info -->
      <t-row :gutter="16">
        <t-col :span="8">
          <t-card title="基本信息" :bordered="false">
            <t-descriptions :column="1">
              <t-descriptions-item label="名称">{{ template?.name }}</t-descriptions-item>
              <t-descriptions-item label="显示名称">{{
                template?.displayName || '-'
              }}</t-descriptions-item>
              <t-descriptions-item label="描述">{{
                template?.description || '-'
              }}</t-descriptions-item>
              <t-descriptions-item label="作者">{{ template?.author || '-' }}</t-descriptions-item>
              <t-descriptions-item label="最新版本">
                <t-tag theme="primary" variant="light">v{{ template?.latestVersion }}</t-tag>
              </t-descriptions-item>
              <t-descriptions-item label="创建时间">{{
                formatTime(template?.createdAt)
              }}</t-descriptions-item>
            </t-descriptions>
          </t-card>
        </t-col>

        <t-col :span="8">
          <t-card title="资源配置" :bordered="false">
            <t-descriptions :column="1">
              <t-descriptions-item label="镜像">
                <span class="mono">{{ template?.spec?.image }}</span>
              </t-descriptions-item>
              <t-descriptions-item label="CPU">{{
                template?.spec?.resources?.cpu
              }}</t-descriptions-item>
              <t-descriptions-item label="内存">{{
                template?.spec?.resources?.memory
              }}</t-descriptions-item>
              <t-descriptions-item label="TTL">{{ template?.spec?.ttl }} 秒</t-descriptions-item>
            </t-descriptions>
          </t-card>
        </t-col>

        <t-col :span="8">
          <t-card title="操作" :bordered="false">
            <t-space direction="vertical" style="width: 100%">
              <t-button theme="primary" block @click="createSandbox">
                <template #icon><add-icon /></template>
                创建 Sandbox
              </t-button>
              <t-button theme="default" block @click="exportTemplate">
                <template #icon><download-icon /></template>
                导出 YAML
              </t-button>
              <t-button theme="default" block @click="editTemplate">
                <template #icon><edit-icon /></template>
                编辑模板
              </t-button>
            </t-space>
          </t-card>
        </t-col>
      </t-row>

      <!-- Environment Variables -->
      <t-card title="环境变量" :bordered="false" v-if="envVars.length">
        <div class="code-block">
          <div v-for="[k, v] in envVars" :key="k" class="env-line">
            <span class="env-key">{{ k }}</span>
            <span class="env-separator">=</span>
            <span class="env-value">{{ v }}</span>
          </div>
        </div>
      </t-card>

      <!-- Startup Script -->
      <t-card title="启动脚本" :bordered="false" v-if="template?.spec?.startupScript">
        <div class="code-block">{{ template.spec.startupScript }}</div>
      </t-card>

      <!-- Files -->
      <t-card title="预置文件" :bordered="false" v-if="template?.spec?.files?.length">
        <t-table :data="template.spec.files" :columns="fileColumns" size="small">
          <template #destination="{ row }">
            <span class="mono">{{ row.destination }}</span>
          </template>
          <template #content="{ row }">
            <span class="content-preview">{{ row.content?.substring(0, 50) }}...</span>
          </template>
        </t-table>
      </t-card>

      <!-- Readiness Probe -->
      <t-card title="就绪探针" :bordered="false" v-if="template?.spec?.readinessProbe">
        <t-descriptions :column="2">
          <t-descriptions-item label="命令">
            <span class="mono">{{ template.spec.readinessProbe.exec.command.join(' ') }}</span>
          </t-descriptions-item>
          <t-descriptions-item label="初始延迟">
            {{ template.spec.readinessProbe.initialDelaySeconds }}s
          </t-descriptions-item>
          <t-descriptions-item label="检查周期">
            {{ template.spec.readinessProbe.periodSeconds }}s
          </t-descriptions-item>
          <t-descriptions-item label="失败阈值">
            {{ template.spec.readinessProbe.failureThreshold }}
          </t-descriptions-item>
        </t-descriptions>
      </t-card>

      <!-- Version History -->
      <t-card title="版本历史" :bordered="false">
        <t-table
          :data="versions"
          :columns="versionColumns"
          :loading="versionsLoading"
          size="small"
          row-key="id"
        >
          <template #version="{ row }">
            <t-tag theme="primary" variant="light">v{{ row.version }}</t-tag>
          </template>
          <template #createdAt="{ row }">
            {{ formatTime(row.createdAt) }}
          </template>
          <template #operation="{ row }">
            <t-space>
              <t-link theme="primary" @click="viewVersion(row)">查看</t-link>
              <t-popconfirm
                content="回滚到此版本将创建新版本，确定吗？"
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

    <!-- Version Detail Dialog -->
    <t-dialog v-model:visible="showVersionDetail" header="版本详情" width="800px">
      <t-descriptions v-if="selectedVersion" :column="1" bordered>
        <t-descriptions-item label="版本">v{{ selectedVersion.version }}</t-descriptions-item>
        <t-descriptions-item label="创建者">{{ selectedVersion.createdBy }}</t-descriptions-item>
        <t-descriptions-item label="创建时间">{{
          formatTime(selectedVersion.createdAt)
        }}</t-descriptions-item>
        <t-descriptions-item label="变更日志">{{ selectedVersion.changelog }}</t-descriptions-item>
      </t-descriptions>
      <t-divider />
      <div class="version-spec">
        <h4>规格配置</h4>
        <pre class="code-block">{{ JSON.stringify(selectedVersion?.spec, null, 2) }}</pre>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { ChevronLeftIcon, AddIcon, DownloadIcon, EditIcon } from 'tdesign-icons-vue-next'
import { templateApi, type Template, type TemplateVersion } from '../api/template'

const router = useRouter()
const route = useRoute()

const template = ref<Template | null>(null)
const versions = ref<TemplateVersion[]>([])
const versionsLoading = ref(false)
const showVersionDetail = ref(false)
const selectedVersion = ref<TemplateVersion | null>(null)

const fileColumns = [
  { colKey: 'destination', title: '目标路径', ellipsis: true },
  { colKey: 'content', title: '内容预览', ellipsis: true },
]

const versionColumns = [
  { colKey: 'version', title: '版本', width: 100 },
  { colKey: 'changelog', title: '变更日志', ellipsis: true },
  { colKey: 'createdBy', title: '创建者', width: 120 },
  { colKey: 'createdAt', title: '创建时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 120 },
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

const editTemplate = () => {
  router.push({ path: '/templates', query: { action: 'edit', name: template.value?.name } })
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

onMounted(() => {
  loadTemplate()
  loadVersions()
})
</script>

<style scoped>
.template-detail {
  padding: 24px;
}

.code-block {
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);
  border-radius: 4px;
  padding: 12px;
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
}

.mono {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 12px;
}

.env-line {
  padding: 4px 0;
}

.env-key {
  color: var(--td-brand-color);
}

.env-separator {
  margin: 0 8px;
  color: var(--td-text-color-secondary);
}

.env-value {
  color: var(--td-success-color);
}

.content-preview {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.version-spec h4 {
  margin-bottom: 8px;
}
</style>
