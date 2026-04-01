import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowInstanceTable } from './WorkflowInstanceTable'
import type { WorkflowState } from '@/types/workflow'

const PAGE_SIZE = 10

function makeState(overrides: Partial<WorkflowState> = {}): WorkflowState {
  return {
    workflow: 'feature',
    instance_id: 'abcdef1234567890',
    version: 4,
    scope_type: 'project',
    current_phase: 'impl',
    status: 'completed',
    phases: {},
    phase_order: [],
    active_agents: {},
    agent_history: [],
    findings: {},
    ...overrides,
  }
}

const INST_ID = 'abcdef1234567890'
const SHORT_ID = '#abcdef12'

describe('WorkflowInstanceTable', () => {
  it('returns null when instanceIds is empty', () => {
    const { container } = render(
      <WorkflowInstanceTable
        instanceIds={[]}
        instances={{}}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders table headers', () => {
    const state = makeState()
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    expect(screen.getByText('Workflow')).toBeInTheDocument()
    expect(screen.getByText('Instance')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(screen.getByText('Duration')).toBeInTheDocument()
    expect(screen.getByText('Completed At')).toBeInTheDocument()
  })

  it('renders instance row with short ID and workflow name', () => {
    const state = makeState({ workflow: 'bugfix' })
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    expect(screen.getByText(SHORT_ID)).toBeInTheDocument()
    expect(screen.getByText('bugfix')).toBeInTheDocument()
  })

  it('shows dash for missing duration and completed_at', () => {
    const state = makeState({ total_duration_sec: undefined, completed_at: undefined })
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    const row = screen.getByText(SHORT_ID).closest('tr')!
    const cells = within(row).getAllByRole('cell')
    // Duration cell (index 3) and Completed At cell (index 4) should show dash
    expect(cells[3].textContent).toBe('-')
    expect(cells[4].textContent).toBe('-')
  })

  it('shows fail badge for failed status', () => {
    const state = makeState({ status: 'failed' })
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    expect(screen.getByText('fail')).toBeInTheDocument()
  })

  it('shows pass badge for completed status', () => {
    const state = makeState({ status: 'completed' })
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={vi.fn()}
        onDelete={vi.fn()}
      />
    )
    expect(screen.getByText('pass')).toBeInTheDocument()
  })

  it('calls onSelect with instance id when row is clicked', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    const state = makeState()
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={onSelect}
        onDelete={vi.fn()}
      />
    )
    await user.click(screen.getByText(SHORT_ID))
    expect(onSelect).toHaveBeenCalledOnce()
    expect(onSelect).toHaveBeenCalledWith(INST_ID)
  })

  it('calls onDelete with instance id when delete button is clicked, not onSelect', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    const onDelete = vi.fn()
    const state = makeState()
    render(
      <WorkflowInstanceTable
        instanceIds={[INST_ID]}
        instances={{ [INST_ID]: state }}
        selectedId=""
        onSelect={onSelect}
        onDelete={onDelete}
      />
    )
    const row = screen.getByText(SHORT_ID).closest('tr')!
    await user.click(within(row).getByRole('button'))
    expect(onDelete).toHaveBeenCalledOnce()
    expect(onDelete).toHaveBeenCalledWith(INST_ID)
    expect(onSelect).not.toHaveBeenCalled()
  })

  describe('pagination', () => {
    function makePageInstances(count: number) {
      const instanceIds = Array.from({ length: count }, (_, i) => `${String(i).padStart(2, '0')}abcdef1234`)
      const instances: Record<string, WorkflowState> = {}
      for (const id of instanceIds) {
        instances[id] = makeState({ instance_id: id, workflow: 'feature' })
      }
      return { instanceIds, instances }
    }

    it('hides pagination controls when items fit in one page', () => {
      const { instanceIds, instances } = makePageInstances(PAGE_SIZE)
      render(
        <WorkflowInstanceTable
          instanceIds={instanceIds}
          instances={instances}
          selectedId=""
          onSelect={vi.fn()}
          onDelete={vi.fn()}
        />
      )
      expect(screen.queryByText(new RegExp(`of ${PAGE_SIZE}`))).not.toBeInTheDocument()
    })

    it('shows range text and Prev disabled on first page when items exceed page size', () => {
      const total = PAGE_SIZE + 5
      const { instanceIds, instances } = makePageInstances(total)
      render(
        <WorkflowInstanceTable
          instanceIds={instanceIds}
          instances={instances}
          selectedId=""
          onSelect={vi.fn()}
          onDelete={vi.fn()}
        />
      )
      expect(screen.getByText(`1–${PAGE_SIZE} of ${total}`)).toBeInTheDocument()
      expect(document.querySelectorAll('tbody tr')).toHaveLength(PAGE_SIZE)
      const buttons = screen.getAllByRole('button')
      expect(buttons[buttons.length - 2]).toBeDisabled()   // Prev
      expect(buttons[buttons.length - 1]).not.toBeDisabled() // Next
    })

    it('navigates to next page and back with Prev/Next buttons', async () => {
      const user = userEvent.setup()
      const total = PAGE_SIZE + 5
      const { instanceIds, instances } = makePageInstances(total)
      render(
        <WorkflowInstanceTable
          instanceIds={instanceIds}
          instances={instances}
          selectedId=""
          onSelect={vi.fn()}
          onDelete={vi.fn()}
        />
      )

      // Navigate to page 2
      const buttons = screen.getAllByRole('button')
      await user.click(buttons[buttons.length - 1]) // Next

      expect(screen.getByText(`${PAGE_SIZE + 1}–${total} of ${total}`)).toBeInTheDocument()
      expect(document.querySelectorAll('tbody tr')).toHaveLength(5)

      const page2Buttons = screen.getAllByRole('button')
      expect(page2Buttons[page2Buttons.length - 1]).toBeDisabled()   // Next disabled on last page
      expect(page2Buttons[page2Buttons.length - 2]).not.toBeDisabled() // Prev enabled

      // Navigate back to page 1
      await user.click(page2Buttons[page2Buttons.length - 2]) // Prev
      expect(screen.getByText(`1–${PAGE_SIZE} of ${total}`)).toBeInTheDocument()
      expect(document.querySelectorAll('tbody tr')).toHaveLength(PAGE_SIZE)
    })
  })
})
