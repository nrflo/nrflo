import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'
import type { PhaseDef, WorkflowDefCreateRequest } from '@/types/workflow'

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

describe('WorkflowDefForm', () => {
  describe('formToPhases conversion', () => {
    it('emits object format with layer field', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      // Fill in the form
      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test-workflow')

      // The form starts with one default empty agent — fill that one
      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'setup-analyzer')

      // Submit
      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'test-workflow',
          phases: [
            {
              id: 'setup-analyzer',
              agent: 'setup-analyzer',
              layer: 0,
            },
          ],
        })
      )
    })

    it('never emits string-only phase entries', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'bugfix')

      // Fill the default agent, then add one more
      const defaultInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(defaultInputs[0], 'analyzer')

      const addButton = screen.getByRole('button', { name: /add agent/i })
      await user.click(addButton)

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[1], 'implementor')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      // Verify all phases are objects with required fields
      expect(call.phases).toHaveLength(2)
      call.phases.forEach((phase) => {
        expect(typeof phase).toBe('object')
        expect(phase).toHaveProperty('id')
        expect(phase).toHaveProperty('agent')
        expect(phase).toHaveProperty('layer')
        expect(typeof phase.layer).toBe('number')
      })
    })

    it('includes skip_for when categories are added', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'feature')

      // Fill the default agent
      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'test-writer')

      // First add 'docs' as a workflow category so it appears in the agent's skip_for buttons
      const catDocsButton = screen.getByRole('button', { name: '+docs' })
      await user.click(catDocsButton)

      // Now the agent row has a +docs button for skip_for (since 'docs' is now a category)
      // The category section no longer has +docs (it was added). Only agent row has it.
      const skipDocsButton = screen.getByRole('button', { name: '+docs' })
      await user.click(skipDocsButton)

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases[0]).toMatchObject({
        id: 'test-writer',
        agent: 'test-writer',
        layer: 0,
        skip_for: ['docs'],
      })
    })

    it('omits skip_for when empty', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'hotfix')

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'implementor')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases[0]).toMatchObject({
        id: 'implementor',
        agent: 'implementor',
        layer: 0,
      })
      expect(call.phases[0]).not.toHaveProperty('skip_for')
    })

    it('filters out empty agent entries', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'feature')

      // Fill the default (index 0), add 2 more, fill only index 2
      const defaultInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(defaultInputs[0], 'setup-analyzer')

      await user.click(screen.getByRole('button', { name: /add agent/i }))
      await user.click(screen.getByRole('button', { name: /add agent/i }))

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[2], 'implementor')

      // Remove the empty agent (index 1) so form validation passes
      const removeButtons = screen.getAllByTitle(/remove agent/i)
      await user.click(removeButtons[1])

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases).toHaveLength(2)
      expect(call.phases.map((p) => p.agent)).toEqual(['setup-analyzer', 'implementor'])
    })
  })

  describe('phasesToForm conversion', () => {
    it('handles missing layer field gracefully (defaults to 0)', () => {
      const phases: PhaseDef[] = [
        { id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 },
        // Simulate old data without layer
        { id: 'implementor', agent: 'implementor' } as PhaseDef,
      ]

      renderForm({
        isCreate: false,
        initial: { id: 'feature', phases },
      })

      const layerInputs = screen.getAllByRole('spinbutton')
      expect(layerInputs[0]).toHaveValue(0)
      expect(layerInputs[1]).toHaveValue(0) // Defaults to 0
    })

    it('populates form with existing phase data', () => {
      const phases: PhaseDef[] = [
        { id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0, skip_for: ['docs'] },
        { id: 'implementor', agent: 'implementor', layer: 1, skip_for: [] },
      ]

      renderForm({
        isCreate: false,
        initial: { id: 'feature', phases },
      })

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      expect(agentInputs[0]).toHaveValue('setup-analyzer')
      expect(agentInputs[1]).toHaveValue('implementor')

      const layerInputs = screen.getAllByRole('spinbutton')
      expect(layerInputs[0]).toHaveValue(0)
      expect(layerInputs[1]).toHaveValue(1)

      expect(screen.getByText('docs')).toBeInTheDocument()
    })

    it('defaults to single empty agent when no phases provided', () => {
      renderForm({
        isCreate: true,
        initial: { id: 'new-workflow' },
      })

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      expect(agentInputs).toHaveLength(1)
      expect(agentInputs[0]).toHaveValue('')

      const layerInputs = screen.getAllByRole('spinbutton')
      expect(layerInputs[0]).toHaveValue(0)
    })
  })

  describe('form submission', () => {
    it('submits WorkflowDefCreateRequest in create mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'feature')
      await user.type(screen.getByPlaceholderText(/short description/i), 'Full TDD workflow')

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'setup-analyzer')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        id: 'feature',
        description: 'Full TDD workflow',
        scope_type: 'ticket',
        categories: ['full'],
        phases: [
          {
            id: 'setup-analyzer',
            agent: 'setup-analyzer',
            layer: 0,
          },
        ],
      })
    })

    it('submits WorkflowDefUpdateRequest in update mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      const phases: PhaseDef[] = [
        { id: 'setup-analyzer', agent: 'setup-analyzer', layer: 0 },
      ]

      renderForm({
        isCreate: false,
        initial: { id: 'feature', description: 'Old desc', phases },
        onSubmit,
      })

      const descInput = screen.getByDisplayValue('Old desc')
      await user.clear(descInput)
      await user.type(descInput, 'New description')

      const submitButton = screen.getByRole('button', { name: /save changes/i })
      await user.click(submitButton)

      expect(onSubmit).toHaveBeenCalledWith({
        description: 'New description',
        scope_type: 'ticket',
        categories: ['full'],
        phases: [
          {
            id: 'setup-analyzer',
            agent: 'setup-analyzer',
            layer: 0,
          },
        ],
      })
    })

    it('does not submit when isPending is true', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit, isPending: true })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test')

      const submitButton = screen.getByRole('button', { name: /saving/i })
      expect(submitButton).toBeDisabled()
    })
  })

  describe('categories management', () => {
    it('starts with default category "full"', () => {
      renderForm({ isCreate: true })

      expect(screen.getByText('full')).toBeInTheDocument()
    })

    it('adds preset categories', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      // The form-level preset buttons have the format "+simple"
      // but the PhaseListEditor agent row also has "+simple" for skip_for
      // The form-level ones are in the categories section
      const simpleButtons = screen.getAllByRole('button', { name: /\+simple/i })
      // First match is the form-level category button
      await user.click(simpleButtons[0])

      expect(screen.getByText('simple')).toBeInTheDocument()
    })

    it('removes categories', async () => {
      const user = userEvent.setup()
      renderForm({
        isCreate: false,
        initial: { id: 'test', categories: ['full', 'simple'] },
      })

      const simpleBadge = screen.getByText('simple').closest('.gap-1')
      const removeButton = simpleBadge?.querySelector('button')
      await user.click(removeButton!)

      expect(screen.queryByText('simple')).not.toBeInTheDocument()
    })

    it('adds custom category via input', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const catInput = screen.getByPlaceholderText(/^custom/i)
      await user.type(catInput, 'experimental{Enter}')

      expect(screen.getByText('experimental')).toBeInTheDocument()
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

    it('shows correct button text based on mode', () => {
      const { rerender } = renderForm({ isCreate: true })
      expect(screen.getByRole('button', { name: /create workflow/i })).toBeInTheDocument()

      rerender(<WorkflowDefForm isCreate={false} onSubmit={vi.fn()} onCancel={vi.fn()} />)
      expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument()
    })

    it('calls onCancel when cancel button clicked', async () => {
      const user = userEvent.setup()
      const onCancel = vi.fn()
      renderForm({ onCancel })

      const cancelButton = screen.getByRole('button', { name: /cancel/i })
      await user.click(cancelButton)

      expect(onCancel).toHaveBeenCalledTimes(1)
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

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'analyzer')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
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
      const phases: PhaseDef[] = [
        { id: 'analyzer', agent: 'analyzer', layer: 0 },
      ]
      renderForm({
        isCreate: false,
        initial: { id: 'test', scope_type: 'ticket', phases },
        onSubmit,
      })

      const projectButton = screen.getByRole('button', { name: /^project$/i })
      await user.click(projectButton)

      const submitButton = screen.getByRole('button', { name: /save changes/i })
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

  describe('edge cases', () => {
    it('handles empty phases array submission', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'empty-workflow')

      // Remove the default empty agent so form validation passes
      const removeButton = screen.getByTitle(/remove agent/i)
      await user.click(removeButton)

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases).toHaveLength(0)
    })

    it('handles phase with multiple skip_for categories', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test')

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], 'test-writer')

      // First add 'docs' and 'simple' as workflow categories
      await user.click(screen.getByRole('button', { name: '+docs' }))
      await user.click(screen.getByRole('button', { name: '+simple' }))

      // Now add both as skip_for on the agent row
      // After adding to categories, the category preset buttons are gone,
      // only agent row skip_for buttons remain
      await user.click(screen.getByRole('button', { name: '+docs' }))
      await user.click(screen.getByRole('button', { name: '+simple' }))

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases[0].skip_for).toEqual(['docs', 'simple'])
    })

    it('trims whitespace from agent names', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'test')

      const agentInputs = screen.getAllByPlaceholderText(/agent type/i)
      await user.type(agentInputs[0], '  setup-analyzer  ')

      const submitButton = screen.getByRole('button', { name: /create workflow/i })
      await user.click(submitButton)

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.phases[0].agent).toBe('setup-analyzer')
    })
  })
})
