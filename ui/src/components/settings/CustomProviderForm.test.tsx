import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CustomProviderForm, emptyCustomProviderForm, providerToFormData } from './CustomProviderForm'
import type { CustomProvider } from '@/api/customProviders'

const mutation = { isPending: false, isError: false, error: null }

const provider: CustomProvider = {
  name: 'my-provider', base_url: 'https://api.example.com/v1', api_key: 'stored-key',
  api_wire: 'responses', enabled: true, created_at: '', updated_at: '',
}

function renderForm(overrides: Record<string, unknown> = {}) {
  const props = {
    formData: emptyCustomProviderForm,
    setFormData: vi.fn(), onCancel: vi.fn(), onSave: vi.fn(), mutation,
    ...overrides,
  }
  return { ...render(<CustomProviderForm {...props} />), props }
}

describe('CustomProviderForm', () => {
  it('create mode: Create is disabled until name and base_url are filled', () => {
    renderForm({ isCreate: true })
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('create mode: Create enables once name, base_url, and api_key are present', () => {
    renderForm({
      isCreate: true,
      formData: { ...emptyCustomProviderForm, name: 'acme', base_url: 'https://acme.test', api_key: 'sk-1' },
    })
    expect(screen.getByRole('button', { name: 'Create' })).toBeEnabled()
  })

  it('create mode: name field is editable', () => {
    renderForm({ isCreate: true })
    expect(screen.getByPlaceholderText('my-provider')).toBeEnabled()
  })

  it('edit mode: name field is disabled and Save does not require api_key', () => {
    renderForm({ formData: providerToFormData(provider) })
    expect(screen.getByPlaceholderText('my-provider')).toBeDisabled()
    expect(screen.getByRole('button', { name: /Save/ })).toBeEnabled()
  })

  it('edit mode: blank api_key field means providerToFormData seeded it empty', () => {
    const data = providerToFormData(provider)
    expect(data.api_key).toBe('')
    expect(data.name).toBe('my-provider')
    expect(data.base_url).toBe('https://api.example.com/v1')
  })

  it('typing in the API key field calls setFormData with the new value', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({ formData: providerToFormData(provider) })
    await user.type(screen.getByPlaceholderText('Leave blank to keep current key'), 'x')
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ api_key: 'x' }))
  })

  it('toggling Enabled calls setFormData with the flipped value', async () => {
    const user = userEvent.setup()
    const { props } = renderForm({ formData: { ...emptyCustomProviderForm, enabled: true } })
    await user.click(screen.getByRole('switch'))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }))
  })

  it('changing the API Wire dropdown calls setFormData with the new wire', async () => {
    const user = userEvent.setup()
    const { props } = renderForm()
    await user.click(screen.getByText('Responses API'))
    await user.click(screen.getByText('Chat Completions API'))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ api_wire: 'chat_completions' }))
  })

  it('changing the API Wire dropdown to Ollama Native calls setFormData with ollama_native', async () => {
    const user = userEvent.setup()
    const { props } = renderForm()
    await user.click(screen.getByText('Responses API'))
    await user.click(screen.getByText('Ollama Native (/api/chat)'))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ api_wire: 'ollama_native' }))
  })

  it('shows the mutation error message', () => {
    renderForm({ mutation: { isPending: false, isError: true, error: new Error('name already exists') } })
    expect(screen.getByText(/name already exists/)).toBeInTheDocument()
  })
})
