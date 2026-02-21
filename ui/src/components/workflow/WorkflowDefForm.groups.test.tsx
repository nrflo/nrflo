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
    onCancel: vi.fn(),
    isPending: false,
    ...props,
  }
  return {
    ...render(<WorkflowDefForm {...defaultProps} />),
    props: defaultProps,
  }
}

const groupInput = () => screen.getByPlaceholderText(/type a tag and press enter/i)

/** Chip remove buttons use aria-label; PhaseListEditor uses title. This distinguishes them. */
const chipRemoveButtons = () =>
  Array.from(document.querySelectorAll('button[aria-label^="Remove "]'))

describe('WorkflowDefForm - groups chip input', () => {
  describe('adding chips', () => {
    it('adds chip on Enter key', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')

      expect(screen.getByText('be')).toBeInTheDocument()
      expect(groupInput()).toHaveValue('')
    })

    it('adds chip on comma key', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'fe,')

      expect(screen.getByText('fe')).toBeInTheDocument()
      expect(groupInput()).toHaveValue('')
    })

    it('adds multiple distinct chips', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')
      await user.type(groupInput(), 'fe')
      await user.keyboard('{Enter}')

      expect(screen.getByText('be')).toBeInTheDocument()
      expect(screen.getByText('fe')).toBeInTheDocument()
    })

    it('lowercases tag before adding', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'BE')
      await user.keyboard('{Enter}')

      expect(screen.getByText('be')).toBeInTheDocument()
      expect(screen.queryByText('BE')).not.toBeInTheDocument()
    })

    it('allows hyphens in tags', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'my-tag')
      await user.keyboard('{Enter}')

      expect(screen.getByText('my-tag')).toBeInTheDocument()
    })

    it('does not add empty tag', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), '   ')
      await user.keyboard('{Enter}')

      // Chip remove buttons use aria-label="Remove <tag>"; none should exist
      expect(chipRemoveButtons()).toHaveLength(0)
    })

    it('does not add duplicate tag', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')
      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')

      expect(screen.getAllByText('be')).toHaveLength(1)
    })

    it('rejects tag with special characters', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be@tag')
      await user.keyboard('{Enter}')

      expect(chipRemoveButtons()).toHaveLength(0)
    })
  })

  describe('removing chips', () => {
    it('removes chip on X button click', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')
      expect(screen.getByText('be')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /remove be/i }))

      expect(screen.queryByText('be')).not.toBeInTheDocument()
    })

    it('removes only the clicked chip when multiple exist', async () => {
      const user = userEvent.setup()
      renderForm()

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')
      await user.type(groupInput(), 'fe')
      await user.keyboard('{Enter}')

      await user.click(screen.getByRole('button', { name: /remove be/i }))

      expect(screen.queryByText('be')).not.toBeInTheDocument()
      expect(screen.getByText('fe')).toBeInTheDocument()
    })
  })

  describe('initial population', () => {
    it('pre-populates chips from initial.groups', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', groups: ['be', 'fe', 'docs'] },
      })

      expect(screen.getByText('be')).toBeInTheDocument()
      expect(screen.getByText('fe')).toBeInTheDocument()
      expect(screen.getByText('docs')).toBeInTheDocument()
    })

    it('shows no chips when initial.groups is empty', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', groups: [] },
      })

      expect(chipRemoveButtons()).toHaveLength(0)
    })

    it('shows remove buttons for pre-populated chips', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', groups: ['be', 'fe'] },
      })

      expect(screen.getByRole('button', { name: /remove be/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /remove fe/i })).toBeInTheDocument()
    })
  })

  describe('form submission', () => {
    it('includes added chips in create submission', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-workflow')
      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')
      await user.type(groupInput(), 'fe')
      await user.keyboard('{Enter}')

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'analyzer')
      await user.click(screen.getByRole('button', { name: /create workflow/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ groups: ['be', 'fe'] })
      )
    })

    it('Enter in tag input does not submit the form', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(groupInput(), 'be')
      await user.keyboard('{Enter}')

      expect(onSubmit).not.toHaveBeenCalled()
    })

    it('includes pre-populated groups in update submission', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: {
          id: 'feature',
          groups: ['be', 'fe'],
          phases: [{ id: 'analyzer', agent: 'analyzer', layer: 0 }],
        },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /save changes/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ groups: ['be', 'fe'] })
      )
    })
  })
})
