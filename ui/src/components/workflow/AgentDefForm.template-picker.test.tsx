import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'
import { renderWithQuery } from '@/test/utils'
import * as defaultTemplatesApi from '@/api/defaultTemplates'

// Mock MarkdownEditor to avoid CodeMirror dependencies
vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: any) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

vi.mock('@/api/defaultTemplates', () => ({
  listDefaultTemplates: vi.fn().mockResolvedValue([]),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  const defaultProps = { isCreate: true, onSubmit: vi.fn(), onCancel: vi.fn(), ...props }
  return renderWithQuery(<AgentDefForm {...defaultProps} />)
}

describe('AgentDefForm - Apply Template button', () => {
  it('renders Apply Template button next to Prompt Template label', () => {
    renderForm()
    expect(screen.getByRole('button', { name: /apply template/i })).toBeInTheDocument()
  })

  it('clicking Apply Template opens the template picker dialog', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderForm()

    await user.click(screen.getByRole('button', { name: /apply template/i }))

    // Dialog header should appear
    expect(screen.getByText('Apply Default Template')).toBeInTheDocument()
  })

  it('applies selected template to prompt field', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      {
        id: 'implementor',
        name: 'Implementor',
        type: 'agent',
        template: 'You are an implementor.',
        readonly: true,
        created_at: '',
        updated_at: '',
      },
    ])
    const user = userEvent.setup()
    renderForm()

    await user.click(screen.getByRole('button', { name: /apply template/i }))

    // Wait for templates to load in dialog
    await screen.findByText('Template')

    // Select the template from dropdown
    const label = screen.getByText('Template')
    const btn = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(btn)
    const container = btn.closest('.relative')!
    const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
      (el) => el.textContent === 'Implementor'
    ) as HTMLElement
    await user.click(option)

    await user.click(screen.getByRole('button', { name: /^apply$/i }))

    // Dialog closed, prompt updated
    expect(screen.queryByText('Apply Default Template')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText(/agent prompt template/i)).toHaveValue('You are an implementor.')
  })

  it('passes hasExistingPrompt=true to dialog when prompt is non-empty', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      {
        id: 'implementor',
        name: 'Implementor',
        type: 'agent',
        template: 'New template',
        readonly: true,
        created_at: '',
        updated_at: '',
      },
    ])
    const user = userEvent.setup()
    renderForm({ initial: { prompt: 'Existing prompt content' } })

    await user.click(screen.getByRole('button', { name: /apply template/i }))
    await screen.findByText('Template')

    // Select a template to trigger warning visibility
    const label = screen.getByText('Template')
    const btn = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(btn)
    const container = btn.closest('.relative')!
    const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
      (el) => el.textContent === 'Implementor'
    ) as HTMLElement
    await user.click(option)

    expect(screen.getByText(/current agent prompt is not empty/i)).toBeInTheDocument()
  })
})
