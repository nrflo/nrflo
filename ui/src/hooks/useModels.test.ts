import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import * as api from '@/api/models'
import { createTestQueryClient, createWrapper } from '@/test/utils'
import { useModelOptions } from './useModels'
import type { Model } from '@/api/models'

vi.mock('@/api/models')

function model(overrides: Partial<Model>): Model {
  return {
    id: 'sonnet-5', provider: 'anthropic', display_name: 'Sonnet', cli_model: 'sonnet', api_model: 'sonnet',
    cli_efforts: [], api_efforts: [], cli_context: 200000, api_context: 200000, fallback_models: '',
    default_effort: '', read_only: true, enabled: true, created_at: '', updated_at: '', ...overrides,
  }
}

describe('useModelOptions', () => {
  it('filters disabled rows and rows that do not support the requested mode', async () => {
    vi.mocked(api.listModels).mockResolvedValue([
      model({ id: 'both', display_name: 'Both' }),
      model({ id: 'cli-only', display_name: 'CLI only', api_model: '' }),
      model({ id: 'api-only', display_name: 'API only', cli_model: '' }),
      model({ id: 'disabled', display_name: 'Disabled', enabled: false }),
    ])
    const wrapper = createWrapper(createTestQueryClient())
    const { result } = renderHook(() => ({ cli: useModelOptions('cli'), api: useModelOptions('api') }), { wrapper })
    await waitFor(() => expect(result.current.cli).toHaveLength(1))
    expect(result.current.cli[0].options.map((option) => option.value)).toEqual(['both', 'cli-only'])
    expect(result.current.api[0].options.map((option) => option.value)).toEqual(['api-only', 'both'])
  })

  it('groups and sorts options by provider', async () => {
    vi.mocked(api.listModels).mockResolvedValue([
      model({ id: 'z', provider: 'openai', display_name: 'Zulu' }),
      model({ id: 'a', display_name: 'Alpha' }),
    ])
    const { result } = renderHook(() => useModelOptions('cli'), { wrapper: createWrapper(createTestQueryClient()) })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current.map((group) => group.label)).toEqual(['Anthropic', 'OpenAI'])
  })

  it('groups an enabled openrouter api row under the OpenRouter label', async () => {
    vi.mocked(api.listModels).mockResolvedValue([
      model({ id: 'kimi', provider: 'openrouter', display_name: 'Kimi', cli_model: '', api_model: 'moonshotai/kimi-k3' }),
    ])
    const { result } = renderHook(() => useModelOptions('api'), { wrapper: createWrapper(createTestQueryClient()) })
    await waitFor(() => expect(result.current).toHaveLength(1))
    expect(result.current[0].label).toBe('OpenRouter')
    expect(result.current[0].options).toEqual([{ value: 'kimi', label: 'OpenRouter: Kimi' }])
  })

  it('falls back to the raw provider name for a custom provider with no PROVIDER_LABELS entry', async () => {
    vi.mocked(api.listModels).mockResolvedValue([
      model({ id: 'm1', provider: 'acme', display_name: 'Acme Model', cli_model: '', api_model: 'acme-model-1' }),
    ])
    const { result } = renderHook(() => useModelOptions('api'), { wrapper: createWrapper(createTestQueryClient()) })
    await waitFor(() => expect(result.current).toHaveLength(1))
    expect(result.current[0].label).toBe('acme')
    expect(result.current[0].options).toEqual([{ value: 'm1', label: 'acme: Acme Model' }])
    expect(result.current[0].label).not.toMatch(/^undefined/)
  })
})
