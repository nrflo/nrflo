import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CLIModelCheckButton } from './CLIModelCheckButton'
import * as cliModelsApi from '@/api/cliModels'

vi.mock('@/api/cliModels')

describe('CLIModelCheckButton', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders idle Zap button with title and not disabled', () => {
    render(<CLIModelCheckButton modelId="sonnet" />)
    const btn = screen.getByRole('button', { name: /check model/i })
    expect(btn).toBeInTheDocument()
    expect(btn).not.toBeDisabled()
  })

  it('calls testCLIModel with the provided modelId on click', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({ success: true, duration_ms: 100 })
    render(<CLIModelCheckButton modelId="my-custom-model" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(cliModelsApi.testCLIModel).toHaveBeenCalledWith('my-custom-model', expect.any(AbortSignal))
  })

  it('shows spinner and disables button during testing', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockReturnValue(new Promise(() => {}))
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('shows duration text on success', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({ success: true, duration_ms: 1234 })
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(await screen.findByText('1234ms')).toBeInTheDocument()
  })

  it('auto-fades success indicator back to idle after 3s', async () => {
    vi.useFakeTimers()
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({ success: true, duration_ms: 500 })

    render(<CLIModelCheckButton modelId="sonnet" />)

    // Use fireEvent to avoid userEvent's internal timer dependencies
    await act(async () => {
      fireEvent.click(screen.getByRole('button'))
    })

    expect(screen.getByText('500ms')).toBeInTheDocument()

    act(() => { vi.advanceTimersByTime(3000) })

    expect(screen.queryByText('500ms')).not.toBeInTheDocument()
  })

  it('shows timeout error in dialog after 45s if server does not respond', async () => {
    // Only fake setTimeout/clearTimeout — leave setImmediate/nextTick real so Radix can render
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })

    // Mock that stays pending until the abort signal fires
    vi.mocked(cliModelsApi.testCLIModel).mockImplementation((_id, signal) =>
      new Promise<never>((_, reject) => {
        signal?.addEventListener('abort', () =>
          reject(new DOMException('The operation was aborted.', 'AbortError'))
        )
      })
    )

    render(<CLIModelCheckButton modelId="opencode-model" />)

    await act(async () => {
      fireEvent.click(screen.getByRole('button'))
    })

    // Fire the 45s client-side timeout → controller.abort() → mock rejects with AbortError
    await act(async () => {
      vi.advanceTimersByTime(45_000)
    })

    // Dialog opens automatically on error
    expect(screen.getByText('Timeout — server did not respond')).toBeInTheDocument()
  })

  afterEach(() => vi.useRealTimers())

  it('shows error in popup dialog automatically on failure', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'binary not found',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(await screen.findByText('binary not found')).toBeInTheDocument()
  })

  it('shows "Unknown error" in popup dialog when error field is absent', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({ success: false, duration_ms: 0 })
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(await screen.findByText('Unknown error')).toBeInTheDocument()
  })

  it('shows error in popup dialog when testCLIModel throws', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockRejectedValue(new Error('network failure'))
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    expect(await screen.findByText('network failure')).toBeInTheDocument()
  })

  it('closes error dialog when re-testing', async () => {
    const user = userEvent.setup()
    vi.mocked(cliModelsApi.testCLIModel)
      .mockResolvedValueOnce({ success: false, error: 'connection refused', duration_ms: 0 })
      .mockReturnValueOnce(new Promise(() => {}))

    render(<CLIModelCheckButton modelId="sonnet" />)

    await user.click(screen.getByRole('button', { name: /check model/i }))

    // Dialog opens automatically
    expect(await screen.findByText('connection refused')).toBeInTheDocument()

    // Re-test closes dialog
    await user.click(screen.getByRole('button', { name: /check model/i }))
    expect(screen.queryByText('connection refused')).not.toBeInTheDocument()
  })

  it('opens error dialog automatically on failure', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'unique-inline-check-error',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="sonnet" />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    // Dialog opens automatically — error visible without clicking the icon
    expect(await screen.findByText('unique-inline-check-error')).toBeInTheDocument()
  })

  it('dialog header includes modelId for context', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'oops',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="my-custom-model" />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))
    expect(await screen.findByText(/Model Check Error.*my-custom-model/)).toBeInTheDocument()
  })

  it('dialog closes when Close button is clicked', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'close-button-test-error',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="sonnet" />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))
    expect(await screen.findByText('close-button-test-error')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByText('close-button-test-error')).not.toBeInTheDocument()
  })

  it('dialog closes when Escape key is pressed', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'escape-key-test-error',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="sonnet" />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))
    expect(await screen.findByText('escape-key-test-error')).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByText('escape-key-test-error')).not.toBeInTheDocument()
  })

  it('button is disabled when disabled prop is true', () => {
    render(<CLIModelCheckButton modelId="sonnet" disabled />)
    expect(screen.getByRole('button')).toBeDisabled()
  })
})
