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

describe('ProjectForm — worktree tooltip', () => {
  it('shows worktree explanation tooltip on hover', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    expect(screen.queryByText(/Git worktrees give each ticket/)).not.toBeInTheDocument()

    await user.hover(screen.getByText('Use Git Worktrees'))

    expect(screen.getByText(/Git worktrees give each ticket/)).toBeInTheDocument()
    expect(screen.getByText(/Applies to ticket-scoped workflows only/)).toBeInTheDocument()
    expect(screen.getByText(/Lifecycle: creates a feature branch/)).toBeInTheDocument()
  })

  it('hides tooltip on mouse leave', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    const trigger = screen.getByText('Use Git Worktrees')
    await user.hover(trigger)
    expect(screen.getByText(/Git worktrees give each ticket/)).toBeInTheDocument()

    await user.unhover(trigger)
    expect(screen.queryByText(/Git worktrees give each ticket/)).not.toBeInTheDocument()
  })

  it('tooltip applies whitespace-normal max-w-sm classes', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    await user.hover(screen.getByText('Use Git Worktrees'))

    const tooltip = document.body.querySelector('.whitespace-normal.max-w-sm')
    expect(tooltip).toBeInTheDocument()
  })
})

describe('ProjectForm — worktree toggle state', () => {
  it('disables toggle when default_branch is empty', () => {
    render(<ProjectForm {...makeProps({ default_branch: '' })} />)

    const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
    expect(toggle).toBeDisabled()
  })

  it('enables toggle when default_branch is set', () => {
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
    expect(toggle).not.toBeDisabled()
  })
})
