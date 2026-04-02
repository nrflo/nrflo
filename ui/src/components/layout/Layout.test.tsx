import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { Layout } from './Layout'

vi.mock('./Header', () => ({
  Header: () => <div data-testid="header">Header</div>,
}))

vi.mock('./Sidebar', () => ({
  Sidebar: () => <div data-testid="sidebar">Sidebar</div>,
}))

const mockUseProjectStore = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector?: (s: unknown) => unknown) => mockUseProjectStore(selector),
}))

type StoreShape = { projectsLoaded: boolean; projects: { id: string }[] }

function setStore(overrides: Partial<StoreShape> = {}) {
  const store: StoreShape = { projectsLoaded: true, projects: [{ id: 'test' }], ...overrides }
  mockUseProjectStore.mockImplementation((selector?: (s: StoreShape) => unknown) =>
    selector ? selector(store) : store
  )
}

function renderLayout(initialPath = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<div>Home</div>} />
            <Route path="settings" element={<div data-testid="settings-page">Settings</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Layout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setStore()
  })

  it('renders header, sidebar, and outlet', () => {
    const { getByTestId } = renderLayout()
    expect(getByTestId('header')).toBeInTheDocument()
    expect(getByTestId('sidebar')).toBeInTheDocument()
  })

  describe('no-projects redirect', () => {
    it('redirects to /settings when projectsLoaded and no projects', async () => {
      setStore({ projectsLoaded: true, projects: [] })
      renderLayout('/')
      await screen.findByTestId('settings-page')
      expect(screen.queryByText('Home')).not.toBeInTheDocument()
    })

    it('does not redirect when projects exist', () => {
      setStore({ projectsLoaded: true, projects: [{ id: 'p1' }] })
      renderLayout('/')
      expect(screen.getByText('Home')).toBeInTheDocument()
      expect(screen.queryByTestId('settings-page')).not.toBeInTheDocument()
    })

    it('does not redirect while projectsLoaded is false', () => {
      setStore({ projectsLoaded: false, projects: [] })
      renderLayout('/')
      expect(screen.getByText('Home')).toBeInTheDocument()
      expect(screen.queryByTestId('settings-page')).not.toBeInTheDocument()
    })

    it('does not redirect when already on /settings', () => {
      setStore({ projectsLoaded: true, projects: [] })
      renderLayout('/settings')
      expect(screen.getByTestId('settings-page')).toBeInTheDocument()
    })
  })
})
