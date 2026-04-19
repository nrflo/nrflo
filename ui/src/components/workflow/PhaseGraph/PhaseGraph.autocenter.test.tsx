/**
 * Auto-center toggle tests for PhaseGraph controls toolbar.
 * Covers: checkbox default state, 15s interval, manual zoom un-check, re-check resume.
 *
 * Uses fireEvent (not userEvent) for clicks because fake timers + Radix tooltip
 * portal timers hang userEvent's internal scheduler.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { PhaseGraph } from './PhaseGraph'
import type { PhaseGraphProps } from './types'
import type { PhaseState, ActiveAgentV4 } from '@/types/workflow'

const mockFitView = vi.fn()
const mockZoomIn = vi.fn()
const mockZoomOut = vi.fn()

/**
 * When true, the `useReactFlow` mock returns a NEW `fitView` identity on every
 * call — mimicking @xyflow/react versions where the instance is re-created per
 * render. This exposes the regression guarded by `AutoCenterInterval`'s ref
 * pattern. Scoped per-test; kept off for most tests since it causes
 * `FitViewOnChange`'s debounced effect to also re-arm on every render.
 */
let freshFitViewIdentity = false

vi.mock('@xyflow/react', async () => {
  const actual = await vi.importActual<typeof import('@xyflow/react')>('@xyflow/react')
  return {
    ...actual,
    ReactFlow: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="background" />,
    Controls: ({ children }: { children?: React.ReactNode }) => (
      <div data-testid="controls">{children}</div>
    ),
    ControlButton: ({
      children,
      onClick,
      ...rest
    }: React.ButtonHTMLAttributes<HTMLButtonElement> & { children?: React.ReactNode }) => (
      <button onClick={onClick} {...rest}>{children}</button>
    ),
    useReactFlow: () =>
      freshFitViewIdentity
        ? {
            fitView: (opts?: unknown) => mockFitView(opts),
            zoomIn: () => mockZoomIn(),
            zoomOut: () => mockZoomOut(),
          }
        : {
            fitView: mockFitView,
            zoomIn: mockZoomIn,
            zoomOut: mockZoomOut,
          },
    useStore: (selector: (s: Record<string, unknown>) => unknown) =>
      selector({ width: 800, height: 600 }),
  }
})

vi.mock('./layout', () => ({
  getLayoutedElements: (nodes: unknown[], edges: unknown[]) =>
    Promise.resolve({ nodes, edges }),
  BASE_HEIGHT: 110,
}))

vi.mock('./AgentFlowNode', () => ({
  AgentFlowNode: ({ data }: { data: { agentKey: string; phaseName: string } }) => (
    <div data-testid={`agent-node-${data.agentKey}`}>{data.phaseName}</div>
  ),
}))

vi.mock('@/hooks/useElapsedTime', () => ({ useTickingClock: vi.fn() }))
vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))

function makePhaseState(overrides: Partial<PhaseState> = {}): PhaseState {
  return { status: 'pending', ...overrides }
}

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_id: 'a1',
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude-sonnet-4-5',
    cli: 'claude',
    model: 'sonnet',
    pid: 12345,
    session_id: 's1',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function baseProps(): PhaseGraphProps {
  return {
    phases: { investigation: makePhaseState({ status: 'in_progress' }) },
    phaseOrder: ['investigation'],
    activeAgents: {
      'setup-analyzer:claude:sonnet': makeAgent({
        agent_type: 'setup-analyzer',
        phase: 'investigation',
      }),
    },
    agentHistory: [],
    sessions: [],
  }
}

async function flushLayout() {
  await act(async () => {})
}

/** Flush the two mount timers in FitViewOnChange (nodeKey@100ms, container@150ms) + performFitView's rAF (~16ms). */
function flushMountTimers() {
  act(() => { vi.advanceTimersByTime(200) })
}

/** Advance by 15s + enough to flush performFitView's rAF (~16ms). */
function advanceAutoCenterTick() {
  act(() => { vi.advanceTimersByTime(15050) })
}

/** Toggle the native checkbox. fireEvent.click flips `checked` synchronously. */
function clickCheckbox(checkbox: HTMLInputElement) {
  act(() => { fireEvent.click(checkbox) })
}

function clickButtonByName(name: string) {
  act(() => { fireEvent.click(screen.getByRole('button', { name })) })
}

