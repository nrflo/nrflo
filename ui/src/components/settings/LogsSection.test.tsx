import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, act, fireEvent } from '@testing-library/react'
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
    expect(logsHook.useLogs).toHaveBeenCalledWith('be', undefined)
  })

  it('switches to FE tab on click and calls useLogs("fe")', async () => {
    vi.mocked(logsHook.useLogs).mockReturnValue(
      makeLogsResult({ data: { lines: [], type: 'be' } })
    )
    renderWithQuery(<LogsSection />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /FE/ }))
    expect(logsHook.useLogs).toHaveBeenCalledWith('fe', undefined)
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

  describe('filter input', () => {
    it('renders filter input with placeholder and default indicator', () => {
      vi.mocked(logsHook.useLogs).mockReturnValue(makeLogsResult({ data: { lines: [], type: 'be' } }))
      renderWithQuery(<LogsSection />)
      expect(screen.getByPlaceholderText('Filter logs...')).toBeInTheDocument()
      expect(screen.getByText('Showing last 1000 lines')).toBeInTheDocument()
    })

    it('initialFilter pre-populates input, shows matching indicator, passes filter to useLogs', () => {
      vi.mocked(logsHook.useLogs).mockReturnValue(makeLogsResult({ data: { lines: [], type: 'be' } }))
      renderWithQuery(<LogsSection initialFilter="abc12345" />)
      expect(screen.getByDisplayValue('abc12345')).toBeInTheDocument()
      expect(screen.getByText('Showing all matching lines')).toBeInTheDocument()
      expect(logsHook.useLogs).toHaveBeenCalledWith('be', 'abc12345')
    })

    it('debounces filter input 300ms before passing to useLogs', async () => {
      vi.useFakeTimers()
      vi.mocked(logsHook.useLogs).mockReturnValue(makeLogsResult({ data: { lines: [], type: 'be' } }))
      renderWithQuery(<LogsSection />)

      const input = screen.getByPlaceholderText('Filter logs...')
      fireEvent.change(input, { target: { value: 'abc' } })

      // Before debounce fires: filter not yet passed to useLogs
      expect(vi.mocked(logsHook.useLogs).mock.calls.every((c) => c[1] !== 'abc')).toBe(true)

      await act(async () => { vi.advanceTimersByTime(300) })
      expect(logsHook.useLogs).toHaveBeenCalledWith('be', 'abc')
    })

    afterEach(() => {
      vi.useRealTimers()
    })
  })
})
