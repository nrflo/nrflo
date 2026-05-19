import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AuthFailedBanner } from './AuthFailedBanner'
import type { Connection } from '@/stores/connectionsStore'

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
const REMOTE: Connection = { id: 'r1', name: 'Production', baseURL: 'https://prod.example.com', isLocal: false }

let mockList: Connection[] = [LOCAL]

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn((selector: (s: { list: Connection[] }) => unknown) =>
    selector({ list: mockList })
  ),
}))

function renderBanner() {
  return render(
    <MemoryRouter>
      <AuthFailedBanner />
    </MemoryRouter>
  )
}

describe('AuthFailedBanner', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL]
  })

  it('renders nothing when no connection has authFailed', () => {
    const { container } = renderBanner()
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when remote connection has authFailed=false', () => {
    mockList = [LOCAL, { ...REMOTE, authFailed: false }]
    const { container } = renderBanner()
    expect(container.firstChild).toBeNull()
  })

  it('does not show banner for local connection even if authFailed', () => {
    mockList = [{ ...LOCAL, authFailed: true }]
    const { container } = renderBanner()
    expect(container.firstChild).toBeNull()
  })

  it('shows banner with connection name when remote authFailed is true', () => {
    mockList = [LOCAL, { ...REMOTE, authFailed: true }]
    renderBanner()
    expect(screen.getByText('Production')).toBeInTheDocument()
    expect(screen.getByText(/service token/i)).toBeInTheDocument()
  })

  it('shows Open Connections button when authFailed', () => {
    mockList = [LOCAL, { ...REMOTE, authFailed: true }]
    renderBanner()
    expect(screen.getByRole('button', { name: /open connections/i })).toBeInTheDocument()
  })

  it('shows banner for first failed remote when multiple remotes exist', () => {
    mockList = [
      LOCAL,
      { ...REMOTE, id: 'r1', name: 'Staging', authFailed: false },
      { ...REMOTE, id: 'r2', name: 'Production', authFailed: true },
    ]
    renderBanner()
    expect(screen.getByText('Production')).toBeInTheDocument()
  })
})
