import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkflowDefForm } from './WorkflowDefForm'
import { renderWithQuery } from '@/test/utils'
import * as workflowApi from '@/api/workflows'

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

vi.mock('@/hooks/useCLIModels', () => ({
  useCLIModels: () => ({ data: [] }),
}))

function renderForm(props: Partial<React.ComponentProps<typeof WorkflowDefForm>> = {}) {
  const defaultProps = {
    isCreate: true,
    onSubmit: vi.fn(),
    formId: 'test-form',
    ...props,
  }
  return renderWithQuery(
    <>
      <WorkflowDefForm {...defaultProps} />
      <button type="submit" form="test-form">Submit</button>
    </>
  )
}

describe('WorkflowDefForm — finding schemas', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
  })

  it('renders the finding schemas section', () => {
    renderForm()
    expect(screen.getByText('Finding schemas')).toBeInTheDocument()
  })

  it('adds a row and submits parsed finding_schemas', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    fireEvent.change(screen.getByPlaceholderText(/e.g., feature/i), { target: { value: 'wf' } })
    await user.click(screen.getByRole('button', { name: /add finding schema/i }))
    fireEvent.change(screen.getByPlaceholderText(/finding key/i), { target: { value: 'security_issues' } })
    fireEvent.change(screen.getByPlaceholderText(/"type": "array"/i), { target: { value: '{"type":"array"}' } })
    fireEvent.change(screen.getByPlaceholderText(/"file": "a.go"/i), { target: { value: '[{"file":"a.go"}]' } })
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        finding_schemas: [
          { key: 'security_issues', schema: { type: 'array' }, example: [{ file: 'a.go' }] },
        ],
      })
    )
  })

  it('blocks submit and shows an error when schema JSON is invalid', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    fireEvent.change(screen.getByPlaceholderText(/e.g., feature/i), { target: { value: 'wf' } })
    await user.click(screen.getByRole('button', { name: /add finding schema/i }))
    fireEvent.change(screen.getByPlaceholderText(/finding key/i), { target: { value: 'k' } })
    fireEvent.change(screen.getByPlaceholderText(/"type": "array"/i), { target: { value: '{not json' } })
    fireEvent.change(screen.getByPlaceholderText(/"file": "a.go"/i), { target: { value: '[]' } })
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getAllByText(/invalid json/i).length).toBeGreaterThan(0)
  })
})
