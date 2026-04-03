import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ErrorsPage } from './ErrorsPage'
import type { ErrorLog, ErrorsResponse } from '@/types/errors'

const mockUseErrors = vi.fn()
vi.mock('@/hooks/useErrors', () => ({
  useErrors: (params?: any) => mockUseErrors(params),
}))

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async (importOriginal) => {
  const mod = await importOriginal<typeof import('react-router-dom')>()
  return { ...mod, useNavigate: () => mockNavigate }
})

function makeErrorLog(overrides: Partial<ErrorLog> = {}): ErrorLog {
  return {
    id: 'err-001',
    project_id: 'proj-1',
    error_type: 'agent',
    instance_id: 'abcdef1234567890',
    message: 'Agent failed',
    created_at: '2026-01-15T10:30:00Z',
    ...overrides,
  }
}

function makeResponse(overrides: Partial<ErrorsResponse> = {}): ErrorsResponse {
  return {
    errors: [],
    total: 0,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

function renderPage(url = '/errors') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <ErrorsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('ErrorsPage', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows loading state', () => {
    mockUseErrors.mockReturnValue({ data: undefined, isLoading: true })
    renderPage()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  it('shows empty state when no errors', () => {
    mockUseErrors.mockReturnValue({ data: makeResponse(), isLoading: false })
    renderPage()
    expect(screen.getByText('No errors recorded')).toBeInTheDocument()
  })

  it('renders table with type badge, truncated instance, message, and date', () => {
    mockUseErrors.mockReturnValue({
      data: makeResponse({
        errors: [makeErrorLog({ error_type: 'agent', message: 'Agent timeout', instance_id: 'abcdef1234567890' })],
        total: 1,
      }),
      isLoading: false,
    })
    renderPage()
    expect(screen.getByText('agent')).toBeInTheDocument()
    expect(screen.getAllByText('abcdef12').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('Agent timeout')).toBeInTheDocument()
  })

  it('truncates instance_id to first 8 characters', () => {
    mockUseErrors.mockReturnValue({
      data: makeResponse({ errors: [makeErrorLog({ instance_id: 'abcdef1234567890' })], total: 1 }),
      isLoading: false,
    })
    renderPage()
    expect(screen.getAllByText('abcdef12').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('abcdef1234567890')).not.toBeInTheDocument()
  })

  it('renders badges for all three error types', () => {
    mockUseErrors.mockReturnValue({
      data: makeResponse({
        errors: [
          makeErrorLog({ id: '1', error_type: 'agent',    message: 'msg1', instance_id: '1111111111111111' }),
          makeErrorLog({ id: '2', error_type: 'workflow',  message: 'msg2', instance_id: '2222222222222222' }),
          makeErrorLog({ id: '3', error_type: 'system',   message: 'msg3', instance_id: '3333333333333333' }),
        ],
        total: 3,
      }),
      isLoading: false,
    })
    renderPage()
    expect(screen.getByText('agent')).toBeInTheDocument()
    expect(screen.getByText('workflow')).toBeInTheDocument()
    expect(screen.getByText('system')).toBeInTheDocument()
  })

  describe('type filter tabs', () => {
    beforeEach(() => {
      mockUseErrors.mockReturnValue({ data: makeResponse(), isLoading: false })
    })

    it('renders All / Agent / Workflow / System tabs', () => {
      renderPage()
      expect(screen.getByText('All')).toBeInTheDocument()
      expect(screen.getByText('Agent')).toBeInTheDocument()
      expect(screen.getByText('Workflow')).toBeInTheDocument()
      expect(screen.getByText('System')).toBeInTheDocument()
    })

    it('clicking Agent tab calls useErrors with type=agent', async () => {
      const user = userEvent.setup()
      renderPage()
      await user.click(screen.getByText('Agent'))
      const lastCall = mockUseErrors.mock.calls.at(-1)
      expect(lastCall?.[0]?.type).toBe('agent')
    })

    it('clicking Workflow tab calls useErrors with type=workflow', async () => {
      const user = userEvent.setup()
      renderPage()
      await user.click(screen.getByText('Workflow'))
      const lastCall = mockUseErrors.mock.calls.at(-1)
      expect(lastCall?.[0]?.type).toBe('workflow')
    })

    it('clicking System tab calls useErrors with type=system', async () => {
      const user = userEvent.setup()
      renderPage()
      await user.click(screen.getByText('System'))
      const lastCall = mockUseErrors.mock.calls.at(-1)
      expect(lastCall?.[0]?.type).toBe('system')
    })

    it('clicking All clears the type filter', async () => {
      const user = userEvent.setup()
      renderPage('/errors?type=agent')
      await user.click(screen.getByText('All'))
      const lastCall = mockUseErrors.mock.calls.at(-1)
      expect(lastCall?.[0]?.type).toBeUndefined()
    })
  })

  describe('pagination', () => {
    it('shows X–Y of Z for multi-page results', () => {
      const errors = Array.from({ length: 20 }, (_, i) =>
        makeErrorLog({ id: `e${i}`, message: `Msg ${i}`, instance_id: `${i}`.padStart(16, '0') })
      )
      mockUseErrors.mockReturnValue({
        data: makeResponse({ errors, total: 45, page: 1, per_page: 20, total_pages: 3 }),
        isLoading: false,
      })
      renderPage()
      expect(screen.getByText('1–20 of 45')).toBeInTheDocument()
    })

    it('does not show pagination footer for single page', () => {
      mockUseErrors.mockReturnValue({
        data: makeResponse({ errors: [makeErrorLog()], total: 1, total_pages: 1 }),
        isLoading: false,
      })
      renderPage()
      expect(screen.queryByText(/of \d+/)).not.toBeInTheDocument()
    })

    it('clicking next button increments page', async () => {
      const user = userEvent.setup()
      const errors = Array.from({ length: 20 }, (_, i) =>
        makeErrorLog({ id: `e${i}`, message: `Msg ${i}`, instance_id: `${i}`.padStart(16, '0') })
      )
      mockUseErrors.mockReturnValue({
        data: makeResponse({ errors, total: 40, page: 1, per_page: 20, total_pages: 2 }),
        isLoading: false,
      })
      renderPage()
      // Next button is last among all buttons (tabs: All Agent Workflow System, then Prev Next)
      const buttons = screen.getAllByRole('button')
      const nextButton = buttons[buttons.length - 1]
      await user.click(nextButton)
      expect(mockUseErrors.mock.calls.some((call: any) => call[0]?.page === 2)).toBe(true)
    })
  })

  it('clicking a row calls navigate to /project-workflows', async () => {
    const user = userEvent.setup()
    mockUseErrors.mockReturnValue({
      data: makeResponse({
        errors: [makeErrorLog({ message: 'click me error' })],
        total: 1,
      }),
      isLoading: false,
    })
    renderPage()
    await user.click(screen.getByText('click me error'))
    expect(mockNavigate).toHaveBeenCalledWith('/project-workflows')
  })

  describe('SID column', () => {
    it('renders SID header and shows truncated session ID as link for agent errors, em-dash for others', async () => {
      const user = userEvent.setup()
      mockUseErrors.mockReturnValue({
        data: makeResponse({
          errors: [
            makeErrorLog({ id: '1', error_type: 'agent',    instance_id: 'aabbccdd11223344' }),
            makeErrorLog({ id: '2', error_type: 'workflow', instance_id: 'wfwfwfwf11223344' }),
            makeErrorLog({ id: '3', error_type: 'system',   instance_id: 'syssyssys1223344' }),
          ],
          total: 3,
        }),
        isLoading: false,
      })
      renderPage()

      expect(screen.getByRole('columnheader', { name: 'SID' })).toBeInTheDocument()

      // Agent row: SID is a clickable button showing first 8 chars
      const sidLink = screen.getByRole('button', { name: 'aabbccdd' })
      expect(sidLink).toBeInTheDocument()

      // Non-agent rows: SID is em-dash (×2: workflow + system)
      const emDashes = screen.getAllByText('\u2014')
      expect(emDashes).toHaveLength(2)

      // Clicking SID navigates to logs with full instance_id as filter
      await user.click(sidLink)
      expect(mockNavigate).toHaveBeenCalledWith(
        '/settings?tab=logs&filter=aabbccdd11223344'
      )
    })

    it('SID click does not trigger row navigation (stopPropagation)', async () => {
      const user = userEvent.setup()
      mockUseErrors.mockReturnValue({
        data: makeResponse({
          errors: [makeErrorLog({ error_type: 'agent', instance_id: 'aabbccdd11223344' })],
          total: 1,
        }),
        isLoading: false,
      })
      renderPage()

      mockNavigate.mockClear()
      await user.click(screen.getByRole('button', { name: 'aabbccdd' }))

      // Only the SID navigate call, not the row click /project-workflows
      expect(mockNavigate).toHaveBeenCalledTimes(1)
      expect(mockNavigate).toHaveBeenCalledWith('/settings?tab=logs&filter=aabbccdd11223344')
    })
  })
})
