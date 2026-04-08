import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'

function renderForm(
  props: Partial<React.ComponentProps<typeof WorkflowDefForm>> = {}
) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    formId: 'test-form',
    ...props,
  }
  return {
    ...render(
      <>
        <WorkflowDefForm {...defaultProps} />
        <button type="submit" form="test-form">Submit</button>
      </>
    ),
    props: defaultProps,
  }
}

describe('WorkflowDefForm', () => {
  describe('form submission', () => {
    it('submits WorkflowDefCreateRequest in create mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'feature')
      await user.type(screen.getByPlaceholderText(/short description/i), 'Full TDD workflow')

      const submitButton = screen.getByRole('button', { name: /submit/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        id: 'feature',
        description: 'Full TDD workflow',
        scope_type: 'ticket',
        groups: [],
        close_ticket_on_complete: true,
      })
    })

    it('submits WorkflowDefUpdateRequest in update mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()

      renderForm({
        isCreate: false,
        initial: { id: 'feature', description: 'Old desc' },
        onSubmit,
      })

      const descInput = screen.getByDisplayValue('Old desc')
      await user.clear(descInput)
      await user.type(descInput, 'New description')

      const submitButton = screen.getByRole('button', { name: /submit/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        description: 'New description',
        scope_type: 'ticket',
        groups: [],
        close_ticket_on_complete: true,
      })
    })

  })

  describe('form validation and UI', () => {
    it('requires workflow ID in create mode', () => {
      renderForm({ isCreate: true })

      const idInput = screen.getByPlaceholderText(/e.g., feature/i)
      expect(idInput).toBeRequired()
    })

    it('does not show workflow ID field in update mode', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature' },
      })

      expect(screen.queryByPlaceholderText(/e.g., feature/i)).not.toBeInTheDocument()
    })

  })

  describe('scope_type toggle', () => {
    it('defaults to ticket scope', () => {
      renderForm({ isCreate: true })

      const ticketButton = screen.getByRole('button', { name: /^ticket$/i })
      expect(ticketButton).toHaveClass('border-primary')
    })

    it('toggles to project scope', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      await user.click(projectButton)

      expect(projectButton).toHaveClass('border-primary')
      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()
    })

    it('includes scope_type in create request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'project-workflow')

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      await user.click(projectButton)

      const submitButton = screen.getByRole('button', { name: /submit/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'project-workflow',
          scope_type: 'project',
        })
      )
    })

    it('includes scope_type in update request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'test', scope_type: 'ticket' },
        onSubmit,
      })

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      await user.click(projectButton)

      const submitButton = screen.getByRole('button', { name: /submit/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          scope_type: 'project',
        })
      )
    })

    it('shows info text only when project scope is selected', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      expect(screen.queryByText(/project workflows run without a ticket/i)).not.toBeInTheDocument()

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      await user.click(projectButton)

      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()

      const ticketButton = screen.getByRole('button', { name: /^ticket$/i })
      await user.click(ticketButton)

      expect(screen.queryByText(/project workflows run without a ticket/i)).not.toBeInTheDocument()
    })

    it('respects initial scope_type from props', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'test', scope_type: 'project' },
      })

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      expect(projectButton).toHaveClass('border-primary')
      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()
    })
  })
})
