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
    template: 'Implement the feature described in ${TICKET_TITLE}',
    readonly: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('DefaultTemplatesSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows loading then empty state when no templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(
      await screen.findByText('No default templates found. Create one to get started.')
    ).toBeInTheDocument()
  })

  it('shows error state on fetch failure', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockRejectedValue(
      new Error('fetch failed')
    )
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText(/Error: fetch failed/)).toBeInTheDocument()
  })

  it('displays template list with id, name, and truncated preview', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', name: 'Implementor Template' }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText('implementor')).toBeInTheDocument()
    expect(screen.getByText('Implementor Template')).toBeInTheDocument()
  })

  it('shows Built-in badge for readonly templates and hides delete button', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'setup-analyzer', name: 'Setup Analyzer', readonly: true }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText('Built-in')).toBeInTheDocument()

    // Only New Template button + edit pencil — no delete trash button
    const buttons = screen.getAllByRole('button')
    const labels = buttons.map((b) => b.getAttribute('aria-label') ?? b.textContent)
    // No trash/delete button for readonly
    expect(labels.some((l) => l?.toLowerCase().includes('delete'))).toBe(false)
    // Exactly 2 buttons: "New Template" + edit pencil
    expect(buttons).toHaveLength(2)
  })

  it('opens edit form for readonly template with Save button and name disabled', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'qa-verifier', name: 'QA Verifier', readonly: true }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('qa-verifier')

    const user = userEvent.setup()
    // buttons[0]=New Template, buttons[1]=edit pencil
    const buttons = screen.getAllByRole('button')
    await user.click(buttons[1])

    // No "cannot be modified" message
    expect(
      screen.queryByText('This is a built-in template and cannot be modified.')
    ).not.toBeInTheDocument()
    // ID field disabled with value
    const idInput = screen.getByDisplayValue('qa-verifier')
    expect(idInput).toBeDisabled()
    // Save button present for readonly templates
    expect(screen.getByRole('button', { name: /Save/i })).toBeInTheDocument()
    // Cancel button present
    expect(screen.getByRole('button', { name: /Cancel/i })).toBeInTheDocument()
  })

  it('create form: opens, validates all required fields, submits, then cancels', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates)
      .mockResolvedValueOnce([])
      .mockResolvedValue([makeTemplate({ id: 'my-tpl', name: 'My Tpl' })])
    vi.mocked(defaultTemplatesApi.createDefaultTemplate).mockResolvedValue(
      makeTemplate({ id: 'my-tpl', name: 'My Tpl' })
    )

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('No default templates found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Template/ }))

    // Create button disabled until all three fields are filled
    const createBtn = screen.getByRole('button', { name: 'Create' })
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('my-template'), 'my-tpl')
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('Template name'), 'My Tpl')
    expect(createBtn).toBeDisabled()

    await user.type(screen.getByPlaceholderText('Agent prompt template...'), 'Do the thing')
    expect(createBtn).not.toBeDisabled()

    await user.click(createBtn)
    await waitFor(() => {
      expect(defaultTemplatesApi.createDefaultTemplate).toHaveBeenCalledWith({
        id: 'my-tpl',
        name: 'My Tpl',
        template: 'Do the thing',
      })
    })
  })

  it('cancel on create form closes it without calling API', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('No default templates found. Create one to get started.')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /New Template/ }))
    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('button', { name: 'Create' })).not.toBeInTheDocument()
    expect(defaultTemplatesApi.createDefaultTemplate).not.toHaveBeenCalled()
  })

  it('edit non-readonly: pre-populates form, id disabled, saves with name+template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates)
      .mockResolvedValueOnce([
        makeTemplate({ id: 'doc-updater', name: 'Doc Updater', template: 'Update the docs' }),
      ])
      .mockResolvedValue([])
    vi.mocked(defaultTemplatesApi.updateDefaultTemplate).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('doc-updater')

    const user = userEvent.setup()
    const buttons = screen.getAllByRole('button')
    // buttons[0]=New Template, buttons[1]=edit pencil, buttons[2]=trash
    await user.click(buttons[1])

    // ID is disabled and pre-filled
    const idInput = screen.getByDisplayValue('doc-updater')
    expect(idInput).toBeDisabled()

    // Name field pre-filled
    expect(screen.getByDisplayValue('Doc Updater')).toBeInTheDocument()

    // Save submits with correct args
    await user.click(screen.getByRole('button', { name: /Save/i }))
    await waitFor(() => {
      expect(defaultTemplatesApi.updateDefaultTemplate).toHaveBeenCalledWith('doc-updater', {
        name: 'Doc Updater',
        template: 'Update the docs',
      })
    })
  })

  it('delete non-readonly: confirmation dialog, cancel dismisses, confirm calls API', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates)
      .mockResolvedValueOnce([
        makeTemplate({ id: 'custom-tpl', name: 'Custom', readonly: false }),
      ])
      .mockResolvedValue([])
    vi.mocked(defaultTemplatesApi.deleteDefaultTemplate).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('custom-tpl')

    const user = userEvent.setup()
    // buttons[0]=New Template, buttons[1]=edit pencil, buttons[2]=trash
    let buttons = screen.getAllByRole('button')
    await user.click(buttons[2])

    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()
    expect(screen.getByText('custom-tpl')).toBeInTheDocument()

    // Cancel dismisses without deleting
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument()
    expect(defaultTemplatesApi.deleteDefaultTemplate).not.toHaveBeenCalled()

    // Re-open and confirm delete
    buttons = screen.getAllByRole('button')
    await user.click(buttons[2])
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(defaultTemplatesApi.deleteDefaultTemplate).toHaveBeenCalledWith('custom-tpl')
    })
  })

  it('New Template button is disabled while create form is open', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('No default templates found. Create one to get started.')

    const user = userEvent.setup()
    const newBtn = screen.getByRole('button', { name: /New Template/ })
    await user.click(newBtn)
    expect(newBtn).toBeDisabled()
  })
})
