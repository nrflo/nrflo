import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModelForm } from './ModelForm'
import { emptyModelForm } from './modelFormData'

const mutation = { isPending: false, isError: false, error: null }

function renderForm(overrides = {}) {
  const props = {
    formData: { ...emptyModelForm, id: 'custom', display_name: 'Custom', cli_model: 'custom', ...overrides },
    setFormData: vi.fn(), onCancel: vi.fn(), onSave: vi.fn(), mutation, isCreate: true,
  }
  return { ...render(<ModelForm {...props} />), props }
}

describe('ModelForm', () => {
  it('renders per-mode model, context, and effort controls', () => {
    renderForm()
    const cli = screen.getByText('CLI').closest('fieldset')!
    const api = screen.getByText('Direct API').closest('fieldset')!
    expect(within(cli).getByRole('textbox')).toHaveValue('custom')
    expect(within(cli).getByRole('spinbutton')).toHaveValue(200000)
    expect(within(cli).getByRole('button', { name: 'low' })).toBeEnabled()
    expect(within(api).getByRole('button', { name: 'low' })).toBeDisabled()
  })

  it('updates a mode effort as a multi-select', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({ cli_efforts: ['low'] })
    const cli = screen.getByText('CLI').closest('fieldset')!
    await user.click(within(cli).getByRole('button', { name: 'high' }))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ cli_efforts: ['low', 'high'] }))
  })

  it('resets default_effort when the chosen effort is unchecked from a mode', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({
      api_model: 'custom-api',
      cli_efforts: ['low', 'high'],
      api_efforts: ['low', 'high'],
      default_effort: 'high',
    })
    const cli = screen.getByText('CLI').closest('fieldset')!
    await user.click(within(cli).getByRole('button', { name: 'high' }))
    expect(props.setFormData).toHaveBeenCalledWith(
      expect.objectContaining({ cli_efforts: ['low'], default_effort: '' }),
    )
    // No stale 'high' default is ever submitted.
    expect(props.setFormData).not.toHaveBeenCalledWith(
      expect.objectContaining({ default_effort: 'high' }),
    )
  })

  it('toggles "none" on as a supported effort for a mode', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({ cli_efforts: ['low'] })
    const cli = screen.getByText('CLI').closest('fieldset')!
    await user.click(within(cli).getByRole('button', { name: 'none' }))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ cli_efforts: ['low', 'none'] }))
  })

  it('offers "none" as a selectable Default Effort option once added to api_efforts', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({ cli_model: '', api_model: 'custom-api', api_efforts: ['none', 'high'] })
    const defaultEffort = screen.getByText('Default Effort').closest('div')!
    await user.click(within(defaultEffort).getByRole('button'))
    await user.click(within(defaultEffort).getByText('none'))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ default_effort: 'none' }))
  })

  it('hides the CLI fieldset and CLI-fallback field when apiOnly, and stays valid on api_model alone', () => {
    const props = {
      formData: { ...emptyModelForm, id: 'custom', display_name: 'Custom', provider: 'openrouter', cli_model: '', api_model: 'moonshotai/kimi-k3' },
      setFormData: vi.fn(), onCancel: vi.fn(), onSave: vi.fn(), mutation, isCreate: true, apiOnly: true,
    }
    render(<ModelForm {...props} />)
    expect(screen.queryByText('CLI')).not.toBeInTheDocument()
    expect(screen.getByText('Direct API')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('model-a, model-b')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create' })).toBeEnabled()
  })

  it('locks built-in fields while leaving default effort and fallback editable', () => {
    const props = {
      formData: { ...emptyModelForm, id: 'sonnet-5', display_name: 'Sonnet', cli_model: 'sonnet', cli_efforts: ['high'] },
      setFormData: vi.fn(), onCancel: vi.fn(), onSave: vi.fn(), mutation, readOnly: true,
    }
    render(<ModelForm {...props} />)
    expect(screen.getByDisplayValue('Sonnet')).toBeDisabled()
    expect(screen.getByPlaceholderText('model-a, model-b')).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Provider default' })).toBeEnabled()
  })
})
