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
