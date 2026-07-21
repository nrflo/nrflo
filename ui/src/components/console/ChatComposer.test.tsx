import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatComposer } from './ChatComposer'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import * as useChatToolsHook from '@/hooks/useChatTools'
import type { ConsoleSkill } from '@/types/consoleChat'

vi.mock('@/hooks/useConsoleChats', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/hooks/useConsoleChats')>()
  return { ...actual, useProjectSkills: vi.fn() }
})

vi.mock('@/hooks/useChatTools', () => ({
  useChatTools: vi.fn(),
  useInvokeChatTool: vi.fn(),
}))

const SKILLS: ConsoleSkill[] = [
  { name: 'finalize', description: 'Close out a chunk of work' },
  { name: 'find-bugs', description: 'Hunt for defects' },
  { name: 'deploy', description: 'Ship a release' },
]

function mockSkills(skills: ConsoleSkill[] = SKILLS) {
  vi.mocked(useConsoleChats.useProjectSkills).mockReturnValue({
    data: skills,
  } as unknown as ReturnType<typeof useConsoleChats.useProjectSkills>)
}

function setup(overrides: Partial<Parameters<typeof ChatComposer>[0]> = {}) {
  const onSend = vi.fn()
  const onStop = vi.fn()
  render(
    <ChatComposer
      sid="sid-1"
      isRunning={false}
      sendPending={false}
      stopPending={false}
      onSend={onSend}
      onStop={onStop}
      {...overrides}
    />
  )
  return { onSend, onStop }
}

describe('ChatComposer', () => {
  beforeEach(() => {
    vi.mocked(useChatToolsHook.useChatTools).mockReturnValue({
      data: [],
    } as unknown as ReturnType<typeof useChatToolsHook.useChatTools>)
    vi.mocked(useChatToolsHook.useInvokeChatTool).mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue({ ok: true }),
      isPending: false,
    } as unknown as ReturnType<typeof useChatToolsHook.useInvokeChatTool>)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    delete (HTMLTextAreaElement.prototype as unknown as Record<string, unknown>).scrollHeight
  })

  it('Enter sends the trimmed value and clears the box', async () => {
    mockSkills([])
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, '  hello  {Enter}')

    expect(onSend).toHaveBeenCalledWith('hello')
    expect(box).toHaveValue('')
  })

  it('Shift+Enter inserts a newline and does not send', async () => {
    mockSkills([])
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, 'line1{Shift>}{Enter}{/Shift}line2')

    expect(onSend).not.toHaveBeenCalled()
    expect(box).toHaveValue('line1\nline2')
  })

  it('disables the textarea and shows Stop while a turn is running; clicking Stop calls onStop', async () => {
    mockSkills([])
    const { onStop } = setup({ isRunning: true })
    const user = userEvent.setup()

    const box = screen.getByPlaceholderText('Waiting for the agent to finish its turn…')
    expect(box).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Stop' }))
    expect(onStop).toHaveBeenCalled()
  })

  it('Send is disabled when the draft is empty', () => {
    mockSkills([])
    setup()
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('shows a Spinner instead of the Send label when sendPending', () => {
    mockSkills([])
    setup({ sendPending: true })
    const sendButton = screen.getByRole('button', { name: 'Send' })
    expect(sendButton).toBeDisabled()
  })

  it('shows a Spinner instead of the Stop label when stopPending', () => {
    mockSkills([])
    setup({ isRunning: true, stopPending: true })
    const stopButton = screen.getByRole('button')
    expect(stopButton).toBeDisabled()
    expect(stopButton).not.toHaveTextContent('Stop')
  })

  it('autoresizes: grows height on multi-line input and resets to auto on send', async () => {
    mockSkills([])
    Object.defineProperty(HTMLTextAreaElement.prototype, 'scrollHeight', {
      configurable: true,
      get: () => 120,
    })
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

    await user.type(box, 'line1{Shift>}{Enter}{/Shift}line2')
    expect(box.style.height).toBe('120px')

    await user.type(box, '{Enter}')
    expect(onSend).toHaveBeenCalled()
    expect(box.style.height).toBe('auto')
  })

  describe('"/" skill suggestions', () => {
    it('opens the dropdown when typing "/" into an empty box, listing all skills', async () => {
      mockSkills()
      setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…')

      await user.type(box, '/')

      expect(screen.getByText('/finalize')).toBeInTheDocument()
      expect(screen.getByText('/find-bugs')).toBeInTheDocument()
      expect(screen.getByText('/deploy')).toBeInTheDocument()
    })

    it('filters to matching skills as the name is typed', async () => {
      mockSkills()
      setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…')

      await user.type(box, '/fi')

      expect(screen.getByText('/finalize')).toBeInTheDocument()
      expect(screen.queryByText('/deploy')).not.toBeInTheDocument()
    })

    it('ArrowDown/ArrowUp move the highlighted row', async () => {
      mockSkills()
      setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…')

      await user.type(box, '/')
      const rowFor = (name: string) => screen.getByText(`/${name}`).closest('div')

      // Row 0 is the reserved '/invoke' directive; skills follow.
      expect(rowFor('invoke')).toHaveClass('bg-muted')

      await user.keyboard('{ArrowDown}')
      expect(rowFor('finalize')).toHaveClass('bg-muted')
      expect(rowFor('invoke')).not.toHaveClass('bg-muted')

      await user.keyboard('{ArrowUp}')
      expect(rowFor('invoke')).toHaveClass('bg-muted')
    })

    it('Enter selects the highlighted skill, inserts "/name ", and does not send', async () => {
      mockSkills()
      const { onSend } = setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

      await user.type(box, '/fi{Enter}')

      expect(box).toHaveValue('/finalize ')
      expect(onSend).not.toHaveBeenCalled()
      expect(screen.queryByText('Close out a chunk of work')).not.toBeInTheDocument()
    })

    it('Tab selects the highlighted skill and inserts "/name "', async () => {
      mockSkills()
      setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

      await user.type(box, '/dep')
      await user.tab()

      expect(box).toHaveValue('/deploy ')
    })

    it('Escape closes the dropdown without clearing the typed text', async () => {
      mockSkills()
      setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

      await user.type(box, '/fi')
      await user.keyboard('{Escape}')

      expect(box).toHaveValue('/fi')
      expect(screen.queryByText('/finalize')).not.toBeInTheDocument()
    })

    it('closes the dropdown once a space follows the name, and Enter then sends normally', async () => {
      mockSkills()
      const { onSend } = setup()
      const user = userEvent.setup()
      const box = screen.getByPlaceholderText('Message the agent…')

      await user.type(box, '/finalize ')
      expect(screen.queryByText('Close out a chunk of work')).not.toBeInTheDocument()

      await user.type(box, 'now{Enter}')
      expect(onSend).toHaveBeenCalledWith('/finalize now')
    })
  })
})
