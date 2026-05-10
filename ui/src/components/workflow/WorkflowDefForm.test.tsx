import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import type { WorkflowDefSummary } from '@/types/workflow'

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

const makeWorkflowDef = (overrides: Partial<WorkflowDefSummary> = {}): WorkflowDefSummary => ({
  description: '',
  scope_type: 'project',
  phases: [],
  ...overrides,
})

function renderForm(props: Partial<React.ComponentProps<typeof WorkflowDefForm>> = {}) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    formId: 'test-form',
    ...props,
  }
  return {
    ...renderWithQuery(
      <>
        <WorkflowDefForm {...defaultProps} />
        <button type="submit" form="test-form">Submit</button>
      </>
    ),
    props: defaultProps,
  }
}

describe('WorkflowDefForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
  })

  describe('form submission', () => {
    it('submits WorkflowDefCreateRequest in create mode', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'feature')
      await user.type(screen.getByPlaceholderText(/short description/i), 'Full TDD workflow')
      await user.click(screen.getByRole('button', { name: /submit/i }))

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
      renderForm({ isCreate: false, initial: { id: 'feature', description: 'Old desc' }, onSubmit })

      const descInput = screen.getByDisplayValue('Old desc')
      await user.clear(descInput)
      await user.type(descInput, 'New description')
      await user.click(screen.getByRole('button', { name: /submit/i }))

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
      expect(screen.getByPlaceholderText(/e.g., feature/i)).toBeRequired()
    })

    it('does not show workflow ID field in update mode', () => {
      renderForm({ isCreate: false, initial: { id: 'feature' } })
      expect(screen.queryByPlaceholderText(/e.g., feature/i)).not.toBeInTheDocument()
    })
  })

  describe('scope_type toggle', () => {
    it('defaults to ticket scope', () => {
      renderForm({ isCreate: true })
      expect(screen.getByRole('button', { name: /^ticket$/i })).toHaveClass('border-primary')
    })

    it('toggles to project scope', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      await user.click(screen.getByRole('button', { name: /^project$/i }))

      expect(screen.getByRole('button', { name: /^project$/i })).toHaveClass('border-primary')
      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()
    })

    it('includes scope_type in create request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'project-workflow')
      await user.click(screen.getByRole('button', { name: /^project$/i }))
      await user.click(screen.getByRole('button', { name: /submit/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'project-workflow', scope_type: 'project' })
      )
    })

    it('includes scope_type in update request', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: false, initial: { id: 'test', scope_type: 'ticket' }, onSubmit })

      await user.click(screen.getByRole('button', { name: /^project$/i }))
      await user.click(screen.getByRole('button', { name: /submit/i }))

      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ scope_type: 'project' }))
    })

    it('shows info text only when project scope is selected', async () => {
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      expect(screen.queryByText(/project workflows run without a ticket/i)).not.toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /^project$/i }))
      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: /^ticket$/i }))
      expect(screen.queryByText(/project workflows run without a ticket/i)).not.toBeInTheDocument()
    })

    it('respects initial scope_type from props', () => {
      renderForm({ isCreate: false, initial: { id: 'test', scope_type: 'project' } })

      expect(screen.getByRole('button', { name: /^project$/i })).toHaveClass('border-primary')
      expect(screen.getByText(/project workflows run without a ticket/i)).toBeInTheDocument()
    })
  })

  describe('next_workflow_on_success', () => {
    it('disables checkbox and shows hint when no project workflows available', () => {
      // query resolves to {} (empty) — checkbox stays disabled in both pre/post-resolve states
      renderForm({ isCreate: true })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      expect(checkbox).toBeDisabled()
      expect(screen.getByText(/create a project-scoped workflow first/i)).toBeInTheDocument()
    })

    it('omits next_workflow_on_success from payload when checkbox is unchecked', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'myflow')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.not.objectContaining({ next_workflow_on_success: expect.anything() })
      )
    })

    it('enables checkbox when project workflows are available', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        wf1: makeWorkflowDef({ description: 'A' }),
        wf2: makeWorkflowDef(),
      })
      renderForm({ isCreate: true })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
    })

    it('selects first project workflow on checkbox toggle and shows dropdown', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        wf1: makeWorkflowDef({ description: 'A' }),
        wf2: makeWorkflowDef(),
      })
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
      await user.click(checkbox)

      // Dropdown trigger shows first option (wf1) selected
      expect(screen.getByRole('button', { name: /wf1/i })).toBeInTheDocument()
    })

    it('includes next_workflow_on_success in payload when checked', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        wf1: makeWorkflowDef({ description: 'A' }),
        wf2: makeWorkflowDef(),
      })
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
      await user.click(checkbox)

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'myflow')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ next_workflow_on_success: 'wf1' })
      )
    })

    it('clears next_workflow_on_success when checkbox is unchecked after being set', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({ wf1: makeWorkflowDef() })
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'myflow', next_workflow_on_success: 'wf1' },
        onSubmit,
      })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
      expect(checkbox).toBeChecked()

      await user.click(checkbox)
      await user.click(screen.getByRole('button', { name: /submit/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.not.objectContaining({ next_workflow_on_success: expect.anything() })
      )
    })

    it('excludes self from dropdown options in edit mode', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        wf1: makeWorkflowDef(),
        wf2: makeWorkflowDef(),
      })
      const user = userEvent.setup()
      renderForm({ isCreate: false, initial: { id: 'wf1' } })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
      await user.click(checkbox)

      // wf2 auto-selected (only available option after self-exclusion)
      expect(screen.getByRole('button', { name: /wf2/i })).toBeInTheDocument()
      // wf1 excluded from all options
      expect(screen.queryByText('wf1')).not.toBeInTheDocument()
    })

    it('filters out ticket-scoped workflows from options', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        ticket1: makeWorkflowDef({ scope_type: 'ticket' }),
        project1: makeWorkflowDef({ scope_type: 'project' }),
      })
      const user = userEvent.setup()
      renderForm({ isCreate: true })

      const checkbox = screen.getByRole('checkbox', { name: /run another workflow on success/i })
      await waitFor(() => expect(checkbox).toBeEnabled())
      await user.click(checkbox)

      expect(screen.getByRole('button', { name: /project1/i })).toBeInTheDocument()
      expect(screen.queryByText('ticket1')).not.toBeInTheDocument()
    })

    it('disables checkbox when only ticket-scoped workflows are returned', async () => {
      vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({
        ticket1: makeWorkflowDef({ scope_type: 'ticket' }),
      })
      renderForm({ isCreate: true })

      // After query resolves, projectWorkflowOptions is still [] (all filtered out) → stays disabled
      await waitFor(() => {
        expect(screen.getByRole('checkbox', { name: /run another workflow on success/i })).toBeDisabled()
      })
      expect(screen.getByText(/create a project-scoped workflow first/i)).toBeInTheDocument()
    })
  })
})
