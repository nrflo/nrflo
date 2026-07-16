import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { WorkflowDefForm } from './WorkflowDefForm'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'
import type { WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/hooks/useModels', () => ({
  cliTypeForProvider: vi.fn(),
  useModels: () => ({ data: [] }),
}))

vi.mock('@/hooks/usePythonScripts', () => ({
  usePythonScripts: vi.fn(() => ({ data: [], isLoading: false })),
}))

function renderForm(props: Partial<React.ComponentProps<typeof WorkflowDefForm>> = {}) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    formId: 'test-form',
    ...props,
  }
  return {
    ...renderWithQuery(
      <MemoryRouter>
        <WorkflowDefForm {...defaultProps} />
        <button type="submit" form="test-form">Submit</button>
      </MemoryRouter>
    ),
    props: defaultProps,
  }
}

describe('WorkflowDefForm — Finalize section', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
  })

  describe('section renders', () => {
    it('renders Finalize section heading', () => {
      renderForm()
      expect(screen.getByText('Finalize')).toBeInTheDocument()
    })

    it('renders both On success and On failure slots', () => {
      renderForm()
      expect(screen.getByText('On success')).toBeInTheDocument()
      expect(screen.getByText('On failure')).toBeInTheDocument()
    })

    it('each slot has Command and Script buttons', () => {
      renderForm()
      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      const scriptBtns = screen.getAllByRole('button', { name: /^script$/i })
      expect(commandBtns).toHaveLength(3)
      expect(scriptBtns).toHaveLength(3)
    })

    it('command input is not visible before toggling command mode on', () => {
      renderForm()
      expect(screen.queryByPlaceholderText(/shell command/i)).not.toBeInTheDocument()
    })
  })

  describe('command mode toggle', () => {
    it('shows command input when Command button clicked for success slot', async () => {
      const user = userEvent.setup()
      renderForm()
      const [successCommandBtn] = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(successCommandBtn)
      expect(screen.getByPlaceholderText(/shell command/i)).toBeInTheDocument()
    })

    it('hides command input when Command button clicked again (toggle off)', async () => {
      const user = userEvent.setup()
      renderForm()
      const [successCommandBtn] = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(successCommandBtn) // on
      await user.click(successCommandBtn) // off
      expect(screen.queryByPlaceholderText(/shell command/i)).not.toBeInTheDocument()
    })

    it('switching from command to script clears command input', async () => {
      const user = userEvent.setup()
      renderForm()
      const [successCommandBtn] = screen.getAllByRole('button', { name: /^command$/i })
      const [successScriptBtn] = screen.getAllByRole('button', { name: /^script$/i })

      await user.click(successCommandBtn)
      await user.type(screen.getByPlaceholderText(/shell command/i), 'make deploy')

      await user.click(successScriptBtn) // switch to script mode
      // command input should be gone
      expect(screen.queryByPlaceholderText(/shell command/i)).not.toBeInTheDocument()
    })
  })

  describe('submit payload — command mode', () => {
    it('includes finalize_success_command when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')

      const [successCommandBtn] = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(successCommandBtn)
      await user.type(screen.getByPlaceholderText(/shell command/i), 'make deploy')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.finalize_success_command).toBe('make deploy')
      expect(call.finalize_success_script_id).toBeUndefined()
    })

    it('includes finalize_failure_command when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')

      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(commandBtns[1]) // failure slot
      await user.type(screen.getByPlaceholderText(/shell command/i), 'make notify')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.finalize_failure_command).toBe('make notify')
      expect(call.finalize_failure_script_id).toBeUndefined()
    })

    it('omits finalize fields when no mode selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.finalize_success_command).toBeUndefined()
      expect(call.finalize_success_script_id).toBeUndefined()
      expect(call.finalize_failure_command).toBeUndefined()
      expect(call.finalize_failure_script_id).toBeUndefined()
    })
  })

  describe('edit — hydrates from initial', () => {
    it('initializes success command mode and value from initial', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', finalize_success_command: 'make release' },
      })
      // Command input should be visible with value
      expect(screen.getByDisplayValue('make release')).toBeInTheDocument()
    })

    it('initializes failure command mode and value from initial', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', finalize_failure_command: 'make rollback' },
      })
      expect(screen.getByDisplayValue('make rollback')).toBeInTheDocument()
    })

    it('update payload includes finalize fields from initial', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'feature', finalize_success_command: 'make release' },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefUpdateRequest
      expect(call.finalize_success_command).toBe('make release')
    })

    it('clears finalize fields when mode toggled off in edit', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'feature', finalize_success_command: 'make release' },
        onSubmit,
      })

      const [successCommandBtn] = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(successCommandBtn) // toggle off

      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefUpdateRequest
      expect(call.finalize_success_command).toBeUndefined()
    })
  })
})
