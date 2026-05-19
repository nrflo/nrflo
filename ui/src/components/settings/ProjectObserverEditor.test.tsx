import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectObserverEditor, type ObserverFormState } from './ProjectObserverEditor'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/hooks/useCLIModels', () => ({
  useCLIModels: () => ({
    data: [
      { id: 'claude-sonnet', cli_type: 'claude', display_name: 'Claude Sonnet', enabled: true },
      { id: 'claude-haiku', cli_type: 'claude', display_name: 'Claude Haiku', enabled: true },
      { id: 'codex-mini', cli_type: 'codex', display_name: 'Codex Mini', enabled: true },
    ],
  }),
}))

const PROJECT_ID = 'proj-1'

const defaultValue: ObserverFormState = {
  systemContext: '',
  provider: '',
  model: '',
}

beforeEach(() => vi.clearAllMocks())

function getDropdownButton(labelText: string): HTMLElement {
  const label = screen.getByText(labelText, { selector: 'label' })
  const container = label.closest('div')!
  return within(container).getByRole('button')
}

describe('ProjectObserverEditor', () => {
  it('renders Observer Settings section heading', () => {
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={defaultValue} onChange={vi.fn()} />
    )
    expect(screen.getByText('Observer Settings')).toBeInTheDocument()
  })

  it('renders system context textarea with initial value', () => {
    const value: ObserverFormState = { systemContext: 'Watch for errors', provider: '', model: '' }
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={value} onChange={vi.fn()} />
    )
    expect(screen.getByDisplayValue('Watch for errors')).toBeInTheDocument()
  })

  it('system context change calls onChange with updated systemContext', () => {
    const onChange = vi.fn()
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={defaultValue} onChange={onChange} />
    )

    const textarea = screen.getByPlaceholderText(/optional system context/i)
    fireEvent.change(textarea, { target: { value: 'Monitor errors' } })

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ systemContext: 'Monitor errors' })
    )
  })

  it('selecting provider calls onChange with new provider and clears model', async () => {
    const onChange = vi.fn()
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={defaultValue} onChange={onChange} />
    )

    const user = userEvent.setup()
    await user.click(getDropdownButton('Provider'))
    await user.click(screen.getByText('Claude'))

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'claude', model: '' })
    )
  })

  it('selecting model calls onChange with new model', async () => {
    const onChange = vi.fn()
    const value: ObserverFormState = { systemContext: '', provider: 'claude', model: '' }
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={value} onChange={onChange} />
    )

    const user = userEvent.setup()
    await user.click(getDropdownButton('Model'))
    await user.click(screen.getByText('Claude Sonnet'))

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'claude-sonnet' })
    )
  })

  it('renders serverError verbatim', () => {
    renderWithQuery(
      <ProjectObserverEditor
        projectId={PROJECT_ID}
        value={defaultValue}
        onChange={vi.fn()}
        serverError="observer_system_context must not exceed 4096 chars"
      />
    )
    expect(screen.getByText('observer_system_context must not exceed 4096 chars')).toBeInTheDocument()
  })

  it('does not render serverError paragraph when serverError is null', () => {
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={defaultValue} onChange={vi.fn()} serverError={null} />
    )
    expect(screen.queryByText(/must not exceed/i)).not.toBeInTheDocument()
  })

  it('renders both provider and model dropdowns', () => {
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={defaultValue} onChange={vi.fn()} />
    )
    expect(screen.getByText('Provider', { selector: 'label' })).toBeInTheDocument()
    expect(screen.getByText('Model', { selector: 'label' })).toBeInTheDocument()
  })

  it('model dropdown shows only models for selected provider', async () => {
    const value: ObserverFormState = { systemContext: '', provider: 'codex', model: '' }
    renderWithQuery(
      <ProjectObserverEditor projectId={PROJECT_ID} value={value} onChange={vi.fn()} />
    )

    const user = userEvent.setup()
    await user.click(getDropdownButton('Model'))

    expect(screen.getByText('Codex Mini')).toBeInTheDocument()
    expect(screen.queryByText('Claude Sonnet')).not.toBeInTheDocument()
  })
})
