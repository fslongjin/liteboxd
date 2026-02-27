<template>
  <div class="sandbox-detail">
    <t-card :bordered="false">
      <template #title>
        <div class="header-title">
          <t-button variant="text" shape="square" @click="$router.push('/sandboxes')">
            <template #icon><t-icon name="arrow-left" /></template>
          </t-button>
          <span class="title-text">Sandbox: {{ sandbox?.id }}</span>
          <t-tag :theme="getStatusTheme(sandbox?.status || '')" variant="light-outline">
            {{ getStatusText(sandbox?.status || '') }}
          </t-tag>
        </div>
      </template>
      <template #actions>
        <t-space>
          <t-button theme="primary" @click="openTerminal" :disabled="sandbox?.status !== 'running'">
            登录沙箱
          </t-button>
          <t-popconfirm content="确定要删除该 Sandbox 吗？" @confirm="deleteSandbox">
            <t-button theme="danger" variant="outline">删除</t-button>
          </t-popconfirm>
        </t-space>
      </template>

      <t-loading :loading="loading">
        <t-descriptions v-if="sandbox" :column="2">
          <t-descriptions-item label="ID">{{ sandbox.id }}</t-descriptions-item>
          <t-descriptions-item label="镜像">{{ sandbox.image }}</t-descriptions-item>
          <t-descriptions-item label="CPU">{{ sandbox.cpu }}</t-descriptions-item>
          <t-descriptions-item label="内存">{{ sandbox.memory }}</t-descriptions-item>
          <t-descriptions-item label="TTL">{{ sandbox.ttl }} 秒</t-descriptions-item>
          <t-descriptions-item label="状态">
            <t-tag :theme="getStatusTheme(sandbox.status)">{{
              getStatusText(sandbox.status)
            }}</t-tag>
          </t-descriptions-item>
          <t-descriptions-item label="创建时间">{{
            formatTime(sandbox.created_at)
          }}</t-descriptions-item>
          <t-descriptions-item label="过期时间">{{
            formatTime(sandbox.expires_at)
          }}</t-descriptions-item>
        </t-descriptions>
      </t-loading>
    </t-card>

    <t-card title="日志与事件" :bordered="false" class="logs-card">
      <template #actions>
        <t-button variant="outline" size="small" @click="loadLogs">刷新</t-button>
      </template>
      <t-row :gutter="16">
        <t-col :span="6">
          <div class="log-section">
            <div class="log-title">容器日志</div>
            <pre class="log-content">{{ logs || '(无日志)' }}</pre>
          </div>
        </t-col>
        <t-col :span="6">
          <div class="log-section">
            <div class="log-title">Pod 事件</div>
            <div class="event-list">
              <div v-if="events.length === 0" class="no-events">(无事件)</div>
              <div v-for="(event, idx) in events" :key="idx" class="event-item">
                {{ event }}
              </div>
            </div>
          </div>
        </t-col>
      </t-row>
    </t-card>

    <t-card title="执行命令" :bordered="false" class="exec-card">
      <t-space direction="vertical" style="width: 100%">
        <t-input-adornment prepend="$">
          <t-input
            v-model="command"
            placeholder="输入命令，如：python -c &quot;print('hello')&quot;"
            @keyup.enter="executeCommand"
          />
        </t-input-adornment>
        <t-space>
          <t-button theme="primary" :loading="executing" @click="executeCommand"> 执行 </t-button>
          <t-button variant="outline" @click="clearOutput">清空输出</t-button>
        </t-space>
      </t-space>

      <div class="output-area">
        <div class="output-header">
          <span>输出</span>
          <t-tag v-if="lastExitCode !== null" :theme="lastExitCode === 0 ? 'success' : 'danger'">
            Exit: {{ lastExitCode }}
          </t-tag>
        </div>
        <pre class="output-content">{{ output || '(无输出)' }}</pre>
      </div>
    </t-card>

    <t-card title="文件操作" :bordered="false" class="file-card">
      <t-row :gutter="24">
        <t-col :span="6">
          <t-card title="上传文件" :bordered="true" size="small">
            <t-space direction="vertical" style="width: 100%">
              <t-input v-model="uploadPath" placeholder="目标路径，如：/workspace/main.py" />
              <t-upload v-model="uploadFiles" :auto-upload="false" :multiple="false" />
              <t-button
                theme="primary"
                :loading="uploading"
                :disabled="!uploadPath || uploadFiles.length === 0"
                @click="uploadFile"
              >
                上传
              </t-button>
            </t-space>
          </t-card>
        </t-col>
        <t-col :span="6">
          <t-card title="下载文件" :bordered="true" size="small">
            <t-space direction="vertical" style="width: 100%">
              <t-input v-model="downloadPath" placeholder="文件路径，如：/workspace/output.txt" />
              <t-button
                theme="primary"
                :loading="downloading"
                :disabled="!downloadPath"
                @click="downloadFile"
              >
                下载
              </t-button>
            </t-space>
          </t-card>
        </t-col>
      </t-row>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { sandboxApi, type Sandbox } from '../api/sandbox'

const route = useRoute()
const router = useRouter()
const sandboxId = route.params.id as string

const sandbox = ref<Sandbox | null>(null)
const loading = ref(false)
const command = ref('')
const output = ref('')
const executing = ref(false)
const lastExitCode = ref<number | null>(null)

const logs = ref('')
const events = ref<string[]>([])

const uploadPath = ref('')
const uploadFiles = ref<any[]>([])
const uploading = ref(false)
const downloadPath = ref('')
const downloading = ref(false)

