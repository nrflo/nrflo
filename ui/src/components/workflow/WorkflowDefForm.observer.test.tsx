import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
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
  useCLIModels: () => ({
    data: [
      { id: 'claude-sonnet', cli_type: 'claude', display_name: 'Claude Sonnet', enabled: true },
      { id: 'codex-mini', cli_type: 'codex', display_name: 'Codex Mini', enabled: true },
    ],
  }),
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
      <>
        <WorkflowDefForm {...defaultProps} />
        <button type="submit" form="test-form">Submit</button>
      </>
    ),
    props: defaultProps,
  }
}

describe('WorkflowDefForm — observer fields', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(workflowApi.listWorkflowDefs).mockResolvedValue({})
  })

  it('renders observer overrides section heading', () => {
    renderForm()
    expect(screen.getByText('Observer overrides')).toBeInTheDocument()
  })

  it('observer_context textarea renders with placeholder', () => {
    renderForm()
    expect(screen.getByPlaceholderText(/optional observer context for this workflow/i)).toBeInTheDocument()
  })

  it('submit includes observer_context when typed', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-workflow')
    await user.type(
      screen.getByPlaceholderText(/optional observer context for this workflow/i),
      'Focus on errors'
    )
    await user.click(screen.getByRole('button', { name: /submit/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ observer_context: 'Focus on errors' })
    )
  })

  it('submit includes observer_provider and observer_model when selected', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')

    // Open provider dropdown (within observer overrides section — last two provider/model dropdowns)
    const providerLabel = screen.getByText('Provider', { selector: 'label' })
    const providerDropdown = providerLabel.closest('div')!.querySelector('button') as HTMLButtonElement
    await user.click(providerDropdown)
    await user.click(screen.getByText('Claude'))

    await user.click(screen.getByRole('button', { name: /submit/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ observer_provider: 'claude' })
    )
  })

  it('submit includes null observer_provider when not selected', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')
    await user.click(screen.getByRole('button', { name: /submit/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ observer_provider: null })
    )
  })

  it('initial observer values are pre-populated in edit mode', () => {
    renderForm({
      isCreate: false,
      initial: {
        id: 'my-flow',
        observer_context: 'Watch carefully',
        observer_provider: 'claude',
        observer_model: 'claude-sonnet',
      },
    })

    expect(screen.getByDisplayValue('Watch carefully')).toBeInTheDocument()
  })

  it('observer fields use UI primitives (textarea and button role)', () => {
    renderForm()
    const textarea = screen.getByPlaceholderText(/optional observer context for this workflow/i)
    expect(textarea.tagName).toBe('TEXTAREA')
    // Provider dropdown renders as a button
    const providerLabel = screen.getByText('Provider', { selector: 'label' })
    const btn = providerLabel.closest('div')!.querySelector('button')
    expect(btn).toBeInTheDocument()
  })

  it('selecting observer_model includes it in submit payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ isCreate: true, onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., feature/i), 'my-flow')

    // Select claude provider first
    const providerLabel = screen.getByText('Provider', { selector: 'label' })
    const providerDropdown = providerLabel.closest('div')!.querySelector('button') as HTMLButtonElement
    await user.click(providerDropdown)
    await user.click(screen.getByText('Claude'))

    // Now select a model
    const modelLabel = screen.getByText('Model', { selector: 'label' })
    const modelDropdown = modelLabel.closest('div')!.querySelector('button') as HTMLButtonElement
    await user.click(modelDropdown)
    await user.click(screen.getByText('Claude Sonnet'))

    await user.click(screen.getByRole('button', { name: /submit/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ observer_provider: 'claude', observer_model: 'claude-sonnet' })
    )
  })

  it('changing provider resets model to empty string', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({
      isCreate: false,
      initial: { id: 'my-flow', observer_provider: 'claude', observer_model: 'claude-sonnet' },
      onSubmit,
    })

    // Change provider to codex — model should reset
    const providerLabel = screen.getByText('Provider', { selector: 'label' })
    const providerDropdown = providerLabel.closest('div')!.querySelector('button') as HTMLButtonElement
    await user.click(providerDropdown)
    await user.click(screen.getByText('Codex'))

    await user.click(screen.getByRole('button', { name: /submit/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ observer_provider: 'codex', observer_model: null })
    )
  })
})
