import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { InstanceList } from './ProjectWorkflowComponents'
import type { WorkflowState } from '@/types/workflow'

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    version: 4,
    instance_id: 'inst-1',
    current_phase: 'implementation',
    status: 'active',
    phases: {},
    phase_order: [],
    ...overrides,
  }
}

describe('InstanceList — running tab badges', () => {
  it('shows a plan status label with the secondary badge variant for a plan-suspended instance', () => {
    render(
      <InstanceList
        instanceIds={['inst-1']}
        instances={{ 'inst-1': makeState({ status: 'waiting_approval' }) }}
        labels={{ 'inst-1': 'feature' }}
        selectedId=""
        onSelect={vi.fn()}
        tab="running"
      />
    )
    const badge = screen.getByText('Awaiting plan approval')
    expect(badge).toHaveClass('bg-secondary')
  })

  it('shows the raw "active" label with the default badge variant for a running instance', () => {
    render(
      <InstanceList
        instanceIds={['inst-1']}
        instances={{ 'inst-1': makeState({ status: 'active' }) }}
        labels={{ 'inst-1': 'feature' }}
        selectedId=""
        onSelect={vi.fn()}
        tab="running"
      />
    )
    const badge = screen.getByText('active')
    expect(badge).toHaveClass('bg-primary')
  })

  it('shows the "failed" label with the destructive badge variant', () => {
    render(
      <InstanceList
        instanceIds={['inst-1']}
        instances={{ 'inst-1': makeState({ status: 'failed' }) }}
        labels={{ 'inst-1': 'feature' }}
        selectedId=""
        onSelect={vi.fn()}
        tab="running"
      />
    )
    const badge = screen.getByText('failed')
    expect(badge).toHaveClass('bg-destructive')
  })

  it('maps each plan-boundary status to its human label', () => {
    const instances: Record<string, WorkflowState> = {
      i1: makeState({ instance_id: 'i1', status: 'planning' }),
      i2: makeState({ instance_id: 'i2', status: 'plan_ready' }),
      i3: makeState({ instance_id: 'i3', status: 'waiting_input' }),
    }
    render(
      <InstanceList
        instanceIds={['i1', 'i2', 'i3']}
        instances={instances}
        labels={{ i1: 'a', i2: 'b', i3: 'c' }}
        selectedId=""
        onSelect={vi.fn()}
        tab="running"
      />
    )
    expect(screen.getByText('Planning')).toBeInTheDocument()
    expect(screen.getByText('Awaiting plan approval')).toBeInTheDocument()
    expect(screen.getByText('Needs input')).toBeInTheDocument()
  })
})
