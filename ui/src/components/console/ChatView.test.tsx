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
    useRevokeSessionApproval: vi.fn(),
    useSetYolo: vi.fn(),
  }
})

vi.mock('@/hooks/useConsoleChatStream', () => ({
  useConsoleChatStream: vi.fn(),
}))

function makeStream(
  turn: 'idle' | 'running',
  transcript: unknown[] = [],
  sessionApprovals: string[] = [],
  cost: number | undefined = undefined,
  yolo = false
) {
  return {
    transcript,
    turn,
    approvals: [],
    resolvedApprovals: new Map(),
    sessionApprovals,
    yolo,
    thinking: [],
    errors: [],
    rotations: [],
    contextLeft: undefined,
    cost,
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
  const revoke = vi.fn().mockResolvedValue(undefined)
  const setYolo = vi.fn().mockResolvedValue(undefined)
  vi.mocked(useConsoleChats.useConsoleChat).mockReturnValue({
    data: { session_id: 's1', engine: 'claude', model: 'sonnet' },
  } as ReturnType<typeof useConsoleChats.useConsoleChat>)
  vi.mocked(useConsoleChats.useSendConsoleChatMessage).mockReturnValue(
    mutation(send) as ReturnType<typeof useConsoleChats.useSendConsoleChatMessage>)
  vi.mocked(useConsoleChats.useCloseConsoleChat).mockReturnValue(
    mutation(close) as ReturnType<typeof useConsoleChats.useCloseConsoleChat>)
  vi.mocked(useConsoleChats.useInterruptConsoleChat).mockReturnValue(
    mutation(interrupt) as ReturnType<typeof useConsoleChats.useInterruptConsoleChat>)
  vi.mocked(useConsoleChats.useRevokeSessionApproval).mockReturnValue(
    mutation(revoke) as ReturnType<typeof useConsoleChats.useRevokeSessionApproval>)
  vi.mocked(useConsoleChats.useSetYolo).mockReturnValue(
    mutation(setYolo) as ReturnType<typeof useConsoleChats.useSetYolo>)
  vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
    makeStream(turn) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
  return { send, close, interrupt, revoke, setYolo }
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

  it('search filters the transcript and shows a match count; Esc clears', async () => {
    setup('idle')
    const transcript = [
      { kind: 'message', message: { content: 'find the needle here', category: 'text', created_at: '' } },
      { kind: 'message', message: { content: 'nothing relevant', category: 'text', created_at: '' } },
    ]
    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', transcript) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    const user = userEvent.setup()
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    expect(screen.getByText(/nothing relevant/)).toBeInTheDocument()

    const box = screen.getByLabelText('Search transcript')
    await user.type(box, 'needle')

    expect(screen.getByText('1 match')).toBeInTheDocument()
    expect(screen.queryByText(/nothing relevant/)).not.toBeInTheDocument()
    expect(screen.getByText(/find the needle here/)).toBeInTheDocument()

    await user.type(box, '{Escape}')
    expect(screen.getByText(/nothing relevant/)).toBeInTheDocument()
  })

  it('lists always-allowed tools as chips and revokes on ×', async () => {
    const { revoke } = setup('idle')
    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', [], ['bash', 'edit_file']) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    const user = userEvent.setup()
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    expect(screen.getByText('Always allowed:')).toBeInTheDocument()
    expect(screen.getByText('bash')).toBeInTheDocument()
    expect(screen.getByText('edit_file')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Revoke bash' }))
    expect(revoke).toHaveBeenCalledWith({ sid: 's1', tool: 'bash' })
  })

  it('shows ~$X.XX when the stream has a cost and omits it when cost is unset', () => {
    setup('idle')
    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', [], [], 1.234) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    const { unmount } = renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)
    expect(screen.getByText('~$1.23')).toBeInTheDocument()
    unmount()

    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle') as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)
    expect(screen.queryByText(/^~\$/)).not.toBeInTheDocument()
  })

  it('renders the status bar below the composer with engine·model and workdir', () => {
    setup('idle')
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    expect(screen.getByText('claude')).toBeInTheDocument()
    expect(screen.getByText('· sonnet')).toBeInTheDocument()
    expect(screen.getByText('/tmp/w')).toBeInTheDocument()
  })

  it('YOLO toggle shows current state, calls setYolo with the flipped value, and the status bar badge follows the stream', () => {
    setup('idle')
    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', [], [], undefined, true) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    const { unmount } = renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    expect(screen.getByRole('button', { name: 'YOLO on' })).toBeInTheDocument()
    expect(screen.getByText('YOLO')).toBeInTheDocument()
    unmount()

    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', [], [], undefined, false) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'YOLO off' })).toBeInTheDocument()
    expect(screen.queryByText('YOLO')).not.toBeInTheDocument()
  })

  it('clicking the YOLO toggle calls setYolo with the flipped value', async () => {
    const { setYolo } = setup('idle')
    vi.mocked(useConsoleChatStreamHook.useConsoleChatStream).mockReturnValue(
      makeStream('idle', [], [], undefined, false) as ReturnType<typeof useConsoleChatStreamHook.useConsoleChatStream>)
    const user = userEvent.setup()
    renderWithQuery(<ChatView sid="s1" onClosed={vi.fn()} onDetach={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'YOLO off' }))
    expect(setYolo).toHaveBeenCalledWith({ sid: 's1', on: true })
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
