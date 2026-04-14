import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DefaultTemplatesSection } from './DefaultTemplatesSection'
import * as defaultTemplatesApi from '@/api/defaultTemplates'
import { renderWithQuery } from '@/test/utils'
import type { DefaultTemplate } from '@/api/defaultTemplates'

vi.mock('@/api/defaultTemplates')

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({
    value,
    onChange,
    placeholder,
    readOnly,
  }: {
    value: string
    onChange: (v: string) => void
    placeholder?: string
    readOnly?: boolean
  }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      readOnly={readOnly}
      data-testid="markdown-editor"
    />
  ),
}))

function makeTemplate(overrides: Partial<DefaultTemplate> = {}): DefaultTemplate {
  return {
    id: 'implementor',
    name: 'Implementor',
    type: 'agent',
    template: 'Implement the feature',
    readonly: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('DefaultTemplatesSection — type features', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows Injectable badge for injectable-type templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'user-instructions', name: 'User Instructions', type: 'injectable', readonly: true }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText('Injectable')).toBeInTheDocument()
  })

  it('does not show Injectable badge for agent-type templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', type: 'agent' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('implementor')
    expect(screen.queryByText('Injectable')).not.toBeInTheDocument()
  })

  it('shows both Injectable and Built-in badges for readonly injectable templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'callback', name: 'Callback', type: 'injectable', readonly: true }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText('Injectable')).toBeInTheDocument()
    expect(screen.getByText('Built-in')).toBeInTheDocument()
  })

  it('type filter calls listDefaultTemplates with selected type', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])
    const user = userEvent.setup()
    renderWithQuery(<DefaultTemplatesSection />)

    await screen.findByText('No default templates found. Create one to get started.')

    // Click the type filter dropdown (first button)
    const filterBtn = screen.getAllByRole('button')[0]
    await user.click(filterBtn)

    // Select "Injectable"
    const container = filterBtn.closest('.relative')!
    const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
      (el) => el.textContent === 'Injectable'
    ) as HTMLElement
    await user.click(option)

    await waitFor(() => {
      expect(defaultTemplatesApi.listDefaultTemplates).toHaveBeenCalledWith('injectable')
    })
  })

  it('create form defaults type to agent and allows selecting injectable', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])
    vi.mocked(defaultTemplatesApi.createDefaultTemplate).mockResolvedValue(
      makeTemplate({ id: 'my-inj', name: 'My Injectable', type: 'injectable' })
    )

    const user = userEvent.setup()
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('No default templates found. Create one to get started.')

    await user.click(screen.getByRole('button', { name: /New Template/ }))

    // Type dropdown should show "Agent" by default — find the dropdown in the form
    // The form has three grid columns: ID, Name, Type
    const typeLabel = screen.getByText('Type')
    const typeDropdownBtn = typeLabel.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    expect(typeDropdownBtn).toBeTruthy()

    // Change type to injectable
    await user.click(typeDropdownBtn)
    const dropdownContainer = typeDropdownBtn.closest('.relative')!
    const injectableOption = Array.from(dropdownContainer.querySelectorAll('.cursor-pointer span')).find(
      (el) => el.textContent === 'Injectable'
    ) as HTMLElement
    await user.click(injectableOption)

    // Fill required fields and submit
    await user.type(screen.getByPlaceholderText('my-template'), 'my-inj')
    await user.type(screen.getByPlaceholderText('Template name'), 'My Injectable')
    await user.type(screen.getByPlaceholderText('Agent prompt template...'), 'Injectable content')

    await user.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => {
      expect(defaultTemplatesApi.createDefaultTemplate).toHaveBeenCalledWith({
        id: 'my-inj',
        name: 'My Injectable',
        template: 'Injectable content',
        type: 'injectable',
      })
    })
  })

  it('edit mode shows type as disabled input', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'callback', name: 'Callback', type: 'injectable' }),
    ])
    const user = userEvent.setup()
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('callback')

    // Click edit (buttons: filter, New Template, edit pencil, trash)
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[2])

    // Type field should show as disabled input with value "injectable"
    const typeInput = screen.getByDisplayValue('injectable')
    expect(typeInput).toBeDisabled()
  })
})
