import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  ProjectForm,
  emptyProjectForm,
  parseSafetyHookConfig,
  buildSafetyHookJSON,
} from './ProjectForm'

function makeMutation(overrides = {}) {
  return { isPending: false, isError: false, error: null, ...overrides }
}

function makeProps(formDataOverrides = {}) {
  return {
    formData: { ...emptyProjectForm, ...formDataOverrides },
    setFormData: vi.fn(),
    onCancel: vi.fn(),
    onSave: vi.fn(),
    mutation: makeMutation(),
  }
}

describe('ProjectForm — default branch tooltip', () => {
  it('shows info tooltip on hover over default branch info icon', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps()} />)

    const label = screen.getByText('Default Branch')
    const tooltipTrigger = label.closest('.flex')?.querySelector('[data-state]') as HTMLElement
    await user.hover(tooltipTrigger)

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Git page/)
    expect(tooltip).toHaveTextContent(/worktrees are enabled/)
  })
})

describe('ProjectForm — worktree tooltip', () => {
  it('shows worktree explanation tooltip on hover', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()

    await user.hover(screen.getByText('Use Git Worktrees'))

    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Git worktrees give each ticket/)
    expect(tooltip).toHaveTextContent(/Applies to ticket-scoped workflows only/)
    expect(tooltip).toHaveTextContent(/Lifecycle: creates a feature branch/)
  })

  it('tooltip is accessible via role on hover', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    await user.hover(screen.getByText('Use Git Worktrees'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Git worktrees give each ticket/)
  })

  it('tooltip applies max-w-sm class', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    await user.hover(screen.getByText('Use Git Worktrees'))
    await screen.findByRole('tooltip')

    const tooltip = document.querySelector('[data-side]')
    expect(tooltip).toHaveClass('max-w-sm')
  })
})

describe('ProjectForm — worktree toggle state', () => {
  it('disables toggle when default_branch is empty', () => {
    render(<ProjectForm {...makeProps({ default_branch: '' })} />)

    const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
    expect(toggle).toBeDisabled()
  })

  it('enables toggle when default_branch is set', () => {
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)

    const toggle = screen.getByRole('switch', { name: /use git worktrees/i })
    expect(toggle).not.toBeDisabled()
  })
})

describe('ProjectForm — push after merge toggle', () => {
  it('disables toggle when default_branch is empty', () => {
    render(<ProjectForm {...makeProps({ default_branch: '' })} />)
    expect(screen.getByRole('switch', { name: /push after merge/i })).toBeDisabled()
  })

  it('enables toggle when default_branch is set', () => {
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)
    expect(screen.getByRole('switch', { name: /push after merge/i })).not.toBeDisabled()
  })

  it('reflects checked state true from formData', () => {
    render(<ProjectForm {...makeProps({ default_branch: 'main', push_after_merge: true })} />)
    expect(screen.getByRole('switch', { name: /push after merge/i })).toHaveAttribute('aria-checked', 'true')
  })

  it('reflects checked state false from formData', () => {
    render(<ProjectForm {...makeProps({ default_branch: 'main', push_after_merge: false })} />)
    expect(screen.getByRole('switch', { name: /push after merge/i })).toHaveAttribute('aria-checked', 'false')
  })

  it('calls setFormData with push_after_merge toggled on click', async () => {
    const user = userEvent.setup()
    const props = makeProps({ default_branch: 'main', push_after_merge: false })
    render(<ProjectForm {...props} />)
    await user.click(screen.getByRole('switch', { name: /push after merge/i }))
    expect(props.setFormData).toHaveBeenCalledWith(expect.objectContaining({ push_after_merge: true }))
  })

  it('shows tooltip on hover', async () => {
    const user = userEvent.setup()
    render(<ProjectForm {...makeProps({ default_branch: 'main' })} />)
    await user.hover(screen.getByText('Push after merge'))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(/Automatically push default branch/)
  })
})

describe('parseSafetyHookConfig', () => {
  it('returns disabled defaults for null', () => {
    expect(parseSafetyHookConfig(null)).toEqual({
      safety_hook_enabled: false,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: '',
      safety_hook_dangerous_patterns: '',
    })
  })

  it('returns disabled defaults for empty string', () => {
    expect(parseSafetyHookConfig('')).toEqual({
      safety_hook_enabled: false,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: '',
      safety_hook_dangerous_patterns: '',
    })
  })

  it('parses valid JSON into form fields', () => {
    const json = JSON.stringify({
      enabled: true,
      allow_git: false,
      rm_rf_allowed_paths: ['node_modules', 'dist'],
      dangerous_patterns: ['rm -rf /'],
    })
    expect(parseSafetyHookConfig(json)).toEqual({
      safety_hook_enabled: true,
      safety_hook_allow_git: false,
      safety_hook_allowed_rm_paths: 'node_modules\ndist',
      safety_hook_dangerous_patterns: 'rm -rf /',
    })
  })

  it('uses defaults for missing fields', () => {
    const result = parseSafetyHookConfig(JSON.stringify({ enabled: true }))
    expect(result.safety_hook_enabled).toBe(true)
    expect(result.safety_hook_allow_git).toBe(true)
    expect(result.safety_hook_allowed_rm_paths).toBe('')
    expect(result.safety_hook_dangerous_patterns).toBe('')
  })

  it('returns disabled defaults for invalid JSON', () => {
    expect(parseSafetyHookConfig('not json')).toEqual({
      safety_hook_enabled: false,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: '',
      safety_hook_dangerous_patterns: '',
    })
  })
})

