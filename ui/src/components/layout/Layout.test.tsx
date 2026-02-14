import { describe, it, expect, vi } from 'vitest'
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

describe('Layout', () => {
  it('renders header, sidebar, and outlet', () => {
    const { getByTestId } = renderLayout()

    expect(getByTestId('header')).toBeInTheDocument()
    expect(getByTestId('sidebar')).toBeInTheDocument()
  })
})
