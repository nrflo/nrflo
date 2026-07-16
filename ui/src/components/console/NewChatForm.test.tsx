import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { NewChatForm } from './NewChatForm'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import type { ConsoleCatalog, ConsoleEngineOption } from '@/types/consoleChat'

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return { ...actual, useConsoleCatalog: vi.fn(), useCreateConsoleChat: vi.fn() }
})

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: Object.assign(
    (selector: (s: { currentProject: string; projects: unknown[] }) => unknown) =>
      selector({ currentProject: '', projects: [] }),
    { getState: () => ({ setCurrentProject: vi.fn() }) },
  ),
}))

function engineOption(overrides: Partial<ConsoleEngineOption> = {}): ConsoleEngineOption {
  return {
    id: 'claude',
    display_name: 'Claude',
    kind: 'cli',
    brand: 'claude',
    enabled: true,
    requires_model: false,
    models: [{ id: 'sonnet', display_name: 'Sonnet (CLI)' }],
    ...overrides,
  }
}

function makeCatalog(engines: ConsoleEngineOption[]): ConsoleCatalog {
  return { project_id: 'p1', engines, sessions: [] }
}

const DEFAULT_ENGINES = [
  engineOption(),
  engineOption({ id: 'codex', display_name: 'Codex', models: [{ id: 'codex_gpt', display_name: 'GPT (Codex)' }] }),
  engineOption({
    id: 'api',
    display_name: 'Direct API',
    requires_model: true,
    models: [{ id: 'api-sonnet', display_name: 'Sonnet (API)', provider: 'anthropic' }],
  }),
]

function setup({
  engines = DEFAULT_ENGINES,
  mutateAsync = vi.fn().mockResolvedValue({ session_id: 'sid-x' }),
}: {
  engines?: ConsoleEngineOption[]
  mutateAsync?: ReturnType<typeof vi.fn>
} = {}) {
  vi.mocked(useConsoleChats.useConsoleCatalog).mockReturnValue({
    data: makeCatalog(engines),
  } as ReturnType<typeof useConsoleChats.useConsoleCatalog>)
  vi.mocked(useConsoleChats.useCreateConsoleChat).mockReturnValue({
    mutateAsync,
    isPending: false,
  } as unknown as ReturnType<typeof useConsoleChats.useCreateConsoleChat>)
  return { mutateAsync }
}

async function openEngineDropdown(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Claude' }))
}

describe('NewChatForm (catalog-driven)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders every catalog engine, marking disabled ones non-selectable', async () => {
    setup({
      engines: [
        engineOption(),
        engineOption({
          id: 'api',
          display_name: 'Direct API',
          enabled: false,
          disabled_reason: 'API mode is disabled',
          requires_model: true,
          models: [],
        }),
      ],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openEngineDropdown(user)

    const apiOption = screen.getByText('Direct API')
    expect(apiOption.closest('[aria-disabled="true"]')).not.toBeNull()
  })

  it("lists only the selected engine's catalog models", async () => {
    setup()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await openEngineDropdown(user)
    await user.click(screen.getByText('Direct API'))
    await user.click(screen.getByRole('button', { name: /Select a model…/ }))

    expect(await screen.findByText('Sonnet (API)')).toBeInTheDocument()
    expect(screen.queryByText('Sonnet (CLI)')).not.toBeInTheDocument()
  })

  it('allows creating a CLI chat without a model (engine default) but requires one for api', async () => {
    const { mutateAsync } = setup()
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={onCreated} />)

    // claude (requires_model=false): create enabled with empty model.
    await user.click(screen.getByRole('button', { name: 'New chat' }))
    expect(mutateAsync).toHaveBeenCalledWith({ engine: 'claude', model: '' })
    expect(onCreated).toHaveBeenCalledWith('sid-x')

    // api (requires_model=true): create disabled until a model is picked.
    await openEngineDropdown(user)
    await user.click(screen.getByText('Direct API'))
    expect(screen.getByRole('button', { name: 'New chat' })).toBeDisabled()
  })

  it('shows the no-tools note only for the api engine', async () => {
    setup()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    expect(screen.queryByText(/No file\/edit\/bash tools/)).not.toBeInTheDocument()

    await openEngineDropdown(user)
    await user.click(screen.getByText('Direct API'))

    expect(screen.getByText(/No file\/edit\/bash tools/)).toBeInTheDocument()
  })

  it('submits the api_models row id when the api engine is chosen', async () => {
    const { mutateAsync } = setup()
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={onCreated} />)

    await openEngineDropdown(user)
    await user.click(screen.getByText('Direct API'))
    await user.click(screen.getByRole('button', { name: /Select a model…/ }))
    await user.click(screen.getByText('Sonnet (API)'))
    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(mutateAsync).toHaveBeenCalledWith({ engine: 'api', model: 'api-sonnet' })
    expect(onCreated).toHaveBeenCalledWith('sid-x')
  })

  it('clears the selected model when switching engines, even when ids collide across registries', async () => {
    setup({
      engines: [
        engineOption({ models: [{ id: 'sonnet', display_name: 'Sonnet (CLI)' }] }),
        engineOption({
          id: 'api',
          display_name: 'Direct API',
          requires_model: true,
          models: [{ id: 'sonnet', display_name: 'Sonnet (API)' }],
        }),
      ],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: /Engine default/ }))
    await user.click(screen.getByText('Sonnet (CLI)'))
    expect(screen.getByRole('button', { name: 'Sonnet (CLI)' })).toBeInTheDocument()

    await openEngineDropdown(user)
    await user.click(screen.getByText('Direct API'))

    expect(screen.getByRole('button', { name: /Select a model…/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sonnet (CLI)' })).not.toBeInTheDocument()
  })
})
