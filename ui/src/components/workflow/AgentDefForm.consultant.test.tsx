import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

vi.mock('@/hooks/useModels', () => ({
  useModelOptions: () => [
    { label: 'Anthropic', options: [{ value: 'sonnet-5', label: 'Anthropic: Sonnet' }] },
  ],
  useModels: () => ({ data: [] }),
}))

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: () => true,
}))

vi.mock('@/hooks/useDefaultTemplates', () => ({
  useInjectableTemplates: () => ({ data: [] }),
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm
      isCreate={true}
      onSubmit={vi.fn()}
      onCancel={vi.fn()}
      {...props}
    />
  )
}

function getConsultantToggle() {
  // The tools picker adds its own switches, so scope by the Consultant label.
  return screen.getByRole('switch', { name: 'Consultant' })
}

describe('AgentDefForm — consultant toggle', () => {
  it('consultant toggle is unchecked by default', () => {
    renderForm()
    expect(getConsultantToggle()).toHaveAttribute('aria-checked', 'false')
  })

  it('initialises consultant=true when initial.consultant is true', () => {
    renderForm({ isCreate: false, initial: { consultant: true, execution_mode: 'api' } })
    expect(getConsultantToggle()).toHaveAttribute('aria-checked', 'true')
  })

  it('toggling ON forces execution_mode to api', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'my-consultant')
    await user.type(screen.getByLabelText('Prompt Template'), 'You are a consultant')
    await user.click(getConsultantToggle())

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ consultant: true, execution_mode: 'api' })
    )
  })

  it('toggling OFF resets consultant to undefined in payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderForm({ onSubmit })

    await user.type(screen.getByPlaceholderText(/e.g., setup-analyzer/i), 'regular-agent')
    await user.type(screen.getByLabelText('Prompt Template'), 'prompt')

    // toggle ON then OFF
    await user.click(getConsultantToggle())
    await user.click(getConsultantToggle())

    await user.click(screen.getByRole('button', { name: /create/i }))

    const call = onSubmit.mock.calls[0][0]
    expect(call.consultant).toBeFalsy()
  })

  it('Layer field hidden when consultant is ON', async () => {
    const user = userEvent.setup()
    renderForm()

    expect(screen.getByText('Layer')).toBeInTheDocument()
    await user.click(getConsultantToggle())
    expect(screen.queryByText('Layer')).not.toBeInTheDocument()
  })

  it('Restart % field hidden when consultant is ON', async () => {
    const user = userEvent.setup()
    renderForm()

    expect(screen.getByPlaceholderText('25')).toBeInTheDocument()
    await user.click(getConsultantToggle())
    expect(screen.queryByPlaceholderText('25')).not.toBeInTheDocument()
  })

  it('Fail restarts field hidden when consultant is ON', async () => {
    const user = userEvent.setup()
    renderForm()

    expect(screen.getByText('Fail restarts')).toBeInTheDocument()
    await user.click(getConsultantToggle())
    expect(screen.queryByText('Fail restarts')).not.toBeInTheDocument()
  })

  it('Execution Mode dropdown is disabled when consultant is ON', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.click(getConsultantToggle())

    // Dropdown renders disabled as a CSS class (not the disabled attribute)
    const label = screen.getByText('Execution Mode')
    const btn = label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
    expect(btn.className).toContain('cursor-not-allowed')
  })

  it('Layer and Restart fields visible again after toggling OFF', async () => {
    const user = userEvent.setup()
    renderForm()

    await user.click(getConsultantToggle())
    expect(screen.queryByText('Layer')).not.toBeInTheDocument()

    await user.click(getConsultantToggle())
    expect(screen.getByText('Layer')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('25')).toBeInTheDocument()
  })
})
