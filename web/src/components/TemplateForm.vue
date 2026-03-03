<template>
  <div class="template-form-scroll">
    <t-form :data="form" :rules="formRules" ref="formRef" label-width="100px">
      <t-form-item label="名称" name="name">
        <t-input v-model="form.name" placeholder="英文名称，如: python-dev" :disabled="isEdit" />
      </t-form-item>
      <t-form-item label="显示名称" name="displayName">
        <t-input v-model="form.displayName" placeholder="如: Python 开发环境" />
      </t-form-item>
      <t-form-item label="描述" name="description">
        <t-textarea v-model="form.description" placeholder="模板描述" :maxlength="200" />
      </t-form-item>
      <t-form-item label="标签" name="tags">
        <t-tag-input v-model="form.tags" placeholder="按回车添加标签" clearable />
      </t-form-item>
      <t-form-item label="镜像" name="spec.image">
        <t-input v-model="form.spec.image" placeholder="如: python:3.11-slim" />
      </t-form-item>
      <t-divider>容器入口 (Command / Args)</t-divider>
      <t-form-item label="Command">
        <t-tag-input
          v-model="form.spec.command"
          placeholder="留空使用镜像默认；每项回车添加"
          clearable
        />
        <t-tooltip content="覆盖容器入口命令；不填则使用镜像 OCI CMD">
          <t-icon
            name="help-circle"
            style="margin-left: 8px; color: var(--td-text-color-placeholder)"
          />
        </t-tooltip>
      </t-form-item>
      <t-form-item label="Args">
        <t-tag-input
          v-model="form.spec.args"
          placeholder="留空使用镜像默认；每项回车添加"
          clearable
        />
        <t-tooltip content="覆盖容器参数；不填则使用镜像默认">
          <t-icon
            name="help-circle"
            style="margin-left: 8px; color: var(--td-text-color-placeholder)"
          />
        </t-tooltip>
      </t-form-item>
      <t-divider>资源配置</t-divider>
      <t-form-item label="CPU">
        <t-input v-model="form.spec.resources.cpu" placeholder="如: 500m" />
      </t-form-item>
      <t-form-item label="内存">
        <t-input v-model="form.spec.resources.memory" placeholder="如: 512Mi" />
      </t-form-item>
      <t-form-item label="TTL (秒)">
        <t-input-number v-model="form.spec.ttl" :min="0" :max="86400" />
      </t-form-item>
      <t-divider>持久化配置</t-divider>
      <t-form-item label="启用持久化">
        <t-switch v-model="persistenceEnabled" />
        <t-tooltip content="启用后将使用 PVC 持久化 rootfs（适用于长期运行沙箱）">
          <t-icon
            name="help-circle"
            style="margin-left: 8px; color: var(--td-text-color-placeholder)"
          />
        </t-tooltip>
      </t-form-item>
      <template v-if="persistenceEnabled">
        <t-form-item label="模式">
          <t-select
            v-model="persistenceMode"
            :options="[{ label: 'rootfs-overlay', value: 'rootfs-overlay' }]"
          />
        </t-form-item>
        <t-form-item label="磁盘大小">
          <t-input v-model="persistenceSize" placeholder="如: 1Gi" />
        </t-form-item>
        <t-form-item label="StorageClass">
          <t-input v-model="persistenceStorageClassName" placeholder="如: longhorn" />
        </t-form-item>
        <t-form-item label="回收策略">
          <t-select
            v-model="persistenceReclaimPolicy"
            :options="[
              { label: 'Delete（删除模板沙箱时删除PVC）', value: 'Delete' },
              { label: 'Retain（保留PVC）', value: 'Retain' },
            ]"
          />
        </t-form-item>
      </template>
      <t-form-item label="环境变量">
        <t-textarea
          v-model="envText"
          placeholder="KEY=value&#10;KEY2=value2"
          :autosize="{ minRows: 2, maxRows: 4 }"
        />
      </t-form-item>
      <t-form-item label="启动脚本">
        <t-textarea
          v-model="form.spec.startupScript"
          placeholder="容器启动后执行的 Shell 脚本"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </t-form-item>
      <t-form-item label="启动超时(秒)">
        <t-input-number v-model="form.spec.startupTimeout" :min="30" :max="600" />
      </t-form-item>
      <t-form-item label="自动预拉取">
        <t-switch v-model="form.autoPrepull" />
      </t-form-item>
      <t-form-item label="允许公网访问">
        <t-switch v-model="networkAllowInternet" />
        <t-tooltip content="开启后，允许沙箱出站访问公网（80/443 端口）">
          <t-icon
            name="help-circle"
            style="margin-left: 8px; color: var(--td-text-color-placeholder)"
          />
        </t-tooltip>
      </t-form-item>
      <t-form-item>
        <template #label>
          <span>域名白名单</span>
          <t-tooltip content="仅允许访问白名单域名（需要开启公网访问才能生效）">
            <t-icon
              name="help-circle"
              style="margin-left: 8px; color: var(--td-text-color-placeholder)"
            />
          </t-tooltip>
        </template>
        <div class="domain-whitelist-field">
          <t-tag-input
            v-model="networkAllowedDomains"
            placeholder="如: example.com 或 *.example.com"
            clearable
            style="margin-bottom: 0"
          />
          <p
            v-if="!networkAllowInternet && networkAllowedDomains.length > 0"
            class="domain-whitelist-hint"
          >
            公网访问已关闭，暂不生效
          </p>
        </div>
      </t-form-item>
    </t-form>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, type PropType } from 'vue'