describe('buildSafetyHookJSON', () => {
  it('returns empty string when disabled', () => {
    expect(buildSafetyHookJSON({ ...emptyProjectForm, safety_hook_enabled: false })).toBe('')
  })

  it('builds correct JSON when enabled', () => {
    const formData = {
      ...emptyProjectForm,
      safety_hook_enabled: true,
      safety_hook_allow_git: false,
      safety_hook_allowed_rm_paths: 'node_modules\ndist',
      safety_hook_dangerous_patterns: 'rm -rf /',
    }
    expect(JSON.parse(buildSafetyHookJSON(formData))).toEqual({
      enabled: true,
      allow_git: false,
      rm_rf_allowed_paths: ['node_modules', 'dist'],
      dangerous_patterns: ['rm -rf /'],
    })
  })

  it('filters empty lines from textareas', () => {
    const formData = {
      ...emptyProjectForm,
      safety_hook_enabled: true,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: 'node_modules\n\ndist\n',
      safety_hook_dangerous_patterns: '',
    }
    const result = JSON.parse(buildSafetyHookJSON(formData))
    expect(result.rm_rf_allowed_paths).toEqual(['node_modules', 'dist'])
    expect(result.dangerous_patterns).toEqual([])
  })

  it('handles Windows line endings', () => {
    const formData = {
      ...emptyProjectForm,
      safety_hook_enabled: true,
      safety_hook_allow_git: true,
      safety_hook_allowed_rm_paths: 'node_modules\r\ndist',
      safety_hook_dangerous_patterns: '',
    }
    const result = JSON.parse(buildSafetyHookJSON(formData))
    expect(result.rm_rf_allowed_paths).toEqual(['node_modules', 'dist'])
  })
})

describe('ProjectForm — safety hook section', () => {
  it('hides safety hook fields when disabled', () => {
    render(<ProjectForm {...makeProps({ safety_hook_enabled: false })} />)
    expect(screen.queryByRole('switch', { name: /allow git operations/i })).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText(/node_modules/i)).not.toBeInTheDocument()
  })

  it('shows all safety hook fields when enabled', () => {
    render(<ProjectForm {...makeProps({ safety_hook_enabled: true })} />)
    expect(screen.getByRole('switch', { name: /allow git operations/i })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/node_modules/i)).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/rm -rf/i)).toBeInTheDocument()
  })

  it('shows Check button when safety hook is enabled', () => {
    render(<ProjectForm {...makeProps({ safety_hook_enabled: true })} />)
    expect(screen.getByRole('button', { name: /check/i })).toBeInTheDocument()
  })

  it('hides Check button when safety hook is disabled', () => {
    render(<ProjectForm {...makeProps({ safety_hook_enabled: false })} />)
    expect(screen.queryByRole('button', { name: /check/i })).not.toBeInTheDocument()
  })

  it('enabling toggle populates default rm paths when empty', async () => {
    const user = userEvent.setup()
    const setFormData = vi.fn()
    render(
      <ProjectForm
        {...makeProps({ safety_hook_enabled: false, safety_hook_allowed_rm_paths: '' })}
        setFormData={setFormData}
      />
    )
    await user.click(screen.getByRole('switch', { name: /enable safety hook/i }))
    expect(setFormData).toHaveBeenCalledWith(
      expect.objectContaining({
        safety_hook_enabled: true,
        safety_hook_allowed_rm_paths: expect.stringContaining('node_modules'),
      })
    )
  })

  it('enabling toggle preserves existing rm paths', async () => {
    const user = userEvent.setup()
    const setFormData = vi.fn()
    render(
      <ProjectForm
        {...makeProps({ safety_hook_enabled: false, safety_hook_allowed_rm_paths: 'custom-dir' })}
        setFormData={setFormData}
      />
    )
    await user.click(screen.getByRole('switch', { name: /enable safety hook/i }))
    expect(setFormData).toHaveBeenCalledWith(
      expect.objectContaining({
        safety_hook_enabled: true,
        safety_hook_allowed_rm_paths: 'custom-dir',
      })
    )
  })
})
