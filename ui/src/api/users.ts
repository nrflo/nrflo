import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type { User, UserListResponse, CreateUserRequest, UpdateUserRequest, ResetPasswordRequest } from '@/types/user'

export async function listUsers(): Promise<UserListResponse> {
  return apiGet<UserListResponse>('/api/v1/users')
}

export async function createUser(data: CreateUserRequest): Promise<User> {
  return apiPost<User>('/api/v1/users', data)
}

export async function updateUser(id: string, data: UpdateUserRequest): Promise<User> {
  return apiPatch<User>(`/api/v1/users/${encodeURIComponent(id)}`, data)
}

export async function resetUserPassword(id: string, data: ResetPasswordRequest): Promise<void> {
  return apiPost<void>(`/api/v1/users/${encodeURIComponent(id)}/reset-password`, data)
}

export async function deleteUser(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/users/${encodeURIComponent(id)}`)
}
