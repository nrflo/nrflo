import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { Layout } from './Layout'

// Mock child components
vi.mock('./Header', () => ({
  Header: () => <div data-testid="header">Header</div>,
}))

vi.mock('./Sidebar', () => ({
  Sidebar: () => <div data-testid="sidebar">Sidebar</div>,
}))

// Mock useProjectStore
const mockProjectStore = vi.fn()
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    mockProjectStore(selector),
}))

// Mock useWebSocket
const mockSubscribe = vi.fn()
const mockUnsubscribe = vi.fn()
vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    isConnected: true,
    subscribe: mockSubscribe,
    unsubscribe: mockUnsubscribe,
  }),
}))

function renderLayout() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <Layout />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Layout - WebSocket Subscription', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: projects loaded
    mockProjectStore.mockImplementation((selector) =>
      selector({ currentProject: 'test-project', projectsLoaded: true })
    )
  })

  it('subscribes to project-wide WebSocket when projectsLoaded is true', () => {
    renderLayout()

    // Should subscribe with empty ticketId for project-wide events
    expect(mockSubscribe).toHaveBeenCalledWith('')
    expect(mockSubscribe).toHaveBeenCalledTimes(1)
  })

  it('does not subscribe when projectsLoaded is false', () => {
    mockProjectStore.mockImplementation((selector) =>
      selector({ currentProject: '', projectsLoaded: false })
    )

    renderLayout()

    expect(mockSubscribe).not.toHaveBeenCalled()
  })

  it('unsubscribes on unmount', () => {
    const { unmount } = renderLayout()

    expect(mockSubscribe).toHaveBeenCalledWith('')

    unmount()

    expect(mockUnsubscribe).toHaveBeenCalledWith('')
    expect(mockUnsubscribe).toHaveBeenCalledTimes(1)
  })

  it('resubscribes when currentProject changes', () => {
    mockProjectStore.mockImplementation((selector) =>
      selector({ currentProject: 'project-1', projectsLoaded: true })
    )

    const { rerender } = renderLayout()

    expect(mockSubscribe).toHaveBeenCalledTimes(1)
    expect(mockSubscribe).toHaveBeenCalledWith('')

    // Simulate project change
    mockProjectStore.mockImplementation((selector) =>
      selector({ currentProject: 'project-2', projectsLoaded: true })
    )

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <Layout />
        </MemoryRouter>
      </QueryClientProvider>
    )

    // Should unsubscribe and resubscribe
    expect(mockUnsubscribe).toHaveBeenCalledTimes(1)
    expect(mockSubscribe).toHaveBeenCalledTimes(2)
  })

  it('renders header, sidebar, and outlet', () => {
    const { getByTestId } = renderLayout()

    expect(getByTestId('header')).toBeInTheDocument()
    expect(getByTestId('sidebar')).toBeInTheDocument()
  })

  it('does not unsubscribe when projectsLoaded is false', () => {
    mockProjectStore.mockImplementation((selector) =>
      selector({ currentProject: '', projectsLoaded: false })
    )

    const { unmount } = renderLayout()

    unmount()

    // No subscription was made, so no unsubscribe
    expect(mockUnsubscribe).not.toHaveBeenCalled()
  })
})
