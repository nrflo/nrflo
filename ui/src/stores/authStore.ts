import { create } from 'zustand'
import * as authApi from '@/api/auth'
import type { User } from '@/types/user'

type AuthStatus = 'loading' | 'authed' | 'anon'

interface AuthState {
  user: User | null
  status: AuthStatus
  refresh: () => Promise<void>
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  clear: () => void
}

export const useAuthStore = create<AuthState>()((set) => ({
  user: null,
  status: 'loading',

  refresh: async () => {
    try {
      const user = await authApi.getMe()
      set({ user, status: 'authed' })
    } catch {
      set({ user: null, status: 'anon' })
    }
  },

  login: async (email: string, password: string) => {
    await authApi.login(email, password)
    const user = await authApi.getMe()
    set({ user, status: 'authed' })
  },

  logout: async () => {
    try {
      await authApi.logout()
    } catch {
      // ignore logout errors
    }
    set({ user: null, status: 'anon' })
  },

  clear: () => {
    set({ user: null, status: 'anon' })
  },
}))

export const useIsAdmin = () => useAuthStore((s) => s.user?.role === 'admin')
export const useIsAuthed = () => useAuthStore((s) => s.status === 'authed')
export const useMustChangePassword = () => useAuthStore((s) => !!s.user?.must_change_password)
