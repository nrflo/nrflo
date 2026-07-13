import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefEffortField } from './AgentDefEffortField'

const cliModels = [
  { id: 'sonnet', cli_type: 'claude', display_name: 'Sonnet', mapped_model: 'claude-sonnet-5', reasoning_effort: 'high' },
  { id: 'haiku', cli_type: 'claude', display_name: 'Haiku', mapped_model: 'claude-haiku-4-5', reasoning_effort: '' },
]
const apiModels = [
  { id: 'anthropic-sonnet', provider: 'anthropic', display_name: 'Sonnet', mapped_model: 'claude-sonnet-5', reasoning_effort: 'medium' },
]

vi.mock('@/hooks/useCLIModels', () => ({ useCLIModels: () => ({ data: cliModels }) }))
vi.mock('@/hooks/useAPIModels', () => ({ useAPIModels: () => ({ data: apiModels }) }))

function getDropdownButton() {
  const label = screen.getByText('Reasoning Effort')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefEffortField', () => {
  it('renders nothing for execution_mode=script', () => {
    render(<AgentDefEffortField executionMode="script" model="sonnet" value="" onChange={vi.fn()} />)
    expect(screen.queryByText('Reasoning Effort')).not.toBeInTheDocument()
  })

  it('shows an inherit label with the selected model row effort when value is empty', () => {
    render(<AgentDefEffortField executionMode="cli_interactive" model="sonnet" value="" onChange={vi.fn()} />)
    expect(getDropdownButton().textContent).toContain('Inherit from model (high)')
  })

  it('shows "none" in the inherit label when the model row has no effort', () => {
    render(<AgentDefEffortField executionMode="cli_interactive" model="haiku" value="" onChange={vi.fn()} />)
    expect(getDropdownButton().textContent).toContain('Inherit from model (none)')
  })

  it('gates xhigh according to the SELECTED model row: sonnet enables it, haiku disables it', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <AgentDefEffortField executionMode="cli_interactive" model="sonnet" value="" onChange={vi.fn()} />
    )
    await user.click(getDropdownButton())
    let dropdownContainer = getDropdownButton().closest('.relative')!
    const enabledXHigh = Array.from(dropdownContainer.querySelectorAll('.cursor-pointer span')).find(
      (el) => el.textContent?.startsWith('Extra High')
    )
    expect(enabledXHigh).toBeDefined()
    await user.click(getDropdownButton())

    rerender(<AgentDefEffortField executionMode="cli_interactive" model="haiku" value="" onChange={vi.fn()} />)
    await user.click(getDropdownButton())
    dropdownContainer = getDropdownButton().closest('.relative')!
    const disabledXHigh = Array.from(dropdownContainer.querySelectorAll('.cursor-not-allowed span')).find(
      (el) => el.textContent?.startsWith('Extra High')
    )
    expect(disabledXHigh).toBeDefined()
    expect(
      Array.from(dropdownContainer.querySelectorAll('.cursor-pointer span')).find((el) =>
        el.textContent?.startsWith('Extra High')
      )
    ).toBeUndefined()
  })

  it('uses the API model row when executionMode is api', () => {
    render(
      <AgentDefEffortField executionMode="api" model="anthropic-sonnet" value="" onChange={vi.fn()} />
    )
    expect(getDropdownButton().textContent).toContain('Inherit from model (medium)')
  })
})
