import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { WorkflowSubTabBar } from './WorkflowSubTabBar'

describe('WorkflowSubTabBar — Trace tab', () => {
  it('renders the Trace tab and fires onSwitch with "trace"', () => {
    const onSwitch = vi.fn()
    render(
      <WorkflowSubTabBar
        activeSubTab="running"
        onSwitch={onSwitch}
        runningCount={1}
        failedCount={0}
        completedCount={2}
      />
    )
    const traceTab = screen.getByText('Trace')
    expect(traceTab).toBeInTheDocument()
    fireEvent.click(traceTab)
    expect(onSwitch).toHaveBeenCalledWith('trace')
  })

  it('highlights Trace when active', () => {
    render(
      <WorkflowSubTabBar
        activeSubTab="trace"
        onSwitch={() => {}}
        runningCount={0}
        failedCount={0}
        completedCount={0}
      />
    )
    expect(screen.getByText('Trace').className).toContain('bg-primary/10')
  })
})