import type { CreateTemplateRequest } from '../api/template'

const props = defineProps({
  isEdit: {
    type: Boolean,
    default: false,
  },
  initialData: {
    type: Object as PropType<(CreateTemplateRequest & { autoPrepull?: boolean }) | null>,
    default: () => null,
  },
})

const formRef = ref()
const form = ref<CreateTemplateRequest & { autoPrepull?: boolean }>({
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
})

const formRules = {
  name: [
    { required: true, message: '请输入模板名称' },
    { pattern: /^[a-z0-9-]+$/, message: '只能包含小写字母、数字和连字符' },
  ],
  'spec.image': [{ required: true, message: '请输入镜像' }],
}

const envText = computed({
  get: () => {
    const env = form.value.spec.env || {}
    return Object.entries(env)
      .map(([k, v]) => `${k}=${v}`)
      .join('\n')
  },
  set: (val: string) => {
    const env: Record<string, string> = {}
    val.split('\n').forEach((line) => {
      const [k, ...vParts] = line.split('=')
      if (k && vParts.length > 0) {
        env[k] = vParts.join('=')
      }
    })
    form.value.spec.env = env
  },
})

const networkAllowInternet = computed({
  get: () => form.value.spec.network?.allowInternetAccess ?? false,
  set: (v: boolean) => {
    if (!form.value.spec.network) {
      form.value.spec.network = { allowInternetAccess: false, allowedDomains: [] }
    }
    form.value.spec.network.allowInternetAccess = v
  },
})

const networkAllowedDomains = computed({
  get: () => form.value.spec.network?.allowedDomains ?? [],
  set: (v: string[]) => {
    if (!form.value.spec.network) {
      form.value.spec.network = { allowInternetAccess: false, allowedDomains: [] }
    }
    form.value.spec.network.allowedDomains = v
  },
})

const persistenceEnabled = computed({
  get: () => form.value.spec.persistence?.enabled ?? false,
  set: (v: boolean) => {
    if (!v) {
      form.value.spec.persistence = undefined
      return
    }
    if (!form.value.spec.persistence) {
      form.value.spec.persistence = {
        enabled: true,
        mode: 'rootfs-overlay',
        size: '1Gi',
        storageClassName: 'longhorn',
        reclaimPolicy: 'Delete',
      }
      return
    }
    form.value.spec.persistence.enabled = true
  },
})

const persistenceMode = computed({
  get: () => form.value.spec.persistence?.mode || 'rootfs-overlay',
  set: (v: string) => {
    if (!form.value.spec.persistence) return
    form.value.spec.persistence.mode = v
  },
})

const persistenceSize = computed({
  get: () => form.value.spec.persistence?.size || '',
  set: (v: string) => {
    if (!form.value.spec.persistence) return
    form.value.spec.persistence.size = v
  },
})

const persistenceStorageClassName = computed({
  get: () => form.value.spec.persistence?.storageClassName || '',
  set: (v: string) => {
    if (!form.value.spec.persistence) return
    form.value.spec.persistence.storageClassName = v
  },
})

const persistenceReclaimPolicy = computed({
  get: () => form.value.spec.persistence?.reclaimPolicy || 'Delete',
  set: (v: string) => {
    if (!form.value.spec.persistence) return
    form.value.spec.persistence.reclaimPolicy = v
  },
})

watch(
  () => props.initialData,
  (val) => {
    if (val) {
      // Deep copy to avoid mutating prop directly
      form.value = JSON.parse(JSON.stringify(val))
    }
  },
  { immediate: true, deep: true }
)

const validate = () => {
  return formRef.value?.validate()
}

const getData = () => {
  return form.value
}

defineExpose({
  validate,
  getData,
})
</script>

<style scoped>
.template-form-scroll {
  max-height: 70vh;
  overflow-y: auto;
  padding-right: 4px;
}

/* 域名白名单：输入框与下方提示纵向排列，提示紧贴输入框 */
.domain-whitelist-field {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  width: 100%;
  gap: 2px;
}

/* 直接由容器 gap 控制间距，避免与组件默认 margin 叠加 */
.domain-whitelist-hint {
  margin: 0;
  padding: 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--td-warning-color);
}
</style>
