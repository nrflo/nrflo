import { apiFetch } from './client'
import type { User } from '@/types/user'

export async function login(email: string, password: string): Promise<void> {
  await apiFetch<unknown>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function logout(): Promise<void> {
  await apiFetch<void>('/api/v1/auth/logout', { method: 'POST' })
}

export async function getMe(): Promise<User> {
  const res = await apiFetch<{ user: User }>('/api/v1/auth/me', { method: 'GET' })
  return res.user
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await apiFetch<void>('/api/v1/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
}
