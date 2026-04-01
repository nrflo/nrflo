import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useModelOptions } from './useCLIModels'
import * as cliModelsApi from '@/api/cliModels'
import { createTestQueryClient, createWrapper } from '@/test/utils'
import type { CLIModel } from '@/api/cliModels'

vi.mock('@/api/cliModels')

function makeCLIModel(overrides: Partial<CLIModel> = {}): CLIModel {
  return {
    id: 'sonnet',
    cli_type: 'claude',
    display_name: 'Sonnet',
    mapped_model: 'claude-sonnet-4-20250514',
    reasoning_effort: '',
    context_length: 200000,
    read_only: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('useModelOptions', () => {
  beforeEach(() => vi.clearAllMocks())

  it('returns empty array when no data', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toEqual([]))
  })

  it('maps models to {value: id, label: display_name} sorted by display_name', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'sonnet', display_name: 'Sonnet' }),
      makeCLIModel({ id: 'haiku', display_name: 'Haiku' }),
      makeCLIModel({ id: 'opus', display_name: 'Opus' }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(3))
    expect(result.current).toEqual([
      { value: 'haiku', label: 'Haiku' },
      { value: 'opus', label: 'Opus' },
      { value: 'sonnet', label: 'Sonnet' },
    ])
  })

  it('sorts case-insensitively by display_name', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'z-model', display_name: 'z-model' }),
      makeCLIModel({ id: 'a-model', display_name: 'A-model' }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current[0].value).toBe('a-model')
    expect(result.current[1].value).toBe('z-model')
  })
})
