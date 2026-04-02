import { describe, it, expect, vi, beforeEach } from 'vitest'

// localStorage is not available in threads-pool jsdom — mock it directly
const localStorageData: Record<string, string> = {}
const mockLocalStorage = {
  getItem: (key: string) => localStorageData[key] ?? null,
  setItem: (key: string, value: string) => { localStorageData[key] = value },
  removeItem: (key: string) => { delete localStorageData[key] },
}
Object.defineProperty(global, 'localStorage', {
  writable: true,
  configurable: true,
  value: mockLocalStorage,
})

// matchMedia is not implemented in jsdom — set up a configurable mock
let mockMatchMediaResult = { matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }

function setupMatchMedia(matches: boolean) {
  mockMatchMediaResult = {
    matches,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockReturnValue(mockMatchMediaResult),
  })
}

// Fresh store import after resetting modules + localStorage state
async function getStore(storedTheme?: string) {
  vi.resetModules()
  delete localStorageData['nrf_theme']
  if (storedTheme !== undefined) {
    localStorageData['nrf_theme'] = storedTheme
  }
  document.documentElement.classList.remove('dark')
  const { useThemeStore } = await import('@/stores/themeStore')
  return useThemeStore
}

describe('themeStore — setTheme', () => {
  beforeEach(() => {
    setupMatchMedia(false)
  })

  it('adds .dark class when set to dark', async () => {
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('removes .dark class when set to light', async () => {
    document.documentElement.classList.add('dark')
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('persists theme to localStorage under nrf_theme', async () => {
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('dark')
    expect(localStorageData['nrf_theme']).toBe('dark')
    useThemeStore.getState().setTheme('light')
    expect(localStorageData['nrf_theme']).toBe('light')
    useThemeStore.getState().setTheme('system')
    expect(localStorageData['nrf_theme']).toBe('system')
  })

  it('updates store state', async () => {
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('dark')
    expect(useThemeStore.getState().theme).toBe('dark')
  })

  it('system + matchMedia.matches=true → adds .dark', async () => {
    setupMatchMedia(true)
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('system')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('system + matchMedia.matches=false → removes .dark', async () => {
    document.documentElement.classList.add('dark')
    setupMatchMedia(false)
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('system')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('system mode registers matchMedia change listener', async () => {
    const useThemeStore = await getStore()
    useThemeStore.getState().setTheme('system')
    expect(mockMatchMediaResult.addEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })

  it('switching away from system calls removeEventListener to clean up listener', async () => {
    // Start from 'light' so no listener is registered during init
    const useThemeStore = await getStore('light')
    useThemeStore.getState().setTheme('system')
    const mq = mockMatchMediaResult
    expect(mq.addEventListener).toHaveBeenCalledTimes(1)
    // Switch away — should clean up
    useThemeStore.getState().setTheme('dark')
    expect(mq.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })
})

describe('themeStore — init from localStorage', () => {
  beforeEach(() => {
    setupMatchMedia(false)
  })

  it('applies .dark class on load when stored value is dark', async () => {
    const useThemeStore = await getStore('dark')
    expect(useThemeStore.getState().theme).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('keeps .dark absent on load when stored value is light', async () => {
    const useThemeStore = await getStore('light')
    expect(useThemeStore.getState().theme).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('defaults to system when no localStorage value', async () => {
    const useThemeStore = await getStore()
    expect(useThemeStore.getState().theme).toBe('system')
  })

  it('defaults to system when localStorage has invalid value', async () => {
    const useThemeStore = await getStore('invalid')
    expect(useThemeStore.getState().theme).toBe('system')
  })

  it('system init with matchMedia.matches=true applies .dark', async () => {
    setupMatchMedia(true)
    const useThemeStore = await getStore() // no stored value → system
    expect(useThemeStore.getState().theme).toBe('system')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('system init registers matchMedia listener', async () => {
    const useThemeStore = await getStore() // system mode
    expect(useThemeStore.getState().theme).toBe('system')
    expect(mockMatchMediaResult.addEventListener).toHaveBeenCalledWith('change', expect.any(Function))
  })
})
