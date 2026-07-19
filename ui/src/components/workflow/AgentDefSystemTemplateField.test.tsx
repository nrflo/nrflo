import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefSystemTemplateField } from './AgentDefSystemTemplateField'

const templates = [
  { id: 'tier-t0-decider', name: 'Tier T0 Decider', type: 'injectable', template: '', readonly: true, created_at: '', updated_at: '' },
  { id: 'tier-t1-executor', name: 'Tier T1 Executor', type: 'injectable', template: '', readonly: true, created_at: '', updated_at: '' },
]

const mockUseInjectableTemplates = vi.fn()
vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => mockUseInjectableTemplates(),
}))

function getDropdownButton() {
  const label = screen.getByText('System template')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

describe('AgentDefSystemTemplateField', () => {
  it('lists the default option plus one per injectable template', async () => {
    mockUseInjectableTemplates.mockReturnValue({ data: templates })
    const user = userEvent.setup()
    render(<AgentDefSystemTemplateField value="" onChange={vi.fn()} />)

    expect(getDropdownButton().textContent).toContain('Default (global rules)')
    await user.click(getDropdownButton())
    expect(screen.getByText('Tier T0 Decider')).toBeInTheDocument()
    expect(screen.getByText('Tier T1 Executor')).toBeInTheDocument()
  })

  it('emits the selected template id on change', async () => {
    mockUseInjectableTemplates.mockReturnValue({ data: templates })
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<AgentDefSystemTemplateField value="" onChange={onChange} />)

    await user.click(getDropdownButton())
    await user.click(screen.getByText('Tier T1 Executor'))
    expect(onChange).toHaveBeenCalledWith('tier-t1-executor')
  })

  it('emits an empty string when the default option is chosen', async () => {
    mockUseInjectableTemplates.mockReturnValue({ data: templates })
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<AgentDefSystemTemplateField value="tier-t0-decider" onChange={onChange} />)

    await user.click(getDropdownButton())
    await user.click(screen.getByText('Default (global rules)'))
    expect(onChange).toHaveBeenCalledWith('')
  })

  it('renders only the default option when no templates are loaded', () => {
    mockUseInjectableTemplates.mockReturnValue({ data: undefined })
    render(<AgentDefSystemTemplateField value="" onChange={vi.fn()} />)
    expect(getDropdownButton().textContent).toBe('Default (global rules)')
  })
})
