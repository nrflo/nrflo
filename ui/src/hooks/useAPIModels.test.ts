import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useAPIModelOptions } from './useAPIModels'
import * as apiModelsApi from '@/api/apiModels'
import { createTestQueryClient, createWrapper } from '@/test/utils'
import type { APIModel } from '@/api/apiModels'

vi.mock('@/api/apiModels')

function makeAPIModel(overrides: Partial<APIModel> = {}): APIModel {
  return {
    id: 'claude-opus',
    provider: 'anthropic',
    display_name: 'Claude Opus',
    mapped_model: 'claude-opus-4-7-20250514',
    reasoning_effort: '',
    context_length: 200000,
    read_only: true,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('useAPIModelOptions', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns empty array when no data', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toEqual([]))
  })

  it('groups enabled models by provider with Anthropic/OpenAI labels', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([
      makeAPIModel({ id: 'claude-opus', provider: 'anthropic', display_name: 'Opus' }),
      makeAPIModel({ id: 'gpt-4o', provider: 'openai', display_name: 'GPT-4o' }),
    ])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current[0].label).toBe('Anthropic')
    expect(result.current[1].label).toBe('OpenAI')
    expect(result.current[0].options[0]).toEqual({ value: 'claude-opus', label: 'Anthropic: Opus' })
    expect(result.current[1].options[0]).toEqual({ value: 'gpt-4o', label: 'OpenAI: GPT-4o' })
  })

  it('excludes disabled models', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([
      makeAPIModel({ id: 'enabled-model', provider: 'anthropic', display_name: 'Enabled', enabled: true }),
      makeAPIModel({ id: 'disabled-model', provider: 'anthropic', display_name: 'Disabled', enabled: false }),
    ])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(1))
    const labels = result.current[0].options.map(o => o.label)
    expect(labels).toContain('Anthropic: Enabled')
    expect(labels).not.toContain('Anthropic: Disabled')
  })

  it('returns empty array when all models are disabled', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([
      makeAPIModel({ id: 'm1', enabled: false }),
      makeAPIModel({ id: 'm2', enabled: false }),
    ])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toEqual([]))
  })

  it('sorts groups alphabetically (Anthropic before OpenAI)', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([
      makeAPIModel({ id: 'gpt-4o', provider: 'openai', display_name: 'GPT-4o' }),
      makeAPIModel({ id: 'claude-opus', provider: 'anthropic', display_name: 'Opus' }),
    ])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current[0].label).toBe('Anthropic')
    expect(result.current[1].label).toBe('OpenAI')
  })

  it('sorts options within a group alphabetically', async () => {
    vi.mocked(apiModelsApi.listAPIModels).mockResolvedValue([
      makeAPIModel({ id: 'b-model', provider: 'anthropic', display_name: 'Beta' }),
      makeAPIModel({ id: 'a-model', provider: 'anthropic', display_name: 'Alpha' }),
    ])
    const { result } = renderHook(() => useAPIModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(1))
    expect(result.current[0].options[0].label).toBe('Anthropic: Alpha')
    expect(result.current[0].options[1].label).toBe('Anthropic: Beta')
  })
})
