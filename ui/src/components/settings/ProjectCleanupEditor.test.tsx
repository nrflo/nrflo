import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectCleanupEditor } from './ProjectCleanupEditor'
import * as api from '@/api/projectSettings'
import { renderWithQuery } from '@/test/utils'

vi.mock('@/api/projectSettings')

const PROJECT_ID = 'proj-1'

beforeEach(() => vi.clearAllMocks())

describe('ProjectCleanupEditor', () => {
  it('default loads with cleanup disabled and hides retention input', async () => {
    vi.mocked(api.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })

    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} />)

    const toggle = await screen.findByRole('switch', { name: /enable cleanup/i })
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByPlaceholderText('e.g. 1000')).not.toBeInTheDocument()
    expect(screen.getByText(/kept indefinitely/i)).toBeInTheDocument()
  })

  it('toggling enabled reveals the retention limit input', async () => {
    vi.mocked(api.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })

    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} />)
    await screen.findByRole('switch', { name: /enable cleanup/i })

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch', { name: /enable cleanup/i }))

    expect(screen.getByRole('switch', { name: /enable cleanup/i })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByPlaceholderText('e.g. 1000')).toBeInTheDocument()
    expect(screen.queryByText(/kept indefinitely/i)).not.toBeInTheDocument()
  })

  it('submit posts correct payload when enabled with custom retention limit', async () => {
    vi.mocked(api.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })
    vi.mocked(api.setCleanup).mockResolvedValue({ enabled: true, retention_limit: 1000 })

    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} />)
    await screen.findByRole('switch', { name: /enable cleanup/i })

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch', { name: /enable cleanup/i }))

    const retentionInput = screen.getByPlaceholderText('e.g. 1000')
    await user.clear(retentionInput)
    await user.type(retentionInput, '1000')

    await user.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(api.setCleanup).toHaveBeenCalledWith(PROJECT_ID, { enabled: true, retention_limit: 1000 })
    })
  })

  it('server error is rendered verbatim', async () => {
    vi.mocked(api.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })
    vi.mocked(api.setCleanup).mockRejectedValue(new Error('retention_limit must be positive'))

    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} />)
    await screen.findByRole('switch', { name: /enable cleanup/i })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /save/i }))

    expect(await screen.findByText('retention_limit must be positive')).toBeInTheDocument()
  })

  it('shows Saved confirmation after successful submit', async () => {
    vi.mocked(api.getCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })
    vi.mocked(api.setCleanup).mockResolvedValue({ enabled: false, retention_limit: 100 })

    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} />)
    await screen.findByRole('switch', { name: /enable cleanup/i })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /save/i }))

    expect(await screen.findByText('Saved.')).toBeInTheDocument()
  })
})
