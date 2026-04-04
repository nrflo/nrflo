import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SafetyHookCheckDialog } from './SafetyHookCheckDialog'
import { emptyProjectForm } from './ProjectForm'
import type { ProjectFormData } from './ProjectForm'
import * as projectsApi from '@/api/projects'

vi.mock('@/api/projects')

function makeFormData(overrides: Partial<ProjectFormData> = {}): ProjectFormData {
  return {
    ...emptyProjectForm,
    safety_hook_enabled: true,
    safety_hook_allow_git: false,
    safety_hook_allowed_rm_paths: 'node_modules\ndist',
    safety_hook_dangerous_patterns: 'DROP TABLE\nrm -rf /',
    ...overrides,
  }
}

function renderDialog(
  formData: ProjectFormData = makeFormData(),
  open = true,
  onClose = vi.fn()
) {
  return render(<SafetyHookCheckDialog open={open} onClose={onClose} formData={formData} />)
}

describe('SafetyHookCheckDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders command input and Execute button', () => {
    renderDialog()
    expect(screen.getByPlaceholderText('ls -la')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /execute/i })).toBeInTheDocument()
  })

  it('Execute button is disabled when command input is empty', () => {
    renderDialog()
    expect(screen.getByRole('button', { name: /execute/i })).toBeDisabled()
  })

  it('Execute button is enabled once command is entered', async () => {
    const user = userEvent.setup()
    renderDialog()
    await user.type(screen.getByPlaceholderText('ls -la'), 'ls -la')
    expect(screen.getByRole('button', { name: /execute/i })).not.toBeDisabled()
  })

  it('shows config summary: dangerous patterns count', () => {
    renderDialog()
    expect(screen.getByText(/2 dangerous patterns configured/i)).toBeInTheDocument()
  })

  it('shows config summary: allowed rm paths count', () => {
    renderDialog()
    expect(screen.getByText(/2 allowed rm paths/i)).toBeInTheDocument()
  })

  it('shows config summary: git operations blocked', () => {
    renderDialog(makeFormData({ safety_hook_allow_git: false }))
    expect(screen.getByText(/git operations:.*blocked/i)).toBeInTheDocument()
  })

  it('shows config summary: git operations allowed', () => {
    renderDialog(makeFormData({ safety_hook_allow_git: true }))
    expect(screen.getByText(/git operations:.*allowed/i)).toBeInTheDocument()
  })

  it('shows "Allowed" on successful allowed response', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockResolvedValue({ allowed: true, reason: '' })
    const user = userEvent.setup()
    renderDialog()

    await user.type(screen.getByPlaceholderText('ls -la'), 'ls -la')
    await user.click(screen.getByRole('button', { name: /execute/i }))

    expect(await screen.findByText('Allowed')).toBeInTheDocument()
  })

  it('shows "Blocked" with reason on blocked response', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockResolvedValue({
      allowed: false,
      reason: 'Blocked: dangerous pattern matched',
    })
    const user = userEvent.setup()
    renderDialog()

    await user.type(screen.getByPlaceholderText('ls -la'), 'rm -rf /')
    await user.click(screen.getByRole('button', { name: /execute/i }))

    expect(await screen.findByText(/Blocked: dangerous pattern matched/i)).toBeInTheDocument()
  })

  it('shows error message when API throws', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockRejectedValue(new Error('jq not found'))
    const user = userEvent.setup()
    renderDialog()

    await user.type(screen.getByPlaceholderText('ls -la'), 'ls')
    await user.click(screen.getByRole('button', { name: /execute/i }))

    expect(await screen.findByText('jq not found')).toBeInTheDocument()
  })

  it('disables Execute button while request is in flight', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderDialog()

    await user.type(screen.getByPlaceholderText('ls -la'), 'ls')
    await user.click(screen.getByRole('button', { name: /execute/i }))

    expect(screen.getByRole('button', { name: /execute/i })).toBeDisabled()
  })

  it('calls checkSafetyHook with trimmed command and parsed config', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockResolvedValue({ allowed: true, reason: '' })
    const user = userEvent.setup()
    const formData = makeFormData()
    renderDialog(formData)

    await user.type(screen.getByPlaceholderText('ls -la'), '  ls -la  ')
    await user.click(screen.getByRole('button', { name: /execute/i }))

    await screen.findByText('Allowed')
    expect(projectsApi.checkSafetyHook).toHaveBeenCalledWith({
      config: {
        enabled: true,
        allow_git: false,
        rm_rf_allowed_paths: ['node_modules', 'dist'],
        dangerous_patterns: ['DROP TABLE', 'rm -rf /'],
      },
      command: 'ls -la',
    })
  })

  it('Enter key triggers execute when command is non-empty', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockResolvedValue({ allowed: true, reason: '' })
    const user = userEvent.setup()
    renderDialog()

    const input = screen.getByPlaceholderText('ls -la')
    await user.type(input, 'ls{Enter}')

    await screen.findByText('Allowed')
    expect(projectsApi.checkSafetyHook).toHaveBeenCalledTimes(1)
  })

  it('Enter key does nothing when command is empty', async () => {
    const user = userEvent.setup()
    renderDialog()

    const input = screen.getByPlaceholderText('ls -la')
    await user.click(input)
    await user.keyboard('{Enter}')

    expect(projectsApi.checkSafetyHook).not.toHaveBeenCalled()
  })

  it('resets state and calls onClose when Close button clicked', async () => {
    vi.mocked(projectsApi.checkSafetyHook).mockResolvedValue({ allowed: true, reason: '' })
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderDialog(makeFormData(), true, onClose)

    await user.type(screen.getByPlaceholderText('ls -la'), 'ls')
    await user.click(screen.getByRole('button', { name: /execute/i }))
    await screen.findByText('Allowed')

    await user.click(screen.getByRole('button', { name: /close/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows singular "pattern" for exactly 1 dangerous pattern', () => {
    renderDialog(makeFormData({ safety_hook_dangerous_patterns: 'DROP TABLE' }))
    expect(screen.getByText(/1 dangerous pattern configured/i)).toBeInTheDocument()
  })

  it('shows singular "path" for exactly 1 rm path', () => {
    renderDialog(makeFormData({ safety_hook_allowed_rm_paths: 'node_modules' }))
    expect(screen.getByText(/1 allowed rm path$/i)).toBeInTheDocument()
  })
})
