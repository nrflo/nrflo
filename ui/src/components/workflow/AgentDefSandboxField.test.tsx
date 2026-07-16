import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentDefSandboxField } from './AgentDefSandboxField'

describe('AgentDefSandboxField', () => {
  it('shows the default label for an empty value', () => {
    render(<AgentDefSandboxField value="" onChange={vi.fn()} />)
    expect(screen.getByText('Full access (default)')).toBeInTheDocument()
  })

  it('selecting read-only emits the enum value', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<AgentDefSandboxField value="" onChange={onChange} />)
    await user.click(screen.getByText('Full access (default)'))
    await user.click(screen.getByText('Read-only (no file writes)'))
    expect(onChange).toHaveBeenCalledWith('read-only')
  })
})
