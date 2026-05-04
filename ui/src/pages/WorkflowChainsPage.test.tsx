import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { WorkflowChainsPage } from './WorkflowChainsPage'
import type { WorkflowChain } from '@/types/workflowChain'

const mockUseWorkflowChainsList = vi.fn()
const mockCreateMutate = vi.fn()
const mockDeleteMutate = vi.fn()
const mockUseIsAdmin = vi.fn().mockReturnValue(true)

vi.mock('@/hooks/useWorkflowChains', () => ({
  useWorkflowChainsList: () => mockUseWorkflowChainsList(),
  useCreateWorkflowChain: () => ({ mutate: mockCreateMutate, isPending: false, error: null }),
  useDeleteWorkflowChain: () => ({ mutate: mockDeleteMutate, isPending: false }),
}))

vi.mock('@/stores/authStore', () => ({
  useIsAdmin: () => mockUseIsAdmin(),
}))

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) =>
    selector({ currentProject: 'test-project', projectsLoaded: true })
  ),
}))

function makeChain(overrides: Partial<WorkflowChain> = {}): WorkflowChain {
  return {
    id: 'chain-1',
    project_id: 'test-project',
    name: 'My Chain',
    description: 'A test chain',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <WorkflowChainsPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('WorkflowChainsPage - Render States', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('renders loading spinner', () => {
    mockUseWorkflowChainsList.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderPage()
    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument()
  })

  it('renders empty state', () => {
    mockUseWorkflowChainsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderPage()
    expect(screen.getByText(/No workflow chains found/)).toBeInTheDocument()
  })

  it('renders error message', () => {
    mockUseWorkflowChainsList.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Failed to fetch'),
    })
    renderPage()
    expect(screen.getByText('Failed to fetch')).toBeInTheDocument()
  })

  it('renders chain rows and chain count', () => {
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain(), makeChain({ id: 'chain-2', name: 'Second Chain' })],
      isLoading: false,
      error: null,
    })
    renderPage()
    expect(screen.getByText('My Chain')).toBeInTheDocument()
    expect(screen.getByText('Second Chain')).toBeInTheDocument()
    expect(screen.getByText('2 chains')).toBeInTheDocument()
  })

  it('shows singular count for single chain', () => {
    mockUseWorkflowChainsList.mockReturnValue({ data: [makeChain()], isLoading: false, error: null })
    renderPage()
    expect(screen.getByText('1 chain')).toBeInTheDocument()
  })
})

describe('WorkflowChainsPage - Actions', () => {
  beforeEach(() => { vi.clearAllMocks(); mockUseIsAdmin.mockReturnValue(true) })

  it('clicking New Chain opens create dialog', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByRole('button', { name: /New Chain/ }))
    expect(screen.getByText('New Workflow Chain')).toBeInTheDocument()
  })

  it('submitting create dialog calls createChain mutation with name', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({ data: [], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByRole('button', { name: /New Chain/ }))
    await user.type(screen.getByPlaceholderText('My workflow chain'), 'My New Chain')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    expect(mockCreateMutate).toHaveBeenCalledWith(
      { name: 'My New Chain', description: '', steps: [] },
      expect.any(Object)
    )
  })

  it('clicking Delete button opens confirm dialog', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({ data: [makeChain()], isLoading: false, error: null })
    renderPage()

    await user.click(screen.getByTitle('Delete'))
    expect(screen.getByText('Delete Workflow Chain')).toBeInTheDocument()
    expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument()
  })

  it('confirming delete calls deleteMutation with chain id', async () => {
    const user = userEvent.setup()
    mockUseWorkflowChainsList.mockReturnValue({
      data: [makeChain({ id: 'chain-abc', name: 'Doomed Chain' })],
      isLoading: false,
      error: null,
    })
    renderPage()

    await user.click(screen.getByTitle('Delete'))
    await user.click(screen.getByText('Delete', { selector: 'button' }))
    expect(mockDeleteMutate).toHaveBeenCalledWith('chain-abc', expect.any(Object))
  })
})

describe('WorkflowChainsPage - Viewer Role', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAdmin.mockReturnValue(false)
    mockUseWorkflowChainsList.mockReturnValue({ data: [makeChain()], isLoading: false, error: null })
  })

  it('shows ReadOnlyHint banner', () => {
    renderPage()
    expect(screen.getByText('Read-only — admin required to make changes.')).toBeInTheDocument()
  })

  it('hides New Chain button', () => {
    renderPage()
    expect(screen.queryByRole('button', { name: /New Chain/i })).not.toBeInTheDocument()
  })

  it('hides Delete button per row', () => {
    renderPage()
    expect(screen.queryByTitle('Delete')).not.toBeInTheDocument()
  })

  it('still renders chain rows', () => {
    renderPage()
    expect(screen.getByText('My Chain')).toBeInTheDocument()
  })
})
