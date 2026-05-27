import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { AgentDefToolsField } from './AgentDefToolsField'

vi.mock('@/hooks/useAvailableTools', () => ({
  useAvailableTools: () => ({
    data: [
      { name: 'agent_finished', description: 'finish', source: 'builtin', mandatory: true },
      { name: 'findings_add', description: 'add finding', source: 'builtin', mandatory: false },
      { name: 'lookup_sku', description: 'sku', source: 'python', mandatory: false },
    ],
    isLoading: false,
  }),
}))

describe('AgentDefToolsField', () => {
  it('renders builtin and python tools', () => {
    render(<AgentDefToolsField value="" onChange={() => {}} executionMode="cli_interactive" />)
    expect(screen.getByText('built-in')).toBeInTheDocument()
    expect(screen.getByText('python')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /findings_add/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /lookup_sku/ })).toBeInTheDocument()
  })

  it('pins mandatory (lifecycle) tools as disabled', () => {
    render(<AgentDefToolsField value="findings_add" onChange={() => {}} executionMode="cli_interactive" />)
    expect(screen.getByRole('button', { name: /agent_finished/ })).toBeDisabled()
  })

  it('selecting a tool emits a CSV that always includes mandatory names', () => {
    const onChange = vi.fn()
    render(<AgentDefToolsField value="" onChange={onChange} executionMode="cli_interactive" />)
    fireEvent.click(screen.getByRole('button', { name: /findings_add/ }))
    expect(onChange).toHaveBeenCalledWith('agent_finished,findings_add')
  })

  it('All tools toggle emits *', () => {
    const onChange = vi.fn()
    render(<AgentDefToolsField value="" onChange={onChange} executionMode="cli_interactive" />)
    fireEvent.click(screen.getAllByRole('switch')[0]) // first switch is "All tools (*)"
    expect(onChange).toHaveBeenCalledWith('*')
  })

  it('warns when a raw glob pattern matches no known tool', () => {
    // A dotted pattern auto-opens advanced mode and matches nothing.
    render(<AgentDefToolsField value="findings.*" onChange={() => {}} executionMode="cli_interactive" />)
    expect(screen.getByText(/match no known tool/)).toBeInTheDocument()
  })
})
