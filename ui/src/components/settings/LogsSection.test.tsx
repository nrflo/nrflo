import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LogsSection } from './LogsSection'
import * as logsHook from '@/hooks/useLogs'
import { renderWithQuery } from '@/test/utils'
import type { UseQueryResult } from '@tanstack/react-query'
import type { LogsResponse } from '@/types/logs'

vi.mock('@/hooks/useLogs')

function makeLogsResult(
  overrides: Partial<UseQueryResult<LogsResponse, Error>> = {}
): UseQueryResult<LogsResponse, Error> {
  return {
    data: undefined,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    isError: false,
    isPending: false,
    isSuccess: false,
    status: 'pending',
    fetchStatus: 'idle',
    ...overrides,
  } as unknown as UseQueryResult<LogsResponse, Error>
}

describe('LogsSection', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders BE and FE tab buttons, calls useLogs("be") by default', () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({ data: { lines: [], type: 'be' } })
    )
    renderWithQuery(<LogsSection />)
    expect(screen.getByRole('button', { name: /BE/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /FE/ })).toBeInTheDocument()
    expect(logsHook.useLogs).toHaveBeenCalledWith('be')
  })

  it('switches to FE tab on click and calls useLogs("fe")', async () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({ data: { lines: [], type: 'be' } })
    )
    renderWithQuery(<LogsSection />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /FE/ }))
    expect(logsHook.useLogs).toHaveBeenCalledWith('fe')
  })

  it('shows loading spinner when isLoading', () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(makeLogsResult({ isLoading: true }))
    renderWithQuery(<LogsSection />)
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('shows error message and retry button; retry calls refetch', async () => {
    const refetch = vi.fn()
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({ error: new Error('connection refused'), refetch })
    )
    renderWithQuery(<LogsSection />)

    expect(screen.getByText(/Failed to load logs/)).toBeInTheDocument()
    expect(screen.getByText(/connection refused/)).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Retry/i }))
    expect(refetch).toHaveBeenCalled()
  })

  it('shows empty state when lines array is empty', () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({ data: { lines: [], type: 'be' } })
    )
    renderWithQuery(<LogsSection />)
    expect(screen.getByText('No log lines available.')).toBeInTheDocument()
  })

  it('renders each log line', () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({
        data: {
          lines: ['INFO server started', 'DEBUG processing request'],
          type: 'be',
        },
      })
    )
    renderWithQuery(<LogsSection />)
    expect(screen.getByText('INFO server started')).toBeInTheDocument()
    expect(screen.getByText('DEBUG processing request')).toBeInTheDocument()
  })
})