describe('PhaseGraph - auto-center toggle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    freshFitViewIdentity = false
  })

  afterEach(() => {
    vi.useRealTimers()
    freshFitViewIdentity = false
  })

  it('renders the auto-center checkbox, checked by default, with the documented label', async () => {
    render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    flushMountTimers()

    const checkbox = screen.getByRole('checkbox', { name: 'Auto center graph every 15s' })
    expect(checkbox).toBeInTheDocument()
    expect(checkbox).toBeChecked()
  })

  it('fires fitView once every 15s while the toggle stays checked', async () => {
    render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    flushMountTimers()
    mockFitView.mockClear()

    advanceAutoCenterTick()
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })

    advanceAutoCenterTick()
    expect(mockFitView).toHaveBeenCalledTimes(2)
  })

  it('manual zoom-in / zoom-out / fit-view clicks uncheck the toggle and halt the interval', async () => {
    render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    flushMountTimers()
    mockFitView.mockClear()

    const checkbox = screen.getByRole('checkbox', {
      name: 'Auto center graph every 15s',
    }) as HTMLInputElement

    // Zoom-out click unchecks and calls zoomOut.
    clickButtonByName('zoom out')
    expect(mockZoomOut).toHaveBeenCalledTimes(1)
    expect(checkbox).not.toBeChecked()

    // Re-check; zoom-in click unchecks and calls zoomIn.
    clickCheckbox(checkbox)
    expect(checkbox).toBeChecked()
    clickButtonByName('zoom in')
    expect(mockZoomIn).toHaveBeenCalledTimes(1)
    expect(checkbox).not.toBeChecked()

    // Re-check; fit-view click unchecks and calls fitView with FIT_VIEW_OPTIONS
    // (deferred via performFitView's rAF — flush with a small advance).
    clickCheckbox(checkbox)
    expect(checkbox).toBeChecked()
    clickButtonByName('fit view')
    expect(checkbox).not.toBeChecked()
    act(() => { vi.advanceTimersByTime(20) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })

    // With the toggle unchecked, the 15s interval must NOT fire again.
    act(() => { vi.advanceTimersByTime(15000) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
  })

  // Regression: on the ticket workflow page, WS-driven session refreshes cause
  // PhaseGraph to re-render roughly once per second. In @xyflow/react the
  // `fitView` returned from `useReactFlow()` is re-created on each render, so
  // a naïve `[enabled, fitView]` dep array tore down and re-armed the 15s
  // interval before it could ever fire (see ticket Background). AutoCenterInterval
  // must keep fitView in a ref so the 15s interval survives parent re-renders.
  it('fires fitView at 15s even when the parent re-renders every second (ticket-page regression)', async () => {
    // Enable fresh fitView identity per useReactFlow() call to reproduce the
    // @xyflow/react churn on the ticket page (WS-driven session refreshes).
    // Without AutoCenterInterval's ref pattern, each re-render tears down the
    // 15s setInterval before it can fire.
    freshFitViewIdentity = true

    const props = baseProps()
    const { rerender } = render(<PhaseGraph {...props} />)
    await flushLayout()
    flushMountTimers()

    // Simulate 14 WS-driven re-renders, one per second, across the 15s window.
    for (let i = 0; i < 14; i++) {
      act(() => { vi.advanceTimersByTime(1000) })
      rerender(<PhaseGraph {...props} />)
    }
    // Settle FitViewOnChange's 150ms debounce so subsequent advances are
    // driven solely by AutoCenterInterval.
    act(() => { vi.advanceTimersByTime(200) })
    mockFitView.mockClear()

    // No further re-renders — FitViewOnChange will not fire. Advance 2s more.
    // With the ref fix the AutoCenterInterval armed near mount fires its 15s
    // tick during this window. Without the fix, each rerender re-armed the
    // interval and the latest arm's 15s tick is still in the future.
    act(() => { vi.advanceTimersByTime(2000) })

    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })
  })

  it('clicking the checkbox itself does not call fitView; re-checking resumes the 15s interval', async () => {
    render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    flushMountTimers()
    mockFitView.mockClear()

    const checkbox = screen.getByRole('checkbox', {
      name: 'Auto center graph every 15s',
    }) as HTMLInputElement

    // Toggle off via the checkbox — must NOT call fitView and must stop the interval.
    clickCheckbox(checkbox)
    expect(checkbox).not.toBeChecked()
    expect(mockFitView).not.toHaveBeenCalled()
    act(() => { vi.advanceTimersByTime(15000) })
    expect(mockFitView).not.toHaveBeenCalled()

    // Toggle back on — must NOT immediately call fitView, then fire once per 15s.
    clickCheckbox(checkbox)
    expect(checkbox).toBeChecked()
    expect(mockFitView).not.toHaveBeenCalled()

    advanceAutoCenterTick()
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })
  })

  // Ticket parity assertion: the Fit View button click and the 15s auto-center
  // tick must end up calling the same fitView function with identical args.
  // Previously these two paths produced visibly different zooms; both now route
  // through performFitView so the captured call args must deep-equal.
  it('Fit View button and 15s auto-center tick pass identical args to fitView', async () => {
    render(<PhaseGraph {...baseProps()} />)
    await flushLayout()
    flushMountTimers()
    mockFitView.mockClear()

    // 15s interval path — capture args.
    advanceAutoCenterTick()
    expect(mockFitView).toHaveBeenCalledTimes(1)
    const intervalArgs = mockFitView.mock.calls[0]

    // Re-check (advanceAutoCenterTick does not un-check, but a preceding manual
    // click would). The button path also unchecks the toggle regardless.
    const checkbox = screen.getByRole('checkbox', {
      name: 'Auto center graph every 15s',
    }) as HTMLInputElement
    expect(checkbox).toBeChecked()

    mockFitView.mockClear()

    // Button path — capture args.
    clickButtonByName('fit view')
    act(() => { vi.advanceTimersByTime(20) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    const buttonArgs = mockFitView.mock.calls[0]

    // The ticket's core assertion: same helper -> same args -> same viewport.
    expect(buttonArgs).toEqual(intervalArgs)
    expect(buttonArgs).toEqual([{ padding: 0.3 }])
  })
})
