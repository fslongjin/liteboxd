import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
  timeout: 30000,
  withCredentials: true,
})

export interface LoginRequest {
  username: string
  password: string
}

export interface MeResponse {
  auth_method: string
  username?: string
}

export interface APIKey {
  id: string
  name: string
  key?: string
  prefix: string
  expires_at: string | null
  last_used_at: string | null
  created_at: string
}

export interface CreateAPIKeyRequest {
  name: string
  expires_in_days?: number
}

export const authApi = {
  login: (data: LoginRequest) =>
    api.post<{ message: string; username: string }>('/auth/login', data),

  logout: () => api.post('/auth/logout'),

  me: () => api.get<MeResponse>('/auth/me'),

  listAPIKeys: () => api.get<APIKey[]>('/auth/api-keys'),

  createAPIKey: (data: CreateAPIKeyRequest) => api.post<APIKey>('/auth/api-keys', data),

  deleteAPIKey: (id: string) => api.delete(`/auth/api-keys/${id}`),
}

export default api
