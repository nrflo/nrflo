import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ChatSiblingActions, isT0Profile } from './ChatSiblingActions'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import type { ConsoleCatalog, ConsoleModelOption } from '@/types/consoleChat'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return {
    ...actual,
    useConsoleCatalog: vi.fn(),
    useSwitchConsoleChatModel: vi.fn(),
    useOpenHandsSibling: vi.fn(),
  }
})

import { toast } from 'sonner'

function makeCatalog(models: ConsoleModelOption[]): ConsoleCatalog {
  return {
    project_id: 'p1',
    engines: [
      { id: 'claude', display_name: 'Claude', kind: 'cli', enabled: true, requires_model: false, models },
    ],
    sessions: [],
    profiles: [],
  }
}

const MODELS: ConsoleModelOption[] = [
  { id: 'opus-4-8', display_name: 'Opus 4.8' },
  { id: 'sonnet-5', display_name: 'Sonnet 5' },
]

function mutation(mutateAsync: ReturnType<typeof vi.fn>) {
  return { mutateAsync, isPending: false } as unknown
}

function setup({
  switchAsync = vi.fn().mockResolvedValue({ sibling_session_id: 'sid-switch' }),
  handsAsync = vi.fn().mockResolvedValue({ sibling_session_id: 'sid-hands' }),
  models = MODELS,
}: {
  switchAsync?: ReturnType<typeof vi.fn>
  handsAsync?: ReturnType<typeof vi.fn>
  models?: ConsoleModelOption[]
} = {}) {
  vi.mocked(useConsoleChats.useConsoleCatalog).mockReturnValue({
    data: makeCatalog(models),
  } as ReturnType<typeof useConsoleChats.useConsoleCatalog>)
  vi.mocked(useConsoleChats.useSwitchConsoleChatModel).mockReturnValue(
    mutation(switchAsync) as ReturnType<typeof useConsoleChats.useSwitchConsoleChatModel>
  )
  vi.mocked(useConsoleChats.useOpenHandsSibling).mockReturnValue(
    mutation(handsAsync) as ReturnType<typeof useConsoleChats.useOpenHandsSibling>
  )
  return { switchAsync, handsAsync }
}

describe('isT0Profile', () => {
  it('is true only for the t0-decider/t0-hands profiles, gating ChatView render', () => {
    expect(isT0Profile('t0-decider')).toBe(true)
    expect(isT0Profile('t0-hands')).toBe(true)
    expect(isT0Profile('some-other-profile')).toBe(false)
    expect(isT0Profile(undefined)).toBe(false)
  })
})

describe('ChatSiblingActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('Switch model is disabled until a different model is picked, then spawns a sibling', async () => {
    const { switchAsync } = setup()
    const onOpenSibling = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(
      <ChatSiblingActions sid="s1" engine="claude" model="opus-4-8" onOpenSibling={onOpenSibling} />
    )

    expect(screen.getByRole('button', { name: 'Switch model' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Opus 4.8' }))
    await user.click(screen.getByText('Sonnet 5'))
    expect(screen.getByRole('button', { name: 'Switch model' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Switch model' }))

    expect(switchAsync).toHaveBeenCalledWith({ sid: 's1', req: { model: 'sonnet-5' } })
    expect(onOpenSibling).toHaveBeenCalledWith('sid-switch')
  })

  it('Open hands sibling spawns the sibling and reports a failure via toast', async () => {
    const { handsAsync } = setup()
    const onOpenSibling = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(
      <ChatSiblingActions sid="s1" engine="claude" model="opus-4-8" onOpenSibling={onOpenSibling} />
    )

    await user.click(screen.getByRole('button', { name: 'Open hands sibling' }))
    expect(handsAsync).toHaveBeenCalledWith('s1')
    expect(onOpenSibling).toHaveBeenCalledWith('sid-hands')
  })

  it('surfaces a toast and skips onOpenSibling when the switch-model mutation rejects', async () => {
    const { switchAsync } = setup({ switchAsync: vi.fn().mockRejectedValue(new Error('boom')) })
    const onOpenSibling = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(
      <ChatSiblingActions sid="s1" engine="claude" model="opus-4-8" onOpenSibling={onOpenSibling} />
    )

    await user.click(screen.getByRole('button', { name: 'Opus 4.8' }))
    await user.click(screen.getByText('Sonnet 5'))
    await user.click(screen.getByRole('button', { name: 'Switch model' }))

    expect(switchAsync).toHaveBeenCalled()
    expect(onOpenSibling).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith('Failed to switch model.')
  })
})
