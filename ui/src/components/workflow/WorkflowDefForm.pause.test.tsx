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

describe('WorkflowDefForm — Pause section', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
  })

  describe('section renders', () => {
    it('renders Pause event hook heading', () => {
      renderForm()
      expect(screen.getByText('Pause event hook')).toBeInTheDocument()
    })

    it('renders On pause slot with Command and Script buttons', () => {
      renderForm()
      expect(screen.getByText('On pause')).toBeInTheDocument()
    })

    it('pause command input is not visible before toggling', () => {
      renderForm()
      expect(screen.queryByPlaceholderText(/shell command to run when workflow pauses/i)).not.toBeInTheDocument()
    })
  })

  describe('command mode toggle', () => {
    it('shows pause command input when pause Command button clicked', async () => {
      const user = userEvent.setup()
      renderForm()

      // The 3rd Command button is for the pause slot
      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(commandBtns[2])

      expect(screen.getByPlaceholderText(/shell command to run when workflow pauses/i)).toBeInTheDocument()
    })

    it('hides pause command input when Command button clicked again (toggle off)', async () => {
      const user = userEvent.setup()
      renderForm()

      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(commandBtns[2]) // on
      await user.click(commandBtns[2]) // off

      expect(screen.queryByPlaceholderText(/shell command to run when workflow pauses/i)).not.toBeInTheDocument()
    })

    it('switching from command to script clears command input', async () => {
      const user = userEvent.setup()
      renderForm()

      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      const scriptBtns = screen.getAllByRole('button', { name: /^script$/i })

      await user.click(commandBtns[2])
      await user.type(screen.getByPlaceholderText(/shell command to run when workflow pauses/i), 'make pause-hook')

      await user.click(scriptBtns[2]) // switch to script
      expect(screen.queryByPlaceholderText(/shell command to run when workflow pauses/i)).not.toBeInTheDocument()
    })
  })

  describe('submit payload — command mode', () => {
    it('includes pause_event_command when set', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')

      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      await user.click(commandBtns[2])
      await user.type(screen.getByPlaceholderText(/shell command to run when workflow pauses/i), 'make pause-hook')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.pause_event_command).toBe('make pause-hook')
      expect(call.pause_event_script_id).toBeUndefined()
    })

    it('omits pause_event fields when no mode selected', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')
      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefCreateRequest
      expect(call.pause_event_command).toBeUndefined()
      expect(call.pause_event_script_id).toBeUndefined()
    })
  })

  describe('edit — hydrates from initial', () => {
    it('initializes pause command mode and value from initial', () => {
      renderForm({
        isCreate: false,
        initial: { id: 'feature', pause_event_command: 'make on-pause' },
      })
      expect(screen.getByDisplayValue('make on-pause')).toBeInTheDocument()
    })

    it('update payload includes pause_event_command from initial', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'feature', pause_event_command: 'make on-pause' },
        onSubmit,
      })

      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefUpdateRequest
      expect(call.pause_event_command).toBe('make on-pause')
    })

    it('clears pause_event_command when mode toggled off in edit', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({
        isCreate: false,
        initial: { id: 'feature', pause_event_command: 'make on-pause' },
        onSubmit,
      })

      const commandBtns = screen.getAllByRole('button', { name: /^command$/i })
      // Find the active (default-variant) pause command button and click to toggle off
      await user.click(commandBtns[2])

      await user.click(screen.getByRole('button', { name: /submit/i }))

      const call = onSubmit.mock.calls[0][0] as WorkflowDefUpdateRequest
      expect(call.pause_event_command).toBeUndefined()
    })
  })
})
