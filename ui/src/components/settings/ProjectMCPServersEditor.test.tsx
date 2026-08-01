import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent, waitFor } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { ProjectMCPServersEditor } from './ProjectMCPServersEditor'
import * as api from '@/api/projectSettings'

vi.mock('@/api/projectSettings', () => ({
  getMCPServers: vi.fn(),
  setMCPServers: vi.fn(),
}))

const getMCPServers = vi.mocked(api.getMCPServers)
const setMCPServers = vi.mocked(api.setMCPServers)

describe('ProjectMCPServersEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getMCPServers.mockResolvedValue({ servers: null })
    setMCPServers.mockResolvedValue({ servers: null })
  })

  it('loads existing config into the textarea', async () => {
    getMCPServers.mockResolvedValue({ servers: { unity: { command: 'uv' } } })
    renderWithQuery(<ProjectMCPServersEditor projectId="p1" />)
    await waitFor(() => {
      expect(screen.getByLabelText('External MCP servers JSON')).toHaveValue(
        JSON.stringify({ unity: { command: 'uv' } }, null, 2),
      )
    })
  })

  it('shows a parse error and disables save on invalid JSON', async () => {
    renderWithQuery(<ProjectMCPServersEditor projectId="p1" />)
    const textarea = screen.getByLabelText('External MCP servers JSON')
    fireEvent.change(textarea, { target: { value: '{not json' } })
    expect(screen.getByText('invalid JSON')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save MCP Servers' })).toBeDisabled()
  })

  it('saves parsed servers', async () => {
    renderWithQuery(<ProjectMCPServersEditor projectId="p1" />)
    const textarea = screen.getByLabelText('External MCP servers JSON')
    fireEvent.change(textarea, { target: { value: '{"unity":{"command":"uv"}}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save MCP Servers' }))
    await waitFor(() => {
      expect(setMCPServers).toHaveBeenCalledWith('p1', { unity: { command: 'uv' } })
    })
  })

  it('saves null when cleared', async () => {
    getMCPServers.mockResolvedValue({ servers: { unity: { command: 'uv' } } })
    renderWithQuery(<ProjectMCPServersEditor projectId="p1" />)
    const textarea = screen.getByLabelText('External MCP servers JSON')
    await waitFor(() => expect(textarea).not.toHaveValue(''))
    fireEvent.change(textarea, { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save MCP Servers' }))
    await waitFor(() => {
      expect(setMCPServers).toHaveBeenCalledWith('p1', null)
    })
  })
})
