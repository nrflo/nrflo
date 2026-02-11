import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TicketForm } from './TicketForm'

function renderForm(props: { onSubmit?: (data: unknown) => Promise<void>; isEdit?: boolean }) {
  const onSubmit = props.onSubmit ?? vi.fn().mockResolvedValue(undefined)
  render(<TicketForm onSubmit={onSubmit as never} isSubmitting={false} isEdit={props.isEdit} />)
  return { onSubmit }
}

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Title'), 'Test ticket')
  // created_by has default value 'ui' from defaultValues, no need to type it
}

describe('TicketForm', () => {
  it('submits successfully with empty ID field', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({})

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    const data = (onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(data.id).toBe('')
  })

  it('submits successfully with a custom ID', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({})

    await user.type(screen.getByLabelText('Ticket ID'), 'PROJ-123')
    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    const data = (onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(data.id).toBe('PROJ-123')
  })

  it('does not show ID validation error when ID is empty', async () => {
    const user = userEvent.setup()
    renderForm({})

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(screen.queryByText(/id is required/i)).not.toBeInTheDocument()
    })
  })

  it('shows validation error when title is empty', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({})

    // Don't fill title, just submit
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(screen.getByText('Title is required')).toBeInTheDocument()
    })
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('disables ID field in edit mode', () => {
    renderForm({ isEdit: true })
    expect(screen.getByLabelText('Ticket ID')).toBeDisabled()
  })

  it('includes default values for priority and issue_type', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderForm({})

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: /create ticket/i }))

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })
    const data = (onSubmit as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(data.priority).toBe(2)
    expect(data.issue_type).toBe('task')
    expect(data.created_by).toBe('ui')
  })
})
