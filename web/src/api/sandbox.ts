import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  timeout: 30000,
})

export interface Sandbox {
  id: string
  image: string
  cpu: string
  memory: string
  ttl: number
  status: string
  template?: string
  templateVersion?: number
  created_at: string
  expires_at: string
  accessToken?: string
  accessUrl?: string
}

export interface CreateSandboxRequest {
  template: string // required
  templateVersion?: number
  overrides?: {
    cpu?: string
    memory?: string
    ttl?: number
    env?: Record<string, string>
  }
}

export interface ExecRequest {
  command: string[]
  timeout?: number
}

export interface ExecResponse {
  exit_code: number
  stdout: string
  stderr: string
}

export interface LogsResponse {
  logs: string
  events: string[]
}

export const sandboxApi = {
  list: () => api.get<{ items: Sandbox[] }>('/sandboxes'),

  get: (id: string) => api.get<Sandbox>(`/sandboxes/${id}`),

  create: (data: CreateSandboxRequest) => api.post<Sandbox>('/sandboxes', data),

  delete: (id: string) => api.delete(`/sandboxes/${id}`),

  exec: (id: string, data: ExecRequest) => api.post<ExecResponse>(`/sandboxes/${id}/exec`, data),

  getLogs: (id: string) => api.get<LogsResponse>(`/sandboxes/${id}/logs`),

  uploadFile: (id: string, path: string, file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('path', path)
    return api.post(`/sandboxes/${id}/files`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  downloadFile: (id: string, path: string) =>
    api.get(`/sandboxes/${id}/files`, {
      params: { path },
      responseType: 'blob',
    }),
}

export default api
