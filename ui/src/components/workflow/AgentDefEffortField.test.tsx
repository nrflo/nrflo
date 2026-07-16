import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefEffortField } from './AgentDefEffortField'

const models = [
  { id: 'sonnet-5', cli_efforts: ['low', 'medium', 'high', 'xhigh'], api_efforts: ['low', 'high'], default_effort: 'high' },
  { id: 'haiku-4-5', cli_efforts: ['low', 'medium', 'high'], api_efforts: [], default_effort: '' },
]

vi.mock('@/hooks/useModels', () => ({ useModels: () => ({ data: models }) }))

function getDropdownButton() {
  const label = screen.getByText('Reasoning Effort')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefEffortField', () => {
  it('renders nothing for execution_mode=script', () => {
    render(<AgentDefEffortField executionMode="script" model="sonnet-5" value="" onChange={vi.fn()} />)
    expect(screen.queryByText('Reasoning Effort')).not.toBeInTheDocument()
  })

  it('shows an inherit label with the selected model row effort when value is empty', () => {
    render(<AgentDefEffortField executionMode="cli_interactive" model="sonnet-5" value="" onChange={vi.fn()} />)
    expect(getDropdownButton().textContent).toContain('Inherit from model (high)')
  })

  it('shows provider default when the row has no default effort', () => {
    render(<AgentDefEffortField executionMode="cli_interactive" model="haiku-4-5" value="" onChange={vi.fn()} />)
    expect(getDropdownButton().textContent).toContain('Inherit from model (provider default)')
  })

  it('uses only the selected row mode efforts', async () => {
    const user = userEvent.setup()
    render(
      <AgentDefEffortField executionMode="api" model="sonnet-5" value="" onChange={vi.fn()} />
    )
    await user.click(getDropdownButton())
    expect(screen.getByText('low')).toBeInTheDocument()
    expect(screen.getByText('high')).toBeInTheDocument()
    expect(screen.queryByText('medium')).not.toBeInTheDocument()
    expect(screen.queryByText('xhigh')).not.toBeInTheDocument()
  })
})
