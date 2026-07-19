import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { NewChatForm } from './NewChatForm'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import * as useDefaultTemplates from '@/hooks/useDefaultTemplates'
import type { ConsoleCatalog, ConsoleEngineOption } from '@/types/consoleChat'
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
    models: [{ id: 'sonnet-5', display_name: 'Sonnet (CLI)' }],
    ...overrides,
  }
}

function makeCatalog(engines: ConsoleEngineOption[]): ConsoleCatalog {
  return { project_id: 'p1', engines, sessions: [] }
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
  templates = [makeTemplate()],
  mutateAsync = vi.fn().mockResolvedValue({ session_id: 'sid-x' }),
}: {
  engines?: ConsoleEngineOption[]
  templates?: DefaultTemplate[]
  mutateAsync?: ReturnType<typeof vi.fn>
} = {}) {
  vi.mocked(useConsoleChats.useConsoleCatalog).mockReturnValue({
    data: makeCatalog(engines),
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

describe('NewChatForm - system template', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('lists injectable templates plus the default option', async () => {
    setup({ templates: [makeTemplate(), makeTemplate({ id: 'tier-t1-executor', name: 'Tier T1 Executor' })] })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: /Default \(global rules\)/ }))
    expect(screen.getByText('Tier T0 Decider')).toBeInTheDocument()
    expect(screen.getByText('Tier T1 Executor')).toBeInTheDocument()
  })

  it('omits system_template_id from the create payload when left at default', async () => {
    const { mutateAsync } = setup()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'New chat' }))
    expect(mutateAsync).toHaveBeenCalledWith({ engine: 'claude', model: '' })
  })

  it('includes system_template_id in the create payload when a template is selected', async () => {
    const { mutateAsync } = setup()
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={onCreated} />)

    await user.click(screen.getByRole('button', { name: /Default \(global rules\)/ }))
    await user.click(screen.getByText('Tier T0 Decider'))
    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(mutateAsync).toHaveBeenCalledWith({ engine: 'claude', model: '', system_template_id: 'tier-t0-decider' })
    expect(onCreated).toHaveBeenCalledWith('sid-x')
  })

  it('resets the selected template when the engine changes', async () => {
    setup({
      engines: [
        engineOption(),
        engineOption({ id: 'codex', display_name: 'Codex', models: [{ id: 'gpt-5.4', display_name: 'GPT (Codex)' }] }),
      ],
    })
    const user = userEvent.setup()
    renderWithQuery(<NewChatForm onCreated={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: /Default \(global rules\)/ }))
    await user.click(screen.getByText('Tier T0 Decider'))
    expect(screen.getByRole('button', { name: 'Tier T0 Decider' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Claude' }))
    await user.click(screen.getByText('Codex'))

    expect(screen.getByRole('button', { name: /Default \(global rules\)/ })).toBeInTheDocument()
  })
})
