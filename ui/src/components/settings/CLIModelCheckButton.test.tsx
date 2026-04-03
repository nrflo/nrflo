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

  it('shows timeout error after 45s if server does not respond', async () => {
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

    // Component is in error state — open tooltip via pointerMove + advance Radix's 200ms open delay
    // Radix Tooltip listens to onPointerMove (not onPointerEnter) to trigger opening
    const tooltipTrigger = screen.getByRole('button').querySelector('[data-state]') as HTMLElement
    await act(async () => {
      fireEvent.pointerMove(tooltipTrigger)
      vi.advanceTimersByTime(300)
    })

    expect(screen.getByRole('tooltip')).toHaveTextContent('Timeout — server did not respond')
  })

  afterEach(() => vi.useRealTimers())

  it('shows error message from result.error on failure via tooltip', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({
      success: false,
      error: 'binary not found',
      duration_ms: 0,
    })
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    const btn = screen.getByRole('button')
    const tooltipTrigger = btn.querySelector('[data-state]') as HTMLElement
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('binary not found')
  })

  it('shows "Unknown error" when error field is absent via tooltip', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockResolvedValue({ success: false, duration_ms: 0 })
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    const btn = screen.getByRole('button')
    const tooltipTrigger = btn.querySelector('[data-state]') as HTMLElement
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Unknown error')
  })

  it('shows error message when testCLIModel throws via tooltip', async () => {
    vi.mocked(cliModelsApi.testCLIModel).mockRejectedValue(new Error('network failure'))
    render(<CLIModelCheckButton modelId="sonnet" />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check model/i }))

    const btn = screen.getByRole('button')
    const tooltipTrigger = btn.querySelector('[data-state]') as HTMLElement
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('network failure')
  })

  it('button is disabled when disabled prop is true', () => {
    render(<CLIModelCheckButton modelId="sonnet" disabled />)
    expect(screen.getByRole('button')).toBeDisabled()
  })
})
