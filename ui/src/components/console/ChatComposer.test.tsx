import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChatComposer } from './ChatComposer'

function setup(overrides: Partial<Parameters<typeof ChatComposer>[0]> = {}) {
  const onSend = vi.fn()
  const onStop = vi.fn()
  render(
    <ChatComposer
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
  afterEach(() => {
    vi.restoreAllMocks()
    delete (HTMLTextAreaElement.prototype as unknown as Record<string, unknown>).scrollHeight
  })

  it('Enter sends the trimmed value and clears the box', async () => {
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, '  hello  {Enter}')

    expect(onSend).toHaveBeenCalledWith('hello')
    expect(box).toHaveValue('')
  })

  it('Shift+Enter inserts a newline and does not send', async () => {
    const { onSend } = setup()
    const user = userEvent.setup()
    const box = screen.getByPlaceholderText('Message the agent…')

    await user.type(box, 'line1{Shift>}{Enter}{/Shift}line2')

    expect(onSend).not.toHaveBeenCalled()
    expect(box).toHaveValue('line1\nline2')
  })

  it('disables the textarea and shows Stop while a turn is running; clicking Stop calls onStop', async () => {
    const { onStop } = setup({ isRunning: true })
    const user = userEvent.setup()

    const box = screen.getByPlaceholderText('Waiting for the agent to finish its turn…')
    expect(box).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Stop' }))
    expect(onStop).toHaveBeenCalled()
  })

  it('Send is disabled when the draft is empty', () => {
    setup()
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('shows a Spinner instead of the Send label when sendPending', () => {
    setup({ sendPending: true })
    const sendButton = screen.getByRole('button', { name: 'Send' })
    expect(sendButton).toBeDisabled()
  })

  it('shows a Spinner instead of the Stop label when stopPending', () => {
    setup({ isRunning: true, stopPending: true })
    const stopButton = screen.getByRole('button')
    expect(stopButton).toBeDisabled()
    expect(stopButton).not.toHaveTextContent('Stop')
  })

  it('autoresizes: grows height on multi-line input and resets to auto on send', async () => {
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
})
