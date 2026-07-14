import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { NewChatForm } from './NewChatForm'
import * as useGlobalSettings from '@/hooks/useGlobalSettings'
import * as useAPIModelsHook from '@/hooks/useAPIModels'
import * as useCLIModelsHook from '@/hooks/useCLIModels'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import type { CLIModel } from '@/api/cliModels'
import type { APIModel } from '@/api/apiModels'

vi.mock('@/hooks/useGlobalSettings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useGlobalSettings')>()
  return { ...actual, useAPIModeEnabled: vi.fn() }
})

vi.mock('@/hooks/useAPIModels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useAPIModels')>()
  return { ...actual, useAPIModels: vi.fn() }
})

vi.mock('@/hooks/useCLIModels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useCLIModels')>()
  return { ...actual, useCLIModels: vi.fn() }
})

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return { ...actual, useCreateConsoleChat: vi.fn() }
})

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: Object.assign(
    (selector: (s: { currentProject: string; projects: unknown[] }) => unknown) =>
      selector({ currentProject: '', projects: [] }),
    { getState: () => ({ setCurrentProject: vi.fn() }) },
  ),
}))

function makeCLIModel(overrides: Partial<CLIModel> = {}): CLIModel {
  return {
    id: 'sonnet',
    cli_type: 'claude',
    display_name: 'Sonnet (CLI)',
    mapped_model: 'claude-sonnet-5',
    reasoning_effort: '',
    fallback_models: '',
    context_length: 200000,
    read_only: false,
    enabled: true,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function makeAPIModel(overrides: Partial<APIModel> = {}): APIModel {
  return {
    id: 'sonnet',
    provider: 'anthropic',
    display_name: 'Sonnet (API)',
    mapped_model: 'claude-sonnet-5',
    reasoning_effort: '',
    context_length: 200000,
    read_only: false,
    enabled: true,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function setup({
  apiModeEnabled = false,
  cliModels = [makeCLIModel()],
  apiModels = [makeAPIModel()],
  mutateAsync = vi.fn().mockResolvedValue({ session_id: 'sid-x' }),
}: {
  apiModeEnabled?: boolean
  cliModels?: CLIModel[]
  apiModels?: APIModel[]
  mutateAsync?: ReturnType<typeof vi.fn>
} = {}) {
  vi.mocked(useGlobalSettings.useAPIModeEnabled).mockReturnValue(apiModeEnabled)
  vi.mocked(useCLIModelsHook.useCLIModels).mockReturnValue({ data: cliModels } as ReturnType<
    typeof useCLIModelsHook.useCLIModels
  >)
  vi.mocked(useAPIModelsHook.useAPIModels).mockReturnValue({ data: apiModels } as ReturnType<
    typeof useAPIModelsHook.useAPIModels
  >)
  vi.mocked(useConsoleChats.useCreateConsoleChat).mockReturnValue({
    mutateAsync,
    isPending: false,
  } as unknown as ReturnType<typeof useConsoleChats.useCreateConsoleChat>)
  return { mutateAsync }
}

async function openEngineDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Claude' }))
}

async function openModelDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: /Select a model…/ }))
}

describe('NewChatForm engine selection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('hides the API (direct) engine option when api_mode_enabled is false', async () => {
    setup({ apiModeEnabled: false })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openEngineDropdown(user)

    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.queryByText('API (direct)')).not.toBeInTheDocument()
  })

  it('shows the API (direct) engine option when api_mode_enabled is true', async () => {
    setup({ apiModeEnabled: true })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openEngineDropdown(user)

    expect(screen.getByText('API (direct)')).toBeInTheDocument()
  })

  it('lists only enabled api_models rows for the api engine, never cli_models rows', async () => {
    setup({
      apiModeEnabled: true,
      cliModels: [makeCLIModel({ id: 'sonnet', display_name: 'Sonnet (CLI)' })],
      apiModels: [
        makeAPIModel({ id: 'sonnet', display_name: 'Sonnet (API)', enabled: true }),
        makeAPIModel({ id: 'disabled-model', display_name: 'Disabled Model', enabled: false }),
      ],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openEngineDropdown(user)
    await user.click(screen.getByText('API (direct)'))
    await openModelDropdown(user)

    expect(await screen.findByText('Sonnet (API)')).toBeInTheDocument()
    expect(screen.queryByText('Sonnet (CLI)')).not.toBeInTheDocument()
    expect(screen.queryByText('Disabled Model')).not.toBeInTheDocument()
  })

  it('shows the no-tools note only for the api engine', async () => {
    setup({ apiModeEnabled: true })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    expect(screen.queryByText(/No file\/edit\/bash tools/)).not.toBeInTheDocument()

    await openEngineDropdown(user)
    await user.click(screen.getByText('API (direct)'))

    expect(screen.getByText(/No file\/edit\/bash tools/)).toBeInTheDocument()
  })

  it('submits the create mutation with the api_models row id when the api engine is chosen', async () => {
    const { mutateAsync } = setup({
      apiModeEnabled: true,
      apiModels: [makeAPIModel({ id: 'api-sonnet', display_name: 'Sonnet (API)' })],
    })
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={onCreated} />)

    await openEngineDropdown(user)
    await user.click(screen.getByText('API (direct)'))
    await openModelDropdown(user)
    await user.click(screen.getByText('Sonnet (API)'))
    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(mutateAsync).toHaveBeenCalledWith({ engine: 'api', model: 'api-sonnet' })
    expect(onCreated).toHaveBeenCalledWith('sid-x')
  })

  it('clears the selected model when switching engines, even when the id collides across registries', async () => {
    setup({
      apiModeEnabled: true,
      cliModels: [makeCLIModel({ id: 'sonnet', cli_type: 'claude', display_name: 'Sonnet (CLI)' })],
      apiModels: [makeAPIModel({ id: 'sonnet', display_name: 'Sonnet (API)' })],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openModelDropdown(user)
    await user.click(screen.getByText('Sonnet (CLI)'))
    expect(screen.getByRole('button', { name: 'Sonnet (CLI)' })).toBeInTheDocument()

    await openEngineDropdown(user)
    await user.click(screen.getByText('API (direct)'))

    expect(screen.getByRole('button', { name: /Select a model…/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sonnet (CLI)' })).not.toBeInTheDocument()
  })
})
