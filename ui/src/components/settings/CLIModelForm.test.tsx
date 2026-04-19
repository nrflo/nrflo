import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CLIModelForm, type CLIModelFormData } from './CLIModelForm'

function makeFormData(overrides: Partial<CLIModelFormData> = {}): CLIModelFormData {
  return {
    id: 'opus_4_7',
    cli_type: 'claude',
    display_name: 'Opus 4.7',
    mapped_model: 'claude-opus-4-7',
    reasoning_effort: '',
    context_length: '200000',
    ...overrides,
  }
}

function renderForm(overrides: Partial<CLIModelFormData> = {}) {
  const setFormData = vi.fn()
  const formData = makeFormData(overrides)
  render(
    <CLIModelForm
      formData={formData}
      setFormData={setFormData}
      onCancel={vi.fn()}
      onSave={vi.fn()}
      mutation={{ isPending: false, isError: false, error: null }}
    />
  )
  return { setFormData, formData }
}

function getEffortDropdownRoot() {
  const label = screen.getByText('Reasoning Effort')
  const wrapper = label.parentElement!.querySelector('.relative') as HTMLElement | null
  if (!wrapper) throw new Error('Reasoning Effort dropdown wrapper not found')
  return wrapper
}

function getEffortTrigger() {
  return getEffortDropdownRoot().querySelector('button') as HTMLButtonElement
}

async function openAndGetPanel() {
  const user = userEvent.setup()
  await user.click(getEffortTrigger())
  const panel = await within(getEffortDropdownRoot()).findByText('Low')
  return { user, panel: panel.closest('.z-50') as HTMLElement }
}

describe('CLIModelForm — Reasoning Effort dropdown', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders a dropdown (not a text input) for Reasoning Effort', () => {
    renderForm()
    const label = screen.getByText('Reasoning Effort')
    // no <input> inside the Reasoning Effort field wrapper
    expect(label.parentElement!.querySelector('input')).toBeNull()
    // but a <button> (Dropdown trigger) exists
    expect(label.parentElement!.querySelector('button')).toBeInTheDocument()
  })

  it('claude + Opus 4.7: all six options render in panel and none are disabled', async () => {
    renderForm({ cli_type: 'claude', mapped_model: 'claude-opus-4-7' })
    const { panel } = await openAndGetPanel()
    const panelUtils = within(panel)

    expect(panelUtils.getByText('Default')).toBeInTheDocument()
    expect(panelUtils.getByText('Low')).toBeInTheDocument()
    expect(panelUtils.getByText('Medium')).toBeInTheDocument()
    expect(panelUtils.getByText('High')).toBeInTheDocument()
    expect(panelUtils.getByText('Extra High (Opus 4.7 only)')).toBeInTheDocument()
    expect(panelUtils.getByText('Max')).toBeInTheDocument()

    const xhighOption = panelUtils.getByText('Extra High (Opus 4.7 only)').parentElement!
    expect(xhighOption).not.toHaveAttribute('aria-disabled')
  })

  it('claude + Opus 4.7 1M prefix: xhigh option is enabled', async () => {
    renderForm({ cli_type: 'claude', mapped_model: 'claude-opus-4-7[1m]' })
    const { panel } = await openAndGetPanel()
    const xhighOption = within(panel).getByText('Extra High (Opus 4.7 only)').parentElement!
    expect(xhighOption).not.toHaveAttribute('aria-disabled')
  })

  it('claude + non-Opus-4.7 model: xhigh rendered but disabled and non-clickable', async () => {
    const { setFormData } = renderForm({ cli_type: 'claude', mapped_model: 'claude-sonnet-4-5' })
    const { user, panel } = await openAndGetPanel()

    const xhighLabel = within(panel).getByText('Extra High (Opus 4.7 only)')
    const xhighOption = xhighLabel.parentElement!
    expect(xhighOption).toHaveAttribute('aria-disabled', 'true')

    await user.click(xhighLabel)
    expect(setFormData).not.toHaveBeenCalled()
  })

  it('claude + non-Opus-4.7 model: hovering disabled xhigh shows tooltip', async () => {
    renderForm({ cli_type: 'claude', mapped_model: 'claude-sonnet-4-5' })
    const { user, panel } = await openAndGetPanel()
    const xhighOption = within(panel).getByText('Extra High (Opus 4.7 only)').parentElement!

    await user.hover(xhighOption)
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/only supported on Opus 4\.7/i)
  })

  it('opencode cli_type: xhigh option is NOT rendered; other five are', async () => {
    renderForm({ cli_type: 'opencode', mapped_model: 'anthropic/claude-sonnet-4' })
    const { panel } = await openAndGetPanel()
    const panelUtils = within(panel)

    expect(panelUtils.getByText('Default')).toBeInTheDocument()
    expect(panelUtils.getByText('Low')).toBeInTheDocument()
    expect(panelUtils.getByText('Medium')).toBeInTheDocument()
    expect(panelUtils.getByText('High')).toBeInTheDocument()
    expect(panelUtils.getByText('Max')).toBeInTheDocument()
    expect(panelUtils.queryByText('Extra High (Opus 4.7 only)')).not.toBeInTheDocument()
  })

  it('codex cli_type: xhigh option is NOT rendered', async () => {
    renderForm({ cli_type: 'codex', mapped_model: 'gpt-5' })
    const { panel } = await openAndGetPanel()
    expect(within(panel).queryByText('Extra High (Opus 4.7 only)')).not.toBeInTheDocument()
  })

  it('selecting High calls setFormData with reasoning_effort="high"', async () => {
    const { setFormData, formData } = renderForm({ reasoning_effort: '' })
    const { user, panel } = await openAndGetPanel()

    await user.click(within(panel).getByText('High'))
    expect(setFormData).toHaveBeenCalledWith({ ...formData, reasoning_effort: 'high' })
  })

  it('selecting Default calls setFormData with reasoning_effort=""', async () => {
    const { setFormData, formData } = renderForm({ reasoning_effort: 'high' })
    const { user, panel } = await openAndGetPanel()

    await user.click(within(panel).getByText('Default'))
    expect(setFormData).toHaveBeenCalledWith({ ...formData, reasoning_effort: '' })
  })

  it('selecting Extra High on Opus 4.7 calls setFormData with reasoning_effort="xhigh"', async () => {
    const { setFormData, formData } = renderForm({
      cli_type: 'claude',
      mapped_model: 'claude-opus-4-7',
      reasoning_effort: '',
    })
    const { user, panel } = await openAndGetPanel()

    await user.click(within(panel).getByText('Extra High (Opus 4.7 only)'))
    expect(setFormData).toHaveBeenCalledWith({ ...formData, reasoning_effort: 'xhigh' })
  })
})
