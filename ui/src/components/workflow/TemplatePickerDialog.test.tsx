import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TemplatePickerDialog } from './TemplatePickerDialog'
import { renderWithQuery } from '@/test/utils'
import * as defaultTemplatesApi from '@/api/defaultTemplates'
import type { DefaultTemplate } from '@/api/defaultTemplates'

vi.mock('@/api/defaultTemplates', () => ({
  listDefaultTemplates: vi.fn(),
}))

const makeTemplate = (overrides: Partial<DefaultTemplate> = {}): DefaultTemplate => ({
  id: 'implementor',
  name: 'Implementor',
  type: 'agent',
  template: 'You are a skilled implementor agent.',
  readonly: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
})

function renderDialog(props: Partial<React.ComponentProps<typeof TemplatePickerDialog>> = {}) {
  return renderWithQuery(
    <TemplatePickerDialog
      open={true}
      onClose={vi.fn()}
      onApply={vi.fn()}
      hasExistingPrompt={false}
      {...props}
    />
  )
}

/** Open the template dropdown and click a named option */
async function selectTemplate(user: ReturnType<typeof userEvent.setup>, templateName: string) {
  const label = screen.getByText('Template')
  const btn = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
  await user.click(btn)
  const container = btn.closest('.relative')!
  const option = Array.from(container.querySelectorAll('.cursor-pointer span')).find(
    (el) => el.textContent === templateName
  ) as HTMLElement
  await user.click(option)
}

describe('TemplatePickerDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading spinner while fetching', () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockReturnValue(new Promise(() => {}))
    renderDialog()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
    expect(screen.queryByText('Template')).not.toBeInTheDocument()
  })

  it('shows empty message when no templates available', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([])
    renderDialog()
    expect(await screen.findByText(/no default templates available/i)).toBeInTheDocument()
  })

  it('shows template names as dropdown options after load', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([
      makeTemplate({ id: 'implementor', name: 'Implementor' }),
      makeTemplate({ id: 'test-writer', name: 'Test Writer' }),
    ])
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('Template')
    const label = screen.getByText('Template')
    const btn = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    await user.click(btn)

    expect(screen.getByText('Implementor')).toBeInTheDocument()
    expect(screen.getByText('Test Writer')).toBeInTheDocument()
  })

  it('shows preview when a template is selected', async () => {
    const template = makeTemplate({ template: 'Analyze the codebase and produce findings.' })
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([template])
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('Template')
    await selectTemplate(user, 'Implementor')

    expect(screen.getByText('Preview')).toBeInTheDocument()
    expect(screen.getByText('Analyze the codebase and produce findings.')).toBeInTheDocument()
  })

  it('Apply button is disabled before any template is selected', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
    renderDialog()
    await screen.findByText('Template')
    expect(screen.getByRole('button', { name: /^apply$/i })).toBeDisabled()
  })

  it('Apply calls onApply with template content and calls onClose', async () => {
    const onApply = vi.fn()
    const onClose = vi.fn()
    const template = makeTemplate({ template: 'Selected template content' })
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([template])
    const user = userEvent.setup()
    renderDialog({ onApply, onClose })

    await screen.findByText('Template')
    await selectTemplate(user, 'Implementor')
    await user.click(screen.getByRole('button', { name: /^apply$/i }))

    expect(onApply).toHaveBeenCalledWith('Selected template content')
    expect(onClose).toHaveBeenCalled()
  })

  it('Cancel calls onClose without calling onApply', async () => {
    const onApply = vi.fn()
    const onClose = vi.fn()
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
    const user = userEvent.setup()
    renderDialog({ onApply, onClose })

    await screen.findByText('Template')
    await user.click(screen.getByRole('button', { name: /^cancel$/i }))

    expect(onClose).toHaveBeenCalled()
    expect(onApply).not.toHaveBeenCalled()
  })

  it('does not fetch when dialog is closed', () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
    renderDialog({ open: false })
    expect(defaultTemplatesApi.listDefaultTemplates).not.toHaveBeenCalled()
  })

  it('fetches only agent-type templates', async () => {
    vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
    renderDialog()
    await screen.findByText('Template')
    expect(defaultTemplatesApi.listDefaultTemplates).toHaveBeenCalledWith('agent')
  })

  describe('with existing prompt', () => {
    it('does not show warning before a template is selected', async () => {
      vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
      renderDialog({ hasExistingPrompt: true })

      await screen.findByText('Template')
      expect(screen.queryByText(/current agent prompt is not empty/i)).not.toBeInTheDocument()
    })

    it('shows warning text when template selected and hasExistingPrompt=true', async () => {
      vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
      const user = userEvent.setup()
      renderDialog({ hasExistingPrompt: true })

      await screen.findByText('Template')
      await selectTemplate(user, 'Implementor')

      expect(screen.getByText(/current agent prompt is not empty/i)).toBeInTheDocument()
    })

    it('shows "Replace Current Prompt" button when template selected and hasExistingPrompt=true', async () => {
      vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([makeTemplate()])
      const user = userEvent.setup()
      renderDialog({ hasExistingPrompt: true })

      await screen.findByText('Template')
      await selectTemplate(user, 'Implementor')

      expect(screen.getByRole('button', { name: /replace current prompt/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^apply$/i })).not.toBeInTheDocument()
    })

    it('"Replace Current Prompt" calls onApply and onClose', async () => {
      const onApply = vi.fn()
      const onClose = vi.fn()
      const template = makeTemplate({ template: 'Replacement content' })
      vi.mocked(defaultTemplatesApi.listDefaultTemplates).mockResolvedValue([template])
      const user = userEvent.setup()
      renderDialog({ hasExistingPrompt: true, onApply, onClose })

      await screen.findByText('Template')
      await selectTemplate(user, 'Implementor')
      await user.click(screen.getByRole('button', { name: /replace current prompt/i }))

      expect(onApply).toHaveBeenCalledWith('Replacement content')
      expect(onClose).toHaveBeenCalled()
    })
  })
})
