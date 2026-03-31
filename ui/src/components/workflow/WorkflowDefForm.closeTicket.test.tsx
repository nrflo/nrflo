import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'
import type { WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

function renderForm(
  props: Partial<React.ComponentProps<typeof WorkflowDefForm>> = {}
) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    isPending: false,
    ...props,
  }
  return { ...render(<WorkflowDefForm {...defaultProps} />), props: defaultProps }
}

describe('WorkflowDefForm – close_ticket_on_complete checkbox', () => {
  describe('visibility', () => {
    it('shows checkbox when scope is ticket (default)', () => {
      renderForm({ isCreate: true })
      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).toBeInTheDocument()
    })

    it('hides checkbox when scope is project', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await user.click(screen.getByRole('button', { name: /^project$/i }))

      expect(screen.queryByRole('checkbox', { name: /close ticket after workflow finished/i })).not.toBeInTheDocument()
    })

    it('re-shows checkbox when scope toggled back to ticket', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await user.click(screen.getByRole('button', { name: /^project$/i }))
      expect(screen.queryByRole('checkbox', { name: /close ticket after workflow finished/i })).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /^ticket$/i }))
      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).toBeInTheDocument()
    })
  })

  describe('default state', () => {
    it('is checked by default on create (no initial prop)', () => {
      renderForm({ isCreate: true })
      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).toBeChecked()
    })

    it('respects initial close_ticket_on_complete=false', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', close_ticket_on_complete: false },
      })
      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).not.toBeChecked()
    })

    it('respects initial close_ticket_on_complete=true', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', close_ticket_on_complete: true },
      })
      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).toBeChecked()
    })
  })

  describe('scope toggle state retention', () => {
    it('retains unchecked state when toggling scope ticket→project→ticket', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const checkbox = screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })
      await user.click(checkbox) // uncheck (was true by default)

      await user.click(screen.getByRole('button', { name: /^project$/i }))
      await user.click(screen.getByRole('button', { name: /^ticket$/i }))

      expect(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i })).not.toBeChecked()
    })
  })

  describe('submit payload', () => {
    it('includes close_ticket_on_complete:false in create request when unchecked', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-workflow')
      await user.type(screen.getAllByPlaceholderText(/agent type/i)[0], 'implementor')
      await user.click(screen.getByRole('checkbox', { name: /close ticket after workflow finished/i }))

      await user.click(screen.getByRole('button', { name: /create workflow/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.close_ticket_on_complete).toBe(false)
    })

    it('includes close_ticket_on_complete:false in update request when unchecked', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'feature', close_ticket_on_complete: false, phases: [{ id: 'analyzer', agent: 'analyzer', layer: 0 }] },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /save changes/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefUpdateRequest
      expect(call.close_ticket_on_complete).toBe(false)
    })
  })
})
