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

describe('DefaultTemplatesSection — restore functionality', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows Modified badge when readonly template text differs from default_template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({
        id: 'setup-analyzer',
        readonly: true,
        template: 'Customized text',
        default_template: 'Original text',
      }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    expect(await screen.findByText('Modified')).toBeInTheDocument()
  })

  it('does not show Modified badge when template matches default_template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({
        id: 'setup-analyzer',
        readonly: true,
        template: 'Same text',
        default_template: 'Same text',
      }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('setup-analyzer')
    expect(screen.queryByText('Modified')).not.toBeInTheDocument()
  })

  it('does not show Modified badge for readonly template with no default_template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'setup-analyzer', readonly: true }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('setup-analyzer')
    expect(screen.queryByText('Modified')).not.toBeInTheDocument()
  })

  it('shows Restore Default button in edit form for modified readonly template', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({
        id: 'setup-analyzer',
        name: 'Setup Analyzer',
        readonly: true,
        template: 'Customized text',
        default_template: 'Original text',
      }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('setup-analyzer')

    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button')[1]) // edit pencil

    expect(screen.getByRole('button', { name: /Restore Default/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Save/i })).toBeInTheDocument()
  })

  it('does not show Restore Default button when readonly template is unmodified', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({
        id: 'setup-analyzer',
        name: 'Setup Analyzer',
        readonly: true,
        template: 'Same text',
        default_template: 'Same text',
      }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('setup-analyzer')

    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button')[1])

    expect(screen.queryByRole('button', { name: /Restore Default/i })).not.toBeInTheDocument()
  })

  it('calls restoreDefaultTemplate and invalidates query on Restore Default click', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({
        id: 'setup-analyzer',
        name: 'Setup Analyzer',
        readonly: true,
        template: 'Customized text',
        default_template: 'Original text',
      }),
    ])
    vi.mocked(defaultTemplatesApi.restoreDefaultTemplate).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('setup-analyzer')

    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button')[1])
    await user.click(screen.getByRole('button', { name: /Restore Default/i }))

    await waitFor(() => {
      expect(defaultTemplatesApi.restoreDefaultTemplate).toHaveBeenCalledWith('setup-analyzer')
    })
  })

  it('saves readonly template with only template field — name is excluded', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates)
      .mockResolvedValueOnce([
        makeTemplate({ id: 'qa-verifier', name: 'QA Verifier', template: 'Check the code', readonly: true }),
      ])
      .mockResolvedValue([])
    vi.mocked(defaultTemplatesApi.updateDefaultTemplate).mockResolvedValue({ status: 'ok' })

    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('qa-verifier')

    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button')[1])
    await user.click(screen.getByRole('button', { name: /Save/i }))

    await waitFor(() => {
      expect(defaultTemplatesApi.updateDefaultTemplate).toHaveBeenCalledWith('qa-verifier', {
        template: 'Check the code',
      })
    })
  })

  it('non-readonly template has no Modified badge and no default_template behavior', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'custom-tpl', name: 'Custom', readonly: false }),
    ])
    renderWithQuery(<DefaultTemplatesSection />)
    await screen.findByText('custom-tpl')
    expect(screen.queryByText('Built-in')).not.toBeInTheDocument()
    expect(screen.queryByText('Modified')).not.toBeInTheDocument()
  })
})
