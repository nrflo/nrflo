import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getGlobalSettings, updateGlobalSettings } from './settings'
import * as client from './client'

vi.mock('./client')

describe('getGlobalSettings', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls GET /api/v1/settings', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ low_consumption_mode: false })
    const result = await getGlobalSettings()
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/settings')
    expect(result).toEqual({ low_consumption_mode: false })
  })

  it('returns true when mode is enabled', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ low_consumption_mode: true })
    expect((await getGlobalSettings()).low_consumption_mode).toBe(true)
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiGet).mockRejectedValue(new Error('Network error'))
    await expect(getGlobalSettings()).rejects.toThrow('Network error')
  })
})

describe('updateGlobalSettings', () => {
  beforeEach(() => vi.clearAllMocks())

  it('calls PATCH /api/v1/settings with body', async () => {
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
    await updateGlobalSettings({ low_consumption_mode: true })
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/settings', { low_consumption_mode: true })
  })

  it('sends false to disable mode', async () => {
    vi.mocked(client.apiPatch).mockResolvedValue({ status: 'ok' })
    await updateGlobalSettings({ low_consumption_mode: false })
    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/settings', { low_consumption_mode: false })
  })

  it('propagates errors', async () => {
    vi.mocked(client.apiPatch).mockRejectedValue(new Error('Server error'))
    await expect(updateGlobalSettings({ low_consumption_mode: true })).rejects.toThrow('Server error')
  })
})