const openTerminal = () => {
  const url = router.resolve({ name: 'sandbox-terminal', params: { id: sandboxId } }).href
  window.open(url, `terminal-${sandboxId}`, 'width=960,height=600')
}

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

const loadSandbox = async () => {
  loading.value = true
  try {
    const res = await sandboxApi.get(sandboxId)
    sandbox.value = res.data
  } catch (err: any) {
    MessagePlugin.error('加载失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const loadLogs = async () => {
  try {
    const res = await sandboxApi.getLogs(sandboxId)
    logs.value = res.data.logs || ''
    events.value = res.data.events || []
  } catch (err: any) {
    console.error('Failed to load logs:', err)
  }
}

const executeCommand = async () => {
  if (!command.value.trim()) return

  executing.value = true
  try {
    const cmdParts = parseCommand(command.value)
    const res = await sandboxApi.exec(sandboxId, {
      command: cmdParts,
      timeout: 30,
    })
    lastExitCode.value = res.data.exit_code
    output.value += `$ ${command.value}\n`
    if (res.data.stdout) output.value += res.data.stdout
    if (res.data.stderr) output.value += `[stderr] ${res.data.stderr}`
    output.value += '\n'
    command.value = ''
  } catch (err: any) {
    MessagePlugin.error('执行失败: ' + (err.response?.data?.error || err.message))
  } finally {
    executing.value = false
  }
}

const parseCommand = (cmd: string): string[] => {
  const result: string[] = []
  let current = ''
  let inQuote = false
  let quoteChar = ''

  for (let i = 0; i < cmd.length; i++) {
    const char = cmd[i]
    if ((char === '"' || char === "'") && !inQuote) {
      inQuote = true
      quoteChar = char
    } else if (char === quoteChar && inQuote) {
      inQuote = false
      quoteChar = ''
    } else if (char === ' ' && !inQuote) {
      if (current) {
        result.push(current)
        current = ''
      }
    } else {
      current += char
    }
  }
  if (current) result.push(current)
  return result
}

const clearOutput = () => {
  output.value = ''
  lastExitCode.value = null
}

const uploadFile = async () => {
  if (!uploadPath.value || uploadFiles.value.length === 0) return

  uploading.value = true
  try {
    const file = uploadFiles.value[0].raw
    await sandboxApi.uploadFile(sandboxId, uploadPath.value, file)
    MessagePlugin.success('上传成功')
    uploadPath.value = ''
    uploadFiles.value = []
  } catch (err: any) {
    MessagePlugin.error('上传失败: ' + (err.response?.data?.error || err.message))
  } finally {
    uploading.value = false
  }
}

const downloadFile = async () => {
  if (!downloadPath.value) return

  downloading.value = true
  try {
    const res = await sandboxApi.downloadFile(sandboxId, downloadPath.value)
    const blob = new Blob([res.data])
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = downloadPath.value.split('/').pop() || 'download'
    a.click()
    window.URL.revokeObjectURL(url)
    MessagePlugin.success('下载成功')
  } catch (err: any) {
    MessagePlugin.error('下载失败: ' + (err.response?.data?.error || err.message))
  } finally {
    downloading.value = false
  }
}

const deleteSandbox = async () => {
  try {
    await sandboxApi.delete(sandboxId)
    MessagePlugin.success('删除成功')
    router.push('/sandboxes')
  } catch (err: any) {
    MessagePlugin.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

let refreshInterval: number

onMounted(() => {
  loadSandbox()
  loadLogs()
  refreshInterval = window.setInterval(() => {
    loadSandbox()
    loadLogs()
  }, 5000)
})

onUnmounted(() => {
  clearInterval(refreshInterval)
})
</script>

<style scoped>
.sandbox-detail {
  padding: 24px;
}

.logs-card,
.exec-card,
.file-card {
  margin-top: 16px;
}

.log-section {
  border: 1px solid var(--td-component-border);
  border-radius: 4px;
}

.log-title {
  padding: 12px 16px;
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-border);
  font-weight: 600;
  font-size: 13px;
  border-radius: 8px 8px 0 0;
  color: var(--td-text-color-primary);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.title-text {
  font-size: 18px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.log-content {
  margin: 0;
  padding: 16px;
  min-height: 200px;
  max-height: 400px;
  overflow: auto;
  font-family: 'Fira Code', 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  background: #1e1e1e; /* Dark theme for logs */
  color: #d4d4d4;
  border-radius: 0 0 8px 8px;
}

.event-list {
  padding: 12px;
  min-height: 200px;
  max-height: 400px;
  overflow: auto;
  background: var(--td-bg-color-container);
  border-radius: 0 0 8px 8px;
}

.event-item {
  padding: 4px 0;
  font-size: 12px;
  font-family: monospace;
  border-bottom: 1px dashed var(--td-component-border);
}

.event-item:last-child {
  border-bottom: none;
}

.no-events {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.output-area {
  margin-top: 16px;
  border: 1px solid var(--td-component-border);
  border-radius: 4px;
}

.output-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 16px;
  background: #0f172a; /* Dark header */
  color: #e2e8f0;
  border-radius: 8px 8px 0 0;
  border-bottom: 1px solid #334155;
  font-family: 'Fira Code', monospace;
  font-size: 12px;
}

.output-content {
  margin: 0;
  padding: 16px;
  min-height: 200px;
  max-height: 400px;
  overflow: auto;
  font-family: 'Fira Code', 'JetBrains Mono', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  background: #0f172a; /* Terminal background */
  color: #4ade80; /* Matrix Green */
  border-radius: 0 0 8px 8px;
}
</style>
