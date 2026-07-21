import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatComposer } from './ChatComposer'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import * as useChatToolsHook from '@/hooks/useChatTools'
import type { ConsoleSkill, ConsoleTool } from '@/types/consoleChat'

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
]

const TOOLS: ConsoleTool[] = [
  { name: 'delete_file', description: 'Delete a file' },
  { name: 'list_files', description: 'List files' },
  { name: 'deploy_service', description: 'Deploy a service' },
]

function mockSkills(skills: ConsoleSkill[] = SKILLS) {
  vi.mocked(useConsoleChats.useProjectSkills).mockReturnValue({
    data: skills,
  } as unknown as ReturnType<typeof useConsoleChats.useProjectSkills>)
}

function mockTools(tools: ConsoleTool[] = TOOLS) {
  vi.mocked(useChatToolsHook.useChatTools).mockReturnValue({
    data: tools,
  } as unknown as ReturnType<typeof useChatToolsHook.useChatTools>)
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

describe('ChatComposer "/invoke" directive', () => {
  beforeEach(() => {
    mockSkills()
    vi.mocked(useChatToolsHook.useInvokeChatTool).mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue({ ok: true }),
      isPending: false,
    } as unknown as ReturnType<typeof useChatToolsHook.useInvokeChatTool>)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the reserved "/invoke" directive row when typing "/inv"', async () => {
    mockTools()
    setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, '/inv')

    expect(screen.getByText('/invoke')).toBeInTheDocument()
    expect(screen.queryByText('/finalize')).not.toBeInTheDocument()
  })

  it('selecting the directive (Enter) sets the box to "/invoke " and keeps the dropdown open', async () => {
    mockTools()
    setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

    await user.type(box, '/invoke{Enter}')

    expect(box).toHaveValue('/invoke ')
    expect(screen.getByText('/delete_file')).toBeInTheDocument()
  })

  it('selecting the directive by click also switches to tool mode', async () => {
    mockTools()
    setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

    await user.type(box, '/inv')
    await user.click(screen.getByText('/invoke'))

    expect(box).toHaveValue('/invoke ')
    expect(screen.getByText('/delete_file')).toBeInTheDocument()
  })

  it('lists the chat tools after typing "/invoke "', async () => {
    mockTools()
    setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, '/invoke ')

    expect(screen.getByText('/delete_file')).toBeInTheDocument()
    expect(screen.getByText('/list_files')).toBeInTheDocument()
    expect(screen.getByText('/deploy_service')).toBeInTheDocument()
  })

  it('filters tools by prefix-then-substring as the name is typed', async () => {
    mockTools()
    setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, '/invoke del')

    expect(screen.getByText('/delete_file')).toBeInTheDocument()
    expect(screen.queryByText('/list_files')).not.toBeInTheDocument()
    expect(screen.queryByText('/deploy_service')).not.toBeInTheDocument()
  })

  it('selecting a tool opens ChatInvokeForm and does not call onSend', async () => {
    mockTools()
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…') as HTMLTextAreaElement

    await user.type(box, '/invoke del{Enter}')

    expect(await screen.findByText('delete_file')).toBeInTheDocument()
    expect(screen.getByText('Delete a file')).toBeInTheDocument()
    expect(onSend).not.toHaveBeenCalled()
    // dropdown and draft text are cleared once the form opens
    expect(box).toHaveValue('')
    expect(screen.queryByText('/delete_file')).not.toBeInTheDocument()
  })
})
