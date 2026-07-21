import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatInvokeForm } from './ChatInvokeForm'
import * as useChatToolsHook from '@/hooks/useChatTools'
import { TurnActiveError } from '@/api/consoleChats'
import type { ConsoleTool } from '@/types/consoleChat'

vi.mock('@/hooks/useChatTools', () => ({
  useInvokeChatTool: vi.fn(),
}))

const TOOL: ConsoleTool = {
  name: 'delete_file',
  description: 'Delete a file from the workspace',
  input_schema: {
    type: 'object',
    required: ['path'],
    properties: {
      path: { type: 'string', description: 'Path to delete' },
      count: { type: 'number', default: 3 },
      recursive: { type: 'boolean', default: true },
      mode: { type: 'string', enum: ['soft', 'hard'], default: 'soft' },
      extra: { type: 'object' },
    },
  },
}

function mockInvoke(mutateAsync = vi.fn().mockResolvedValue({ ok: true }), isPending = false) {
  vi.mocked(useChatToolsHook.useInvokeChatTool).mockReturnValue({
    mutateAsync,
    isPending,
  } as unknown as ReturnType<typeof useChatToolsHook.useInvokeChatTool>)
  return mutateAsync
}

function setup(overrides: Partial<Parameters<typeof ChatInvokeForm>[0]> = {}) {
  const onClose = vi.fn()
  render(<ChatInvokeForm sid="sid-1" tool={TOOL} onClose={onClose} {...overrides} />)
  return { onClose }
}

describe('ChatInvokeForm', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders one field per schema property with defaults pre-filled and required marked', () => {
    mockInvoke()
    setup()

    expect(screen.getByText('path')).toBeInTheDocument()
    expect(screen.getByText('*')).toBeInTheDocument()
    expect(screen.getByText('count')).toBeInTheDocument()
    expect(screen.getByText('recursive')).toBeInTheDocument()
    expect(screen.getByText('mode')).toBeInTheDocument()
    expect(screen.getByText('extra')).toBeInTheDocument()

    expect(screen.getByDisplayValue('3')).toBeInTheDocument() // count
    expect(screen.getAllByRole('switch')[0]).toHaveAttribute('aria-checked', 'true') // recursive
    expect(screen.getByRole('button', { name: /soft/ })).toBeInTheDocument() // mode dropdown
  })

  it('renders enum as a Dropdown, boolean as a Toggle, and object as a JSON textarea', () => {
    mockInvoke()
    setup()

    // enum -> dropdown trigger button showing the default
    expect(screen.getByRole('button', { name: /soft/ })).toBeInTheDocument()
    // boolean -> switch (recursive field + the footer's Inform model toggle)
    expect(screen.getAllByRole('switch')).toHaveLength(2)
    // object with no default -> empty JSON textarea (no visible value assertion needed beyond presence)
    expect(screen.getByText('extra')).toBeInTheDocument()
  })

  it('Run posts {tool, arguments, inform_model:true} by default', async () => {
    const mutateAsync = mockInvoke()
    const { onClose } = setup()
    const user = userEvent.setup()

    const pathInput = screen.getAllByRole('textbox')[0]
    await user.type(pathInput, '/tmp/file.txt')

    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(mutateAsync).toHaveBeenCalledWith({
      sid: 'sid-1',
      tool: 'delete_file',
      arguments: { path: '/tmp/file.txt', count: 3, recursive: true, mode: 'soft' },
      inform_model: true,
    })
    expect(onClose).toHaveBeenCalled()
  })

  it('toggling Inform model off sends inform_model:false', async () => {
    const mutateAsync = mockInvoke()
    setup()
    const user = userEvent.setup()

    const pathInput = screen.getAllByRole('textbox')[0]
    await user.type(pathInput, '/tmp/file.txt')
    await user.click(screen.getByRole('switch', { name: /inform model/i }))
    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ inform_model: false })
    )
  })

  it('disables Run while the mutation is pending', () => {
    mockInvoke(vi.fn(), true)
    setup()

    expect(screen.getByRole('button', { name: 'Run' })).toBeDisabled()
  })

  it('surfaces a 409 (TurnActiveError) inline and keeps the form open', async () => {
    const mutateAsync = vi.fn().mockRejectedValue(new TurnActiveError())
    mockInvoke(mutateAsync)
    const { onClose } = setup()
    const user = userEvent.setup()

    const pathInput = document.querySelector('input[type="text"], input:not([type])') as HTMLInputElement
    await user.type(pathInput, '/tmp/file.txt')
    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(await screen.findByText('A turn is already running.')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('blocks Run and shows a Required error when a required field is left empty', async () => {
    const mutateAsync = mockInvoke()
    setup()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(screen.getByText('Required')).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })

  it('Cancel calls onClose without invoking the tool', async () => {
    const mutateAsync = mockInvoke()
    const { onClose } = setup()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(onClose).toHaveBeenCalled()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
