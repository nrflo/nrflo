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
    enabled: true,
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

  it('groups models by cli_type with prefixed labels sorted alphabetically', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'sonnet', cli_type: 'claude', display_name: 'Sonnet' }),
      makeCLIModel({ id: 'haiku', cli_type: 'claude', display_name: 'Haiku' }),
      makeCLIModel({ id: 'codex_gpt', cli_type: 'codex', display_name: 'GPT' }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current).toEqual([
      { label: 'Claude', options: [
        { value: 'haiku', label: 'Claude: Haiku' },
        { value: 'sonnet', label: 'Claude: Sonnet' },
      ]},
      { label: 'Codex', options: [
        { value: 'codex_gpt', label: 'Codex: GPT' },
      ]},
    ])
  })

  it('falls back to capitalized cli_type for unknown provider', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'custom1', cli_type: 'myprovider', display_name: 'CustomModel' }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(1))
    expect(result.current[0].label).toBe('Myprovider')
    expect(result.current[0].options[0].label).toBe('Myprovider: CustomModel')
  })

  it('excludes disabled models from returned options', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'sonnet', cli_type: 'claude', display_name: 'Sonnet', enabled: true }),
      makeCLIModel({ id: 'haiku', cli_type: 'claude', display_name: 'Haiku', enabled: false }),
      makeCLIModel({ id: 'opus', cli_type: 'claude', display_name: 'Opus', enabled: true }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(1))
    const labels = result.current[0].options.map((o) => o.label)
    expect(labels).toContain('Claude: Sonnet')
    expect(labels).toContain('Claude: Opus')
    expect(labels).not.toContain('Claude: Haiku')
  })

  it('sorts groups and options within groups alphabetically', async () => {
    vi.mocked(cliModelsApi.listCLIModels).mockResolvedValue([
      makeCLIModel({ id: 'oc1', cli_type: 'opencode', display_name: 'Z-model' }),
      makeCLIModel({ id: 'oc2', cli_type: 'opencode', display_name: 'A-model' }),
      makeCLIModel({ id: 'c1', cli_type: 'claude', display_name: 'Opus' }),
    ])
    const { result } = renderHook(() => useModelOptions(), {
      wrapper: createWrapper(createTestQueryClient()),
    })
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current[0].label).toBe('Claude')
    expect(result.current[1].label).toBe('OpenCode')
    expect(result.current[1].options[0].label).toBe('OpenCode: A-model')
    expect(result.current[1].options[1].label).toBe('OpenCode: Z-model')
  })
})
