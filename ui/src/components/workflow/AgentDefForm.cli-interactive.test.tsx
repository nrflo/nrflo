import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefForm } from './AgentDefForm'

const mockUseAPIModeEnabled = vi.hoisted(() => vi.fn().mockReturnValue(true))

vi.mock('@/hooks/useGlobalSettings', () => ({
  useAPIModeEnabled: mockUseAPIModeEnabled,
}))

vi.mock('@/hooks/useCLIModels', () => ({
  useModelOptions: () => [],
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({ value, onChange, placeholder }: {
    value: string; onChange: (v: string) => void; placeholder?: string
  }) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      aria-label="Prompt Template"
    />
  ),
}))

vi.mock('@/components/workflow/PythonScriptPickerField', () => ({
  PythonScriptPickerField: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <select aria-label="Python Script" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">-- select script --</option>
      <option value="script-1">Script One</option>
    </select>
  ),
}))

function renderForm(props: Partial<React.ComponentProps<typeof AgentDefForm>> = {}) {
  return render(
    <AgentDefForm isCreate={true} onSubmit={vi.fn()} onCancel={vi.fn()} {...props} />
  )
}

function getExecutionModeButton() {
  const label = screen.getByText('Execution Mode')
  return label.parentElement!.querySelector('button[type="button"]') as HTMLButtonElement
}

async function selectDropdownOption(
  user: ReturnType<typeof userEvent.setup>,
  trigger: HTMLButtonElement,
  label: string
) {
  await user.click(trigger)
  await user.click(screen.getByText(label))
}

beforeEach(() => {
  mockUseAPIModeEnabled.mockReturnValue(true)
})

describe('AgentDefForm — cli_interactive execution mode', () => {
  describe('dropdown option list when apiModeEnabled=true', () => {
    it('renders 4 options in order', async () => {
      const user = userEvent.setup()
      renderForm()

      const trigger = getExecutionModeButton()
      await user.click(trigger)

      const container = trigger.parentElement!.querySelector('.absolute')!
      const texts = Array.from(container.querySelectorAll('.truncate')).map(el => el.textContent)
      expect(texts).toEqual([
        'CLI (default)',
        'CLI Interactive (PTY)',
        'API (in-process Anthropic runner)',
        'Script (Python)',
      ])
    })

    it('cli_interactive is at position 2 (index 1)', async () => {
      const user = userEvent.setup()
      renderForm()

      const trigger = getExecutionModeButton()
      await user.click(trigger)

      const container = trigger.parentElement!.querySelector('.absolute')!
      const options = container.querySelectorAll('.cursor-pointer')
      expect(options[1].querySelector('.truncate')?.textContent).toBe('CLI Interactive (PTY)')
    })
  })

  describe('field visibility after selecting cli_interactive', () => {
    it('keeps Model label visible', async () => {
      const user = userEvent.setup()
      renderForm()

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      expect(screen.getByText('Model')).toBeInTheDocument()
    })

    it('keeps Prompt Template editor visible', async () => {
      const user = userEvent.setup()
      renderForm()

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      expect(screen.getByPlaceholderText(/agent prompt template/i)).toBeInTheDocument()
    })

    it('does not show Python Script picker', async () => {
      const user = userEvent.setup()
      renderForm()

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      expect(screen.queryByLabelText(/python script/i)).not.toBeInTheDocument()
    })

    it('keeps Low consumption model visible', async () => {
      const user = userEvent.setup()
      renderForm()

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      expect(screen.getByText('Low consumption model')).toBeInTheDocument()
    })
  })

  describe('form submission with cli_interactive', () => {
    it('submits execution_mode: cli_interactive with model and prompt', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'My prompt')

      await user.click(screen.getByRole('button', { name: /create/i }))

      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'my-agent',
          execution_mode: 'cli_interactive',
          model: 'sonnet',
          prompt: 'My prompt',
        })
      )
    })

    it('payload does not contain python_script_id', async () => {
      const user = userEvent.setup()
      const onSubmit = vi.fn()
      renderForm({ isCreate: true, onSubmit })

      await selectDropdownOption(user, getExecutionModeButton(), 'CLI Interactive (PTY)')

      await user.type(screen.getByPlaceholderText(/e\.g\., setup-analyzer/i), 'my-agent')
      await user.type(screen.getByPlaceholderText(/agent prompt template/i), 'My prompt')

      await user.click(screen.getByRole('button', { name: /create/i }))

      const payload = onSubmit.mock.calls[0][0] as Record<string, unknown>
      expect(payload).not.toHaveProperty('python_script_id')
    })

    it('initializes correctly from initial prop with cli_interactive', () => {
      renderForm({
        isCreate: false,
        initial: { execution_mode: 'cli_interactive', prompt: 'Existing prompt' },
      })

      expect(getExecutionModeButton().textContent).toContain('CLI Interactive (PTY)')
      expect(screen.getByText('Model')).toBeInTheDocument()
      expect(screen.getByPlaceholderText(/agent prompt template/i)).toHaveValue('Existing prompt')
    })
  })

  describe('dropdown option list when apiModeEnabled=false', () => {
    it('renders 3 options with cli_interactive at index 1', async () => {
      mockUseAPIModeEnabled.mockReturnValue(false)
      const user = userEvent.setup()
      renderForm()

      const trigger = getExecutionModeButton()
      await user.click(trigger)

      const container = trigger.parentElement!.querySelector('.absolute')!
      const texts = Array.from(container.querySelectorAll('.truncate')).map(el => el.textContent)
      expect(texts).toEqual([
        'CLI (default)',
        'CLI Interactive (PTY)',
        'Script (Python)',
      ])

      const options = container.querySelectorAll('.cursor-pointer')
      expect(options[1].querySelector('.truncate')?.textContent).toBe('CLI Interactive (PTY)')
    })
  })
})
