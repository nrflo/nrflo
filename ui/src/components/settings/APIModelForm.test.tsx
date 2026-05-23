import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { APIModelForm, emptyAPIModelForm } from './APIModelForm'
import type { APIModelFormData } from './APIModelForm'

function makeMutation(overrides = {}) {
  return { isPending: false, isError: false, error: null, ...overrides }
}

function renderForm(props: Partial<React.ComponentProps<typeof APIModelForm>> = {}) {
  const defaults = {
    formData: { ...emptyAPIModelForm },
    setFormData: vi.fn(),
    onCancel: vi.fn(),
    onSave: vi.fn(),
    mutation: makeMutation(),
    isCreate: true,
  }
  return render(<APIModelForm {...defaults} {...props} />)
}

describe('APIModelForm', () => {
  describe('required-field gating on Create', () => {
    it('Create button disabled when id is empty', () => {
      renderForm({
        formData: { ...emptyAPIModelForm, id: '', display_name: 'My Model', mapped_model: 'claude-opus-4-7' },
      })
      expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
    })

    it('Create button disabled when display_name is empty', () => {
      renderForm({
        formData: { ...emptyAPIModelForm, id: 'my-id', display_name: '', mapped_model: 'claude-opus-4-7' },
      })
      expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
    })

    it('Create button disabled when mapped_model is empty', () => {
      renderForm({
        formData: { ...emptyAPIModelForm, id: 'my-id', display_name: 'My Model', mapped_model: '' },
      })
      expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
    })

    it('Create button enabled when all required fields filled', () => {
      renderForm({
        formData: { ...emptyAPIModelForm, id: 'my-id', display_name: 'My Model', mapped_model: 'claude-opus-4-7' },
      })
      expect(screen.getByRole('button', { name: /create/i })).not.toBeDisabled()
    })

    it('Create button disabled when mutation is pending', () => {
      renderForm({
        formData: { ...emptyAPIModelForm, id: 'my-id', display_name: 'My Model', mapped_model: 'claude-opus-4-7' },
        mutation: makeMutation({ isPending: true }),
      })
      expect(screen.getByRole('button', { name: /creating/i })).toBeDisabled()
    })
  })

  describe('xhigh reasoning effort', () => {
    async function openEffortDropdown() {
      const effortLabel = screen.getByText('Reasoning Effort').closest('div')!
      const btn = effortLabel.querySelector('button[type="button"]') as HTMLButtonElement
      await userEvent.click(btn)
    }

    it('xhigh option present for anthropic with Opus 4.7 mapped model', async () => {
      renderForm({
        formData: { ...emptyAPIModelForm, provider: 'anthropic', mapped_model: 'claude-opus-4-7-20250514' },
      })
      await openEffortDropdown()
      expect(screen.getByText('Extra High (Opus 4.7 only)')).toBeInTheDocument()
    })

    it('xhigh option present for anthropic non-Opus (disabled via tooltip)', async () => {
      renderForm({
        formData: { ...emptyAPIModelForm, provider: 'anthropic', mapped_model: 'claude-sonnet-4-6' },
      })
      await openEffortDropdown()
      expect(screen.getByText('Extra High (Opus 4.7 only)')).toBeInTheDocument()
    })

    it('xhigh option absent for openai provider', async () => {
      renderForm({
        formData: { ...emptyAPIModelForm, provider: 'openai', mapped_model: 'gpt-4o' },
      })
      await openEffortDropdown()
      expect(screen.queryByText('Extra High (Opus 4.7 only)')).not.toBeInTheDocument()
    })
  })

  describe('built-in lock banner', () => {
    it('shows lock banner when readOnly=true and isCreate=false', () => {
      renderForm({ isCreate: false, readOnly: true })
      expect(screen.getByText(/built-in model/i)).toBeInTheDocument()
    })

    it('does not show lock banner in create mode', () => {
      renderForm({ isCreate: true, readOnly: true })
      expect(screen.queryByText(/built-in model/i)).not.toBeInTheDocument()
    })

    it('does not show lock banner for editable model in edit mode', () => {
      renderForm({ isCreate: false, readOnly: false })
      expect(screen.queryByText(/built-in model/i)).not.toBeInTheDocument()
    })
  })

  describe('provider dropdown', () => {
    it('shows provider dropdown in create mode', () => {
      renderForm({ isCreate: true })
      expect(screen.getByText('Provider')).toBeInTheDocument()
    })

    it('calling setFormData when provider changes', async () => {
      const setFormData = vi.fn()
      renderForm({
        formData: { ...emptyAPIModelForm, provider: 'anthropic' },
        setFormData,
        isCreate: true,
      })
      // Open provider dropdown
      const providerLabel = screen.getByText('Provider').closest('div')!
      const btn = providerLabel.querySelector('button[type="button"]') as HTMLButtonElement
      await userEvent.click(btn)
      await userEvent.click(screen.getByText('OpenAI'))
      expect(setFormData).toHaveBeenCalledWith(expect.objectContaining({ provider: 'openai' }))
    })
  })

  describe('error display', () => {
    it('shows error message when mutation has error', () => {
      renderForm({
        mutation: makeMutation({ isError: true, error: { message: 'something went wrong' } }),
      })
      expect(screen.getByText('Error: something went wrong')).toBeInTheDocument()
    })
  })

  describe('onSave / onCancel callbacks', () => {
    it('calls onSave when Save clicked', async () => {
      const onSave = vi.fn()
      renderForm({
        formData: { ...emptyAPIModelForm, id: 'x', display_name: 'X', mapped_model: 'y' },
        onSave,
      })
      await userEvent.click(screen.getByRole('button', { name: /create/i }))
      expect(onSave).toHaveBeenCalled()
    })

    it('calls onCancel when Cancel clicked', async () => {
      const onCancel = vi.fn()
      renderForm({ onCancel })
      await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
      expect(onCancel).toHaveBeenCalled()
    })
  })
})
