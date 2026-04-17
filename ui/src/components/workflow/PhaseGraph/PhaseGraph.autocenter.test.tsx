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
    useReactFlow: () => ({
      fitView: mockFitView,
      zoomIn: mockZoomIn,
      zoomOut: mockZoomOut,
    }),
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

/** Flush the two mount timers in FitViewOnChange (nodeKey@100ms, container@150ms). */
function flushMountTimers() {
  act(() => { vi.advanceTimersByTime(150) })
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
  })

  afterEach(() => {
    vi.useRealTimers()
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

    act(() => { vi.advanceTimersByTime(15000) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })

    act(() => { vi.advanceTimersByTime(15000) })
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

    // Re-check; fit-view click unchecks and calls fitView with FIT_VIEW_OPTIONS.
    clickCheckbox(checkbox)
    expect(checkbox).toBeChecked()
    clickButtonByName('fit view')
    expect(checkbox).not.toBeChecked()
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })

    // With the toggle unchecked, the 15s interval must NOT fire again.
    act(() => { vi.advanceTimersByTime(15000) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
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

    act(() => { vi.advanceTimersByTime(15000) })
    expect(mockFitView).toHaveBeenCalledTimes(1)
    expect(mockFitView).toHaveBeenLastCalledWith({ padding: 0.3 })
  })
})
