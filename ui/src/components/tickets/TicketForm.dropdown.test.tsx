import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TicketForm, type ParentOption } from './TicketForm'

function renderForm(props: {
  onSubmit?: (data: unknown) => Promise<void>
  parentOptions?: ParentOption[]
  defaultValues?: Record<string, unknown>
}) {
  const onSubmit = props.onSubmit ?? vi.fn().mockResolvedValue(undefined)
  render(
    <TicketForm
      onSubmit={onSubmit as never}
      isSubmitting={false}
      parentOptions={props.parentOptions}
      defaultValues={props.defaultValues as never}
    />
  )
  return { onSubmit }
}

async function fillTitle(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Title'), 'Test ticket')
}

async function submit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: /create ticket/i }))
}

/** Click the dropdown whose trigger contains the given label text */
async function openDropdown(user: ReturnType<typeof userEvent.setup>, currentLabel: string) {
  const btn = screen.getByText(currentLabel).closest('button')!
  await user.click(btn)
}

describe('TicketForm - Dropdown (Controller) integration', () => {
  describe('Type dropdown', () => {
    it('renders all 4 type options when opened', async () => {
      const user = userEvent.setup()
      renderForm({})

      // Default is "task"
      await openDropdown(user, 'Task')

      expect(screen.getByText('Bug')).toBeInTheDocument()
      expect(screen.getByText('Feature')).toBeInTheDocument()
      expect(screen.getByText('Epic')).toBeInTheDocument()
    })

    it('defaults to Task type', () => {
      renderForm({})
      // Dropdown button shows the selected label
      expect(screen.getByText('Task')).toBeInTheDocument()
    })

    it('submits selected type value via Controller', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit })

      await fillTitle(user)
      await openDropdown(user, 'Task')
      await user.click(screen.getByText('Bug'))
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      expect(onSubmit.mock.calls[0][0].issue_type).toBe('bug')
    })

    it('submits "feature" type when selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit })

      await fillTitle(user)
      await openDropdown(user, 'Task')
      await user.click(screen.getByText('Feature'))
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      expect(onSubmit.mock.calls[0][0].issue_type).toBe('feature')
    })
  })

  describe('Priority dropdown', () => {
    it('defaults to 2 - High', () => {
      renderForm({})
      expect(screen.getByText('2 - High')).toBeInTheDocument()
    })

    it('renders all 4 priority options when opened', async () => {
      const user = userEvent.setup()
      renderForm({})

      await openDropdown(user, '2 - High')

      expect(screen.getByText('1 - Critical')).toBeInTheDocument()
      expect(screen.getByText('3 - Medium')).toBeInTheDocument()
      expect(screen.getByText('4 - Low')).toBeInTheDocument()
    })

    it('coerces selected string priority to number on submit (zod coerce)', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit })

      await fillTitle(user)
      await openDropdown(user, '2 - High')
      await user.click(screen.getByText('1 - Critical'))
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      // z.coerce.number() converts string "1" → number 1
      expect(onSubmit.mock.calls[0][0].priority).toBe(1)
      expect(typeof onSubmit.mock.calls[0][0].priority).toBe('number')
    })

    it('submits priority 3 as number when selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit })

      await fillTitle(user)
      await openDropdown(user, '2 - High')
      await user.click(screen.getByText('3 - Medium'))
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      expect(onSubmit.mock.calls[0][0].priority).toBe(3)
    })
  })

  describe('Parent Epic dropdown', () => {
    const parentOptions: ParentOption[] = [
      { id: 'EPIC-1', title: 'Authentication epic' },
      { id: 'EPIC-2', title: 'Dashboard epic' },
    ]

    it('does not render parent dropdown when parentOptions is empty', () => {
      renderForm({ parentOptions: [] })
      expect(screen.queryByText('Parent Epic')).not.toBeInTheDocument()
    })

    it('does not render parent dropdown when parentOptions is undefined', () => {
      renderForm({})
      expect(screen.queryByText('Parent Epic')).not.toBeInTheDocument()
    })

    it('renders parent dropdown when parentOptions provided', () => {
      renderForm({ parentOptions })
      expect(screen.getByText('Parent Epic')).toBeInTheDocument()
    })

    it('shows None option plus all parent options when opened', async () => {
      const user = userEvent.setup()
      renderForm({ parentOptions })

      // Default is empty string → shows "None"
      await openDropdown(user, 'None')

      expect(screen.getByText('EPIC-1 - Authentication epic')).toBeInTheDocument()
      expect(screen.getByText('EPIC-2 - Dashboard epic')).toBeInTheDocument()
    })

    it('defaults to None (empty string)', async () => {
      renderForm({ parentOptions })
      // Dropdown should display "None" when value is ""
      expect(screen.getByText('None')).toBeInTheDocument()
    })

    it('submits selected parent id', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit, parentOptions })

      await fillTitle(user)
      await openDropdown(user, 'None')
      await user.click(screen.getByText('EPIC-1 - Authentication epic'))
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      expect(onSubmit.mock.calls[0][0].parent_ticket_id).toBe('EPIC-1')
    })

    it('submits empty string when None is selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn().mockResolvedValue(undefined)
      renderForm({ onSubmit, parentOptions, defaultValues: { parent_ticket_id: 'EPIC-1' } })

      await fillTitle(user)
      await openDropdown(user, 'EPIC-1 - Authentication epic')
      await user.click(screen.getAllByText('None')[0])
      await submit(user)

      await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1))
      expect(onSubmit.mock.calls[0][0].parent_ticket_id).toBe('')
    })
  })
})
