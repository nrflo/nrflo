import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ChatView } from './ChatView'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import * as useConsoleChatStreamHook from '@/hooks/useConsoleChatStream'

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return {
    ...actual,
    useConsoleChat: vi.fn(),
    useSendConsoleChatMessage: vi.fn(),
    useCloseConsoleChat: vi.fn(),
    useInterruptConsoleChat: vi.fn(),
  }
})

vi.mock('@/hooks/useConsoleChatStream', () => ({
  useConsoleChatStream: vi.fn(),
}))

function makeStream(turn: 'idle' | 'running') {
  return {
    transcript: [],
    turn,
    approvals: [],
    resolvedApprovals: new Map(),
    thinking: [],
    errors: [],
    contextLeft: undefined,
    workDir: '/tmp/w',
    isLoadingHistory: false,
  }
}

function mutation(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  return { mutateAsync, isPending: false } as unknown
}

function setup(turn: 'idle' | 'running') {
  const send = vi.fn().mockResolvedValue(undefined)
  const close = vi.fn().mockResolvedValue(undefined)
  const interrupt = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useConsoleChats.useConsoleChat).mockReturnValue({
    data: { session_id: 's1', engine: 'claude', model: 'sonnet' },
  } as ReturnType<typeof useConsoleChats.useConsoleChat>)
  vi.mocked(useConsoleChats.useSendConsoleChatMessage).mockReturnValue(
    mutation(send) as ReturnType<typeof useConsoleChats.useSendConsoleChatMessage>)
  vi.mocked(useConsoleChats.useCloseConsoleChat).mockReturnValue(
    mutation(close) as ReturnType<typeof useConsoleChats.useCloseConsoleChat>)
  vi.mocked(useConsoleChats.useInterruptConsoleChat).mockReturnValue(
    mutation(interrupt) as ReturnType<typeof useConsoleChats.useInterruptConsoleChat>)
  vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
    makeStream(turn) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
  return { send, close, interrupt }
}

describe('ChatView turn controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // jsdom has no Element.scrollTo; ChatView auto-scrolls on new items.
    Element.prototype.scrollTo = vi.fn()
  })

  it('shows Send when idle and swaps to Stop (interrupt) while a turn runs', async () => {
    const { interrupt } = setup('running')
    const user = userEvent.setup()
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Stop' }))
    expect(interrupt).toHaveBeenCalledWith('s1')
  })

  it('Detach deselects without closing; Close tears the chat down', async () => {
    const { close } = setup('idle')
    const onDetach = vi.fn()
    const onClosed = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<ChatView sid="s1" onClosed={onClosed} onDetach={onDetach} />)

    await user.click(screen.getByRole('button', { name: 'Detach' }))
    expect(onDetach).toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(close).toHaveBeenCalledWith('s1')
    expect(onClosed).toHaveBeenCalled()
  })
})
