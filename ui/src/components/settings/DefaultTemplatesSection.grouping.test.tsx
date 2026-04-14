import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
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
    template: 'Implement the feature described in ${TICKET_TITLE}',
    readonly: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('DefaultTemplatesSection — type grouping', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders both section headers when templates include agent and injectable types', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', type: 'agent' }),
      makeTemplate({ id: 'user-instructions', type: 'injectable', name: 'User Instructions' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)

    expect(await screen.findByText('Agent Templates')).toBeInTheDocument()
    expect(screen.getByText('Injectable Templates')).toBeInTheDocument()
  })

  it('hides injectable section when only agent templates exist', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', type: 'agent' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)

    expect(await screen.findByText('Agent Templates')).toBeInTheDocument()
    expect(screen.queryByText('Injectable Templates')).not.toBeInTheDocument()
  })

  it('hides agent section when only injectable templates exist', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'user-instructions', type: 'injectable', name: 'User Instructions' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)

    expect(await screen.findByText('Injectable Templates')).toBeInTheDocument()
    expect(screen.queryByText('Agent Templates')).not.toBeInTheDocument()
  })

  it('shows help text in injectable section', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'user-instructions', type: 'injectable', name: 'User Instructions' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)

    expect(
      await screen.findByText(/automatically prepended to every agent prompt/)
    ).toBeInTheDocument()
  })

  it('shows Injectable badge for injectable templates, not for agent templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', type: 'agent' }),
      makeTemplate({ id: 'user-instructions', type: 'injectable', name: 'User Instructions' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)

    await screen.findByText('implementor')
    const badges = screen.getAllByText('Injectable')
    expect(badges).toHaveLength(1)
  })
})

describe('DefaultTemplateForm — type field', () => {
  beforeEach(() => vi.clearAllMocks())

  it('create form defaults type to Agent', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])
    vi.mocked(defaultTemplatesApi.createDefaultTemplate).mockResolvedValue(
      makeTemplate({ id: 'test', name: 'Test' })
    )

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('No default templates found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Template/ }))

    // The type dropdown button should show "Agent" as default
    expect(screen.getByRole('button', { name: 'Agent' })).toBeInTheDocument()
  })

  it('edit mode shows type as disabled input', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', name: 'Implementor', type: 'agent' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('implementor')

    const user = userEvent.setup()
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[2]) // 0=filter, 1=New Template, 2=edit pencil

    const typeInput = screen.getByDisplayValue('agent')
    expect(typeInput).toBeDisabled()
  })

  it('shows injectable cheatsheet when editing injectable template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'user-instructions', name: 'User Instructions', type: 'injectable' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('user-instructions')

    const user = userEvent.setup()
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[2]) // 0=filter, 1=New Template, 2=edit pencil

    expect(screen.getByText('Injectable Placeholders')).toBeInTheDocument()
    expect(screen.getByText(/\$\{USER_INSTRUCTIONS\}/)).toBeInTheDocument()
    expect(screen.getByText(/\$\{PREVIOUS_DATA\}/)).toBeInTheDocument()
    expect(screen.getByText(/\$\{CALLBACK_INSTRUCTIONS\}/)).toBeInTheDocument()
    expect(screen.getByText(/\$\{CALLBACK_FROM_AGENT\}/)).toBeInTheDocument()
  })

  it('does not show injectable cheatsheet when editing agent template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', name: 'Implementor', type: 'agent' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('implementor')

    const user = userEvent.setup()
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[2])

    expect(screen.queryByText('Injectable Placeholders')).not.toBeInTheDocument()
  })
})
