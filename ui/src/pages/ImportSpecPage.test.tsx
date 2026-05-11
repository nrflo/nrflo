import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ImportSpecPage } from './ImportSpecPage'
import type { WSEvent } from '@/hooks/useWebSocket'
import { NotConfiguredError } from '@/api/specImport'

// ── Mocks ──────────────────────────────────────────────────────────────────

const mockNavigate = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

vi.mock('@/hooks/useWebSocketSubscription', () => ({
  useWebSocketSubscription: vi.fn().mockReturnValue({ isConnected: true }),
  useWebSocketEvent: vi.fn(),
}))

vi.mock('@/api/specImport', async () => {
  const actual = await vi.importActual('@/api/specImport')
  return {
    ...actual,
    startImport: vi.fn(),
    getImportPreview: vi.fn(),
    commitImport: vi.fn(),
    searchGitHubIssues: vi.fn(),
  }
})

vi.mock('@/api/workflows', () => ({
  listWorkflowDefs: vi.fn(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector: (s: Record<string, unknown>) => unknown) =>
    selector({ currentProject: 'proj-1', projectsLoaded: true })
  ),
}))

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// ── Helpers ────────────────────────────────────────────────────────────────

import { useWebSocketEvent } from '@/hooks/useWebSocketSubscription'
import * as specImportApi from '@/api/specImport'
import * as workflowsApi from '@/api/workflows'

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ImportSpecPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function captureWSHandler() {
  let handler: ((e: WSEvent) => void) | null = null
  vi.mocked(useWebSocketEvent).mockImplementation((h) => { handler = h })
  return () => handler
}

// ── Tests ──────────────────────────────────────────────────────────────────

describe('ImportSpecPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('markdown source — happy path', () => {
    it('walks through wizard: normalize → WS ready → preview → commit → navigate', async () => {
      const getHandler = captureWSHandler()

      vi.mocked(specImportApi.startImport).mockResolvedValue({ instance_id: 'inst-1' })
      vi.mocked(specImportApi.getImportPreview).mockResolvedValue({
        instance_id: 'inst-1',
        title: 'Fix the login bug',
        description: 'Login flow is broken',
        instructions: 'Fix the auth module',
        suggested_workflow: 'feature',
      })
      vi.mocked(specImportApi.commitImport).mockResolvedValue({ ticket_id: 'T-42' })
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue({
        feature: { description: 'Feature', scope_type: 'ticket', phases: [] },
      })

      renderPage()

      // Step 1: pick Markdown source
      fireEvent.click(screen.getByRole('radio', { name: /markdown \/ text/i }))
      fireEvent.click(screen.getByRole('button', { name: /next/i }))

      // Step 2: type body and normalize
      expect(await screen.findByPlaceholderText(/paste your spec content here/i)).toBeInTheDocument()
      fireEvent.change(screen.getByPlaceholderText(/paste your spec content here/i), {
        target: { value: 'Implement OAuth login' },
      })
      await userEvent.click(screen.getByRole('button', { name: /normalize/i }))

      await waitFor(() =>
        expect(specImportApi.startImport).toHaveBeenCalledWith({
          source: 'markdown',
          body: 'Implement OAuth login',
        })
      )

      // Simulate spec_import.ready WS event
      await act(async () => {
        getHandler()!({ type: 'spec_import.ready', data: { instance_id: 'inst-1' } } as WSEvent)
      })

      // Step 3: preview form
      await screen.findByText('Review & Edit')
      await waitFor(() => expect(specImportApi.getImportPreview).toHaveBeenCalledWith('inst-1'))

      expect(await screen.findByDisplayValue('Fix the login bug')).toBeInTheDocument()
      expect(screen.getByDisplayValue('Login flow is broken')).toBeInTheDocument()

      // Create Ticket button becomes enabled once workflow auto-selected
      const createBtn = await screen.findByRole('button', { name: /create ticket/i })
      await waitFor(() => expect(createBtn).not.toBeDisabled())

      await userEvent.click(createBtn)

      await waitFor(() =>
        expect(specImportApi.commitImport).toHaveBeenCalledWith('inst-1', {
          title: 'Fix the login bug',
          description: 'Login flow is broken',
          workflow_name: 'feature',
          instructions: 'Fix the auth module',
        })
      )
      expect(mockNavigate).toHaveBeenCalledWith('/tickets/T-42')
    })
  })

  describe('github source — 412 NotConfiguredError', () => {
    beforeEach(() => { vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }) })
    afterEach(() => { vi.useRealTimers() })

    it('shows inline config row with env-vars link when GITHUB_TOKEN missing', async () => {
      captureWSHandler()
      vi.mocked(specImportApi.searchGitHubIssues).mockRejectedValue(
        new NotConfiguredError(['GITHUB_TOKEN'])
      )

      renderPage()

      // Step 1: GitHub is default source — click Next
      fireEvent.click(screen.getByRole('button', { name: /next/i }))

      // Step 2: step change is synchronous — grab input without await
      const searchInput = screen.getByPlaceholderText(/search github issues/i)
      fireEvent.change(searchInput, { target: { value: 'abc' } })

      // Advance just past the 250ms debounce, then flush async rejection
      await act(async () => {
        vi.advanceTimersByTime(300)
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(specImportApi.searchGitHubIssues).toHaveBeenCalledWith('abc', undefined)
      expect(screen.getByText(/GITHUB_TOKEN/)).toBeInTheDocument()

      const link = screen.getByRole('link', { name: /configure in project settings/i })
      expect(link.getAttribute('href')).toContain('env-vars')
    })
  })

  describe('spec_import.failed WS event', () => {
    it('shows error message and Try Again resets to step 2 preserving body', async () => {
      const getHandler = captureWSHandler()
      vi.mocked(specImportApi.startImport).mockResolvedValue({ instance_id: 'inst-1' })
      vi.mocked(workflowsApi.listWorkflowDefs).mockResolvedValue({})

      renderPage()

      // Go to step 2 with Markdown
      fireEvent.click(screen.getByRole('radio', { name: /markdown \/ text/i }))
      fireEvent.click(screen.getByRole('button', { name: /next/i }))

      const textarea = await screen.findByPlaceholderText(/paste your spec content here/i)
      fireEvent.change(textarea, { target: { value: 'My spec body' } })

      await userEvent.click(screen.getByRole('button', { name: /normalize/i }))
      await waitFor(() => expect(specImportApi.startImport).toHaveBeenCalled())

      // Simulate spec_import.failed
      await act(async () => {
        getHandler()!({
          type: 'spec_import.failed',
          data: { instance_id: 'inst-1', error: 'Normalize failed: bad input' },
        } as WSEvent)
      })

      // Error message and Try Again button appear
      await screen.findByText('Normalize failed: bad input')
      expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()

      // Click Try Again
      fireEvent.click(screen.getByRole('button', { name: /try again/i }))

      // Still on step 2 with body preserved and error gone
      await waitFor(() => {
        expect(screen.queryByText('Normalize failed: bad input')).not.toBeInTheDocument()
      })
      expect(screen.getByPlaceholderText(/paste your spec content here/i)).toHaveValue('My spec body')
    })
  })
})
