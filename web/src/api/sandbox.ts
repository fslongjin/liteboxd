import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  timeout: 30000,
  withCredentials: true,
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
  desired_state?: string
  lifecycle_status?: string
  status_reason?: string
  pod_phase?: string
  pod_ip?: string
  last_seen_at?: string
  created_at: string
  expires_at: string
  updated_at?: string
  deleted_at?: string
  accessToken?: string
  accessUrl?: string
}

export interface SandboxMetadataListParams {
  id?: string
  template?: string
  desired_state?: string
  lifecycle_status?: string
  created_from?: string
  created_to?: string
  deleted_from?: string
  deleted_to?: string
  page?: number
  page_size?: number
}

export interface SandboxMetadataListResponse {
  items: Sandbox[]
  total: number
  page: number
  page_size: number
}

export interface SandboxStatusHistoryItem {
  id: number
  sandbox_id: string
  source: string
  from_status: string
  to_status: string
  reason: string
  payload_json: string
  created_at: string
}

export interface SandboxStatusHistoryResponse {
  items: SandboxStatusHistoryItem[]
}

export interface ReconcileRun {
  id: string
  trigger_type: string
  started_at: string
  finished_at?: string
  total_db: number
  total_k8s: number
  drift_count: number
  fixed_count: number
  status: string
  error?: string
}

export interface ReconcileItem {
  id: number
  run_id: string
  sandbox_id: string
  drift_type: string
  action: string
  detail: string
  created_at: string
}

export interface ReconcileRunListResponse {
  items: ReconcileRun[]
}

export interface ReconcileRunDetailResponse {
  run: ReconcileRun
  items: ReconcileItem[]
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

  listMetadata: (params?: SandboxMetadataListParams) =>
    api.get<SandboxMetadataListResponse>('/sandboxes/metadata', { params }),

  get: (id: string) => api.get<Sandbox>(`/sandboxes/${id}`),

  getStatusHistory: (id: string, params?: { limit?: number; before_id?: number }) =>
    api.get<SandboxStatusHistoryResponse>(`/sandboxes/${id}/status-history`, { params }),

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

  triggerReconcile: () => api.post<ReconcileRunDetailResponse>('/sandboxes/reconcile'),

  listReconcileRuns: (limit = 20) =>
    api.get<ReconcileRunListResponse>('/sandboxes/reconcile/runs', { params: { limit } }),

  getReconcileRun: (id: string) =>
    api.get<ReconcileRunDetailResponse>(`/sandboxes/reconcile/runs/${id}`),
}

export default api
