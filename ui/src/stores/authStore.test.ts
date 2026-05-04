import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useAuthStore, useIsAdmin, useIsAuthed, useMustChangePassword } from './authStore'
import * as authApi from '@/api/auth'
import type { User } from '@/types/user'

vi.mock('@/api/auth')

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'u1',
    email: 'user@example.com',
    display_name: 'User',
    role: 'admin',
    status: 'active',
    must_change_password: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('authStore', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'loading' })
    vi.clearAllMocks()
  })

  describe('refresh()', () => {
    it('sets status=authed and user on success', async () => {
      const user = makeUser()
      vi.mocked(authApi.getMe).mockResolvedValue(user)

      await useAuthStore.getState().refresh()

      expect(useAuthStore.getState().status).toBe('authed')
      expect(useAuthStore.getState().user).toEqual(user)
    })

    it('sets status=anon and user=null on failure', async () => {
      vi.mocked(authApi.getMe).mockRejectedValue(new Error('unauthorized'))

      await useAuthStore.getState().refresh()

      expect(useAuthStore.getState().status).toBe('anon')
      expect(useAuthStore.getState().user).toBeNull()
    })
  })

  describe('login()', () => {
    it('calls authApi.login then getMe, sets status=authed', async () => {
      const user = makeUser()
      vi.mocked(authApi.login).mockResolvedValue(undefined)
      vi.mocked(authApi.getMe).mockResolvedValue(user)

      await useAuthStore.getState().login('user@example.com', 'secret')

      expect(authApi.login).toHaveBeenCalledWith('user@example.com', 'secret')
      expect(authApi.getMe).toHaveBeenCalled()
      expect(useAuthStore.getState().status).toBe('authed')
      expect(useAuthStore.getState().user).toEqual(user)
    })

    it('propagates errors from authApi.login without changing state', async () => {
      vi.mocked(authApi.login).mockRejectedValue(new Error('invalid credentials'))

      await expect(useAuthStore.getState().login('a@b.com', 'bad')).rejects.toThrow(
        'invalid credentials'
      )
      expect(useAuthStore.getState().status).toBe('loading')
    })
  })

  describe('logout()', () => {
    it('calls authApi.logout and sets anon state', async () => {
      useAuthStore.setState({ user: makeUser(), status: 'authed' })
      vi.mocked(authApi.logout).mockResolvedValue(undefined)

      await useAuthStore.getState().logout()

      expect(authApi.logout).toHaveBeenCalled()
      expect(useAuthStore.getState().status).toBe('anon')
      expect(useAuthStore.getState().user).toBeNull()
    })

    it('sets anon state even when authApi.logout throws', async () => {
      useAuthStore.setState({ user: makeUser(), status: 'authed' })
      vi.mocked(authApi.logout).mockRejectedValue(new Error('network'))

      await useAuthStore.getState().logout()

      expect(useAuthStore.getState().status).toBe('anon')
      expect(useAuthStore.getState().user).toBeNull()
    })
  })

  describe('clear()', () => {
    it('sets user=null and status=anon', () => {
      useAuthStore.setState({ user: makeUser(), status: 'authed' })

      useAuthStore.getState().clear()

      expect(useAuthStore.getState().status).toBe('anon')
      expect(useAuthStore.getState().user).toBeNull()
    })
  })

  describe('selector hooks', () => {
    it('useIsAdmin returns true for admin user', () => {
      useAuthStore.setState({ user: makeUser({ role: 'admin' }), status: 'authed' })
      const { result } = renderHook(() => useIsAdmin())
      expect(result.current).toBe(true)
    })

    it('useIsAdmin returns false for viewer user', () => {
      useAuthStore.setState({ user: makeUser({ role: 'viewer' }), status: 'authed' })
      const { result } = renderHook(() => useIsAdmin())
      expect(result.current).toBe(false)
    })

    it('useIsAdmin returns false when user is null', () => {
      useAuthStore.setState({ user: null, status: 'anon' })
      const { result } = renderHook(() => useIsAdmin())
      expect(result.current).toBe(false)
    })

    it('useIsAuthed returns true when status=authed', () => {
      useAuthStore.setState({ user: makeUser(), status: 'authed' })
      const { result } = renderHook(() => useIsAuthed())
      expect(result.current).toBe(true)
    })

    it('useIsAuthed returns false when status=anon', () => {
      useAuthStore.setState({ user: null, status: 'anon' })
      const { result } = renderHook(() => useIsAuthed())
      expect(result.current).toBe(false)
    })

    it('useIsAuthed returns false when status=loading', () => {
      useAuthStore.setState({ user: null, status: 'loading' })
      const { result } = renderHook(() => useIsAuthed())
      expect(result.current).toBe(false)
    })

    it('useMustChangePassword returns true when must_change_password=true', () => {
      useAuthStore.setState({ user: makeUser({ must_change_password: true }), status: 'authed' })
      const { result } = renderHook(() => useMustChangePassword())
      expect(result.current).toBe(true)
    })

    it('useMustChangePassword returns false when must_change_password=false', () => {
      useAuthStore.setState({ user: makeUser({ must_change_password: false }), status: 'authed' })
      const { result } = renderHook(() => useMustChangePassword())
      expect(result.current).toBe(false)
    })

    it('useMustChangePassword returns false when user is null', () => {
      useAuthStore.setState({ user: null, status: 'anon' })
      const { result } = renderHook(() => useMustChangePassword())
      expect(result.current).toBe(false)
    })
  })
})
