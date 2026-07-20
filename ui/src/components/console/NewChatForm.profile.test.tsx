import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { NewChatForm } from './NewChatForm'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import * as useDefaultTemplates from '@/hooks/useDefaultTemplates'
import type { ConsoleCatalog, ConsoleEngineOption, ConsoleProfileOption } from '@/types/consoleChat'
import type { DefaultTemplate } from '@/api/defaultTemplates'

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return { ...actual, useConsoleCatalog: vi.fn(), useCreateConsoleChat: vi.fn() }
})

vi.mock('@/hooks/useDefaultTemplates', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useDefaultTemplates')>()
  return { ...actual, useInjectableTemplates: vi.fn() }
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
    models: [
      { id: 'sonnet-5', display_name: 'Sonnet (CLI)' },
      { id: 'opus-4-8', display_name: 'Opus 4.8', supported_efforts: ['low', 'medium', 'high', 'xhigh'] },
    ],
    ...overrides,
  }
}

function makeProfile(overrides: Partial<ConsoleProfileOption> = {}): ConsoleProfileOption {
  return {
    name: 't0-decider',
    display_name: 'T0 Decider',
    default_engine: 'claude',
    default_model_id: 'opus-4-8',
    default_effort: 'xhigh',
    context_budget_tokens: 50000,
    refinery_default: true,
    system_template_id: 'tier-t0-decider',
    ...overrides,
  }
}

const T0_HANDS = makeProfile({
  name: 't0-hands',
  display_name: 'T0 Hands',
  default_model_id: 'sonnet-5',
  default_effort: undefined,
  context_budget_tokens: 150000,
  refinery_default: false,
  system_template_id: undefined,
})

function makeCatalog(engines: ConsoleEngineOption[], profiles: ConsoleProfileOption[] = []): ConsoleCatalog {
  return { project_id: 'p1', engines, sessions: [], profiles }
}

function makeTemplate(overrides: Partial<DefaultTemplate> = {}): DefaultTemplate {
  return {
    id: 'tier-t0-decider',
    name: 'Tier T0 Decider',
    type: 'injectable',
    template: '',
    readonly: true,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

function setup({
  engines = [engineOption()],
  profiles = [makeProfile(), T0_HANDS],
  templates = [makeTemplate()],
  mutateAsync = vi.fn().mockResolvedValue({ session_id: 'sid-x' }),
}: {
  engines?: ConsoleEngineOption[]
  profiles?: ConsoleProfileOption[]
  templates?: DefaultTemplate[]
  mutateAsync?: ReturnType<typeof vi.fn>
} = {}) {
  vi.mocked(useConsoleChats.useConsoleCatalog).mockReturnValue({
    data: makeCatalog(engines, profiles),
  } as ReturnType<typeof useConsoleChats.useConsoleCatalog>)
  vi.mocked(useConsoleChats.useCreateConsoleChat).mockReturnValue({
    mutateAsync,
    isPending: false,
  } as unknown as ReturnType<typeof useConsoleChats.useCreateConsoleChat>)
  vi.mocked(useDefaultTemplates.useInjectableTemplates).mockReturnValue({
    data: templates,
  } as ReturnType<typeof useDefaultTemplates.useInjectableTemplates>)
  return { mutateAsync }
}

describe('NewChatForm - profile picker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('omits the Profile picker entirely when the catalog has no profiles', () => {
    setup({ profiles: [] })
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)
    expect(screen.queryByText('Profile')).not.toBeInTheDocument()
  })

  it('lists both profiles, prefills defaults on selection, and tags the create payload', async () => {
    const { mutateAsync } = setup()
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={onCreated} />)

    await user.click(screen.getByRole('button', { name: 'Custom' }))
    expect(screen.getByText('T0 Decider')).toBeInTheDocument()
    expect(screen.getByText('T0 Hands')).toBeInTheDocument()

    await user.click(screen.getByText('T0 Decider'))

    expect(screen.getByRole('button', { name: 'T0 Decider' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Opus 4.8' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Xhigh' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tier T0 Decider' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(mutateAsync).toHaveBeenCalledWith({
      engine: 'claude',
      model: 'opus-4-8',
      reasoning_effort: 'xhigh',
      system_template_id: 'tier-t0-decider',
      profile: 't0-decider',
    })
    expect(onCreated).toHaveBeenCalledWith('sid-x')
  })

  it('resets profile back to Custom when the engine is changed manually afterward', async () => {
    setup({
      engines: [
        engineOption(),
        engineOption({ id: 'codex', display_name: 'Codex', models: [{ id: 'gpt-5.4', display_name: 'GPT (Codex)' }] }),
      ],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Custom' }))
    await user.click(screen.getByText('T0 Decider'))
    expect(screen.getByRole('button', { name: 'T0 Decider' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Claude' }))
    await user.click(screen.getByText('Codex'))

    expect(screen.getByRole('button', { name: 'Custom' })).toBeInTheDocument()
  })
})
