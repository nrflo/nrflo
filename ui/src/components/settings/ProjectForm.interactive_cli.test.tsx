import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectForm, emptyProjectForm } from './ProjectForm'

function makeMutation(overrides = {}) {
  return { isPending: false, isError: false, error: null, ...overrides }
}

function makeProps(formDataOverrides = {}) {
  return {
    formData: { ...emptyProjectForm, ...formDataOverrides },
    setFormData: vi.fn(),
    onCancel: vi.fn(),
    onSave: vi.fn(),
    mutation: makeMutation(),
  }
}

describe('ProjectForm — interactive CLI mode toggle', () => {
  it('reflects checked=true when interactive_cli_mode is true', () => {
    render(<ProjectForm {...makeProps({ interactive_cli_mode: true })} />)
    expect(
      screen.getByRole('switch', { name: /interactive cli mode/i })
    ).toHaveAttribute('aria-checked', 'true')
  })

  it('reflects checked=false when interactive_cli_mode is false', () => {
    render(<ProjectForm {...makeProps({ interactive_cli_mode: false })} />)
    expect(
      screen.getByRole('switch', { name: /interactive cli mode/i })
    ).toHaveAttribute('aria-checked', 'false')
  })

  it('is enabled regardless of default_branch value', () => {
    render(<ProjectForm {...makeProps({ default_branch: '', interactive_cli_mode: false })} />)
    expect(screen.getByRole('switch', { name: /interactive cli mode/i })).not.toBeDisabled()
  })

  it('calls setFormData with interactive_cli_mode toggled on click', async () => {
    const user = userEvent.setup()
    const props = makeProps({ interactive_cli_mode: false })
    render(<ProjectForm {...props} />)
    await user.click(screen.getByRole('switch', { name: /interactive cli mode/i }))
    expect(props.setFormData).toHaveBeenCalledWith(
      expect.objectContaining({ interactive_cli_mode: true })
    )
  })

  it('shows PTY mode tooltip on hover', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps()} />)
    await user.hover(screen.getByText('Interactive CLI mode'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/PTY mode/)
  })
})
