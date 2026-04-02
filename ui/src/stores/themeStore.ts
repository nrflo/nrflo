import { create } from 'zustand'

type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'nrf_theme'

function getEffectiveDark(theme: Theme): boolean {
  if (theme === 'dark') return true
  if (theme === 'light') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function applyClass(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark)
}

interface ThemeState {
  theme: Theme
  setTheme: (theme: Theme) => void
}

let mediaListener: (() => void) | null = null

export const useThemeStore = create<ThemeState>()((set, get) => {
  // Init from localStorage
  const stored = localStorage.getItem(STORAGE_KEY) as Theme | null
  const initial: Theme = stored === 'light' || stored === 'dark' ? stored : 'system'

  // Apply on load
  applyClass(getEffectiveDark(initial))

  // If system mode, register matchMedia listener
  if (initial === 'system') {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => {
      if (get().theme === 'system') {
        applyClass(mq.matches)
      }
    }
    mq.addEventListener('change', handler)
    mediaListener = () => mq.removeEventListener('change', handler)
  }

  return {
    theme: initial,
    setTheme: (theme: Theme) => {
      // Clean up previous matchMedia listener
      if (mediaListener) {
        mediaListener()
        mediaListener = null
      }

      localStorage.setItem(STORAGE_KEY, theme)
      applyClass(getEffectiveDark(theme))

      // Register new listener for system mode
      if (theme === 'system') {
        const mq = window.matchMedia('(prefers-color-scheme: dark)')
        const handler = () => {
          if (get().theme === 'system') {
            applyClass(mq.matches)
          }
        }
        mq.addEventListener('change', handler)
        mediaListener = () => mq.removeEventListener('change', handler)
      }

      set({ theme })
    },
  }
})
