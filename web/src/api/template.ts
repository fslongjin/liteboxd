import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  timeout: 30000,
})

// Template types
export interface TemplateSpec {
  image: string
  resources: {
    cpu: string
    memory: string
  }
  ttl: number
  env?: Record<string, string>
  startupScript?: string
  startupTimeout?: number
  files?: FileSpec[]
  readinessProbe?: ProbeSpec
  network?: NetworkSpec
}

export interface NetworkSpec {
  allowInternetAccess: boolean
  allowedDomains?: string[]
}

export interface FileSpec {
  source?: string
  destination: string
  content?: string
}

export interface ProbeSpec {
  exec: {
    command: string[]
  }
  initialDelaySeconds?: number
  periodSeconds?: number
  failureThreshold?: number
}

export interface Template {
  id: string
  name: string
  displayName: string
  description: string
  tags: string[]
  author: string
  isPublic: boolean
  latestVersion: number
  createdAt: string
  updatedAt: string
  spec?: TemplateSpec
}

export interface CreateTemplateRequest {
  name: string
  displayName?: string
  description?: string
  tags?: string[]
  isPublic?: boolean
  spec: TemplateSpec
  autoPrepull?: boolean
}

export interface UpdateTemplateRequest {
  displayName?: string
  description?: string
  tags?: string[]
  isPublic?: boolean
  spec: TemplateSpec
  changelog?: string
}

export interface TemplateVersion {
  id: string
  templateId: string
  version: number
  spec: TemplateSpec
  changelog: string
  createdBy: string
  createdAt: string
}

export interface PrepullTask {
  id: string
  image: string
  imageHash: string
  status: string
  desiredNodes: number
  readyNodes: number
  createdAt: string
  completedAt?: string
}

export interface ImportResult {
  name: string
  action: string // created, updated, skipped, failed
  version?: number
  error?: string
}

export const templateApi = {
  // Template CRUD
  list: (params?: { tag?: string; search?: string; page?: number; pageSize?: number }) =>
    api.get('/templates', { params }),

  get: (name: string) => api.get<Template>(`/templates/${name}`),

  create: (data: CreateTemplateRequest) => api.post<Template>('/templates', data),

  update: (name: string, data: UpdateTemplateRequest) =>
    api.put<Template>(`/templates/${name}`, data),

  delete: (name: string) => api.delete(`/templates/${name}`),

  // Versions
  listVersions: (name: string) =>
    api.get<{ items: TemplateVersion[] }>(`/templates/${name}/versions`),

  getVersion: (name: string, version: number) =>
    api.get<TemplateVersion>(`/templates/${name}/versions/${version}`),

  rollback: (name: string, data: { targetVersion: number; changelog?: string }) =>
    api.post(`/templates/${name}/rollback`, data),

  // Prepull
  listPrepulls: () => api.get<{ items: PrepullTask[] }>('/images/prepull'),

  createPrepull: (data: { image: string; templateName?: string }) =>
    api.post<PrepullTask>('/images/prepull', data),

  deletePrepull: (id: string) => api.delete(`/images/prepull/${id}`),

  // Import/Export
  exportAll: (params?: { tag?: string; names?: string }) =>
    api.get('/templates/export', {
      params,
      responseType: 'blob',
    }),

  exportOne: (name: string, version?: number) =>
    api.get(`/templates/${name}/export`, {
      params: version ? { version } : undefined,
      responseType: 'blob',
    }),

  import: (file: File, strategy?: string, prepull?: boolean) => {
    const formData = new FormData()
    formData.append('file', file)
    if (strategy) formData.append('strategy', strategy)
    if (prepull) formData.append('prepull', 'true')
    return api.post<{
      total: number
      created: number
      updated: number
      skipped: number
      failed: number
      results: ImportResult[]
      prepullStarted?: string[]
    }>('/templates/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}

export default api
