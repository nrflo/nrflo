import { describe, it, expect, vi, beforeEach } from 'vitest'
import { login, logout, getMe, changePassword } from './auth'
import * as client from './client'

vi.mock('./client')

const mockUser = {
  id: 'u1',
  email: 'user@example.com',
  display_name: 'User',
  role: 'admin' as const,
  status: 'active' as const,
  must_change_password: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

describe('auth API', () => {
  beforeEach(() => vi.clearAllMocks())

  describe('login', () => {
    it('POSTs to /api/v1/auth/login with email and password', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue(undefined)
      await login('user@example.com', 'secret')
      expect(client.apiFetch).toHaveBeenCalledWith('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email: 'user@example.com', password: 'secret' }),
      })
    })

    it('propagates errors', async () => {
      vi.mocked(client.apiFetch).mockRejectedValue(new Error('network'))
      await expect(login('a@b.com', 'pw')).rejects.toThrow('network')
    })
  })

  describe('logout', () => {
    it('POSTs to /api/v1/auth/logout', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue(undefined)
      await logout()
      expect(client.apiFetch).toHaveBeenCalledWith('/api/v1/auth/logout', { method: 'POST' })
    })
  })

  describe('getMe', () => {
    it('GETs /api/v1/auth/me and returns the user', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue({ user: mockUser })
      const result = await getMe()
      expect(client.apiFetch).toHaveBeenCalledWith('/api/v1/auth/me', { method: 'GET' })
      expect(result).toEqual(mockUser)
    })

    it('propagates errors', async () => {
      vi.mocked(client.apiFetch).mockRejectedValue(new Error('unauthorized'))
      await expect(getMe()).rejects.toThrow('unauthorized')
    })
  })

  describe('changePassword', () => {
    it('POSTs to /api/v1/auth/change-password with current and new password', async () => {
      vi.mocked(client.apiFetch).mockResolvedValue(undefined)
      await changePassword('oldpass', 'newpass')
      expect(client.apiFetch).toHaveBeenCalledWith('/api/v1/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ current_password: 'oldpass', new_password: 'newpass' }),
      })
    })

    it('propagates errors', async () => {
      vi.mocked(client.apiFetch).mockRejectedValue(new Error('wrong password'))
      await expect(changePassword('bad', 'new')).rejects.toThrow('wrong password')
    })
  })
})
