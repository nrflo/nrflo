import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Table, TableBody } from '@/components/ui/Table'
import { SessionToolDistribution } from './SessionToolDistribution'
import type { ToolCallDistributionEntry } from '@/types/session'

function makeEntry(overrides: Partial<ToolCallDistributionEntry> = {}): ToolCallDistributionEntry {
  return {
    tool_name: 'Read',
    success: 8,
    error: 2,
    ...overrides,
  }
}

function renderDistribution(toolCalls?: ToolCallDistributionEntry[]) {
  return render(
    <Table>
      <TableBody>
        <SessionToolDistribution toolCalls={toolCalls} />
      </TableBody>
    </Table>
  )
}

describe('SessionToolDistribution', () => {
  it('renders nothing when toolCalls is undefined', () => {
    const { container } = renderDistribution(undefined)
    expect(container.querySelector('table')?.textContent).toBe('')
  })

  it('renders nothing when toolCalls is empty (session with no recorded tool calls)', () => {
    const { container } = renderDistribution([])
    expect(container.querySelector('table')?.textContent).toBe('')
  })

  it('renders per-tool rows with call counts', () => {
    renderDistribution([
      makeEntry({ tool_name: 'Read', success: 10, error: 0 }),
      makeEntry({ tool_name: 'Edit', success: 2, error: 1 }),
    ])
    expect(screen.getByText('Read')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('Edit')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('shows the success/error split percentages in the tooltip', async () => {
    const user = userEvent.setup()
    renderDistribution([makeEntry({ tool_name: 'Bash', success: 3, error: 1 })])
    await user.hover(screen.getByText('Bash').closest('tr')!.querySelector('[class*="inline-flex"]')!)
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent('Success: 3 (75%)')
    expect(tooltip).toHaveTextContent('Error: 1 (25%)')
  })

  it('renders no split bar when both success and error counts are zero', () => {
    renderDistribution([makeEntry({ tool_name: 'Grep', success: 0, error: 0 })])
    expect(screen.getByText('Grep')).toBeInTheDocument()
    // Split bar wrapper (Tooltip -> span.inline-flex) is absent for a zero-total row.
    expect(screen.getByText('Grep').closest('tr')!.querySelector('[class*="inline-flex"]')).toBeNull()
  })
})
