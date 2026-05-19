import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConnectionsSection } from './ConnectionsSection'
import * as clientApi from '@/api/client'
import type { Connection } from '@/stores/connectionsStore'

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
const REMOTE: Connection = {
  id: 'r1',
  name: 'Production',
  baseURL: 'https://prod.example.com',
  isLocal: false,
  token: 'nrf_tok',
}

let mockList: Connection[] = [LOCAL]
let mockActiveId = 'local'
const mockAdd = vi.fn()
const mockRemove = vi.fn()
const mockSetActive = vi.fn()

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn((selector?: (s: unknown) => unknown) => {
    const store = {
      list: mockList,
      activeId: mockActiveId,
      add: mockAdd,
      remove: mockRemove,
      setActive: mockSetActive,
    }
    return selector ? selector(store) : store
  }),
}))

vi.mock('@/api/client', () => ({
  testConnection: vi.fn(),
}))

function renderSection() {
  return render(<ConnectionsSection />)
}

describe('ConnectionsSection - table render', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL]
    mockActiveId = 'local'
  })

  it('renders section heading and description', () => {
    renderSection()
    expect(screen.getByRole('heading', { name: /connections/i })).toBeInTheDocument()
    expect(screen.getByText(/manage connections/i)).toBeInTheDocument()
  })

  it('renders table column headers', () => {
    renderSection()
    expect(screen.getByText('Name')).toBeInTheDocument()
    expect(screen.getByText('Base URL')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
  })

  it('renders Add connection button', () => {
    renderSection()
    expect(screen.getByRole('button', { name: /add connection/i })).toBeInTheDocument()
  })

  it('renders local connection row', () => {
    renderSection()
    expect(screen.getByText('Local')).toBeInTheDocument()
  })

  it('renders all connections from the store', () => {
    mockList = [LOCAL, REMOTE]
    renderSection()
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('Production')).toBeInTheDocument()
    expect(screen.getByText('https://prod.example.com')).toBeInTheDocument()
  })

  it('local connection Remove button is disabled', () => {
    renderSection()
    const removeBtn = screen.getByRole('button', { name: /remove/i })
    expect(removeBtn).toBeDisabled()
  })

  it('remote connection Remove button is enabled', () => {
    mockList = [LOCAL, REMOTE]
    renderSection()
    const removeButtons = screen.getAllByRole('button', { name: /remove/i })
    const remoteRemove = removeButtons[1]
    expect(remoteRemove).not.toBeDisabled()
  })

  it('shows Active status for local connection', () => {
    renderSection()
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('shows authFailed status for remote with authFailed=true', () => {
    mockList = [LOCAL, { ...REMOTE, authFailed: true }]
    renderSection()
    expect(screen.getByText('Authentication failed')).toBeInTheDocument()
  })
})

describe('ConnectionsSection - Test button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL, REMOTE]
    mockActiveId = 'local'
  })

  it('Test button calls testConnection with the connection', async () => {
    vi.mocked(clientApi.testConnection).mockResolvedValue({ ok: true, status: 200 })
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /^test$/i }))

    expect(clientApi.testConnection).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'r1', baseURL: 'https://prod.example.com' })
    )
  })

  it('shows OK result after successful test', async () => {
    vi.mocked(clientApi.testConnection).mockResolvedValue({ ok: true, status: 200 })
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /^test$/i }))
    await screen.findByText('OK')
  })

  it('shows failure message after failed test', async () => {
    vi.mocked(clientApi.testConnection).mockResolvedValue({
      ok: false,
      status: 401,
      message: 'Unauthorized',
    })
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /^test$/i }))
    await screen.findByText('Unauthorized')
  })

  it('shows Testing… while in-flight', async () => {
    let resolve!: (v: { ok: boolean; status: number }) => void
    vi.mocked(clientApi.testConnection).mockReturnValue(
      new Promise((r) => { resolve = r })
    )
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /^test$/i }))
    expect(screen.getByText('Testing…')).toBeInTheDocument()

    resolve({ ok: true, status: 200 })
    await screen.findByText('OK')
  })
})

describe('ConnectionsSection - remove flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL, REMOTE]
    mockActiveId = 'local'
  })

  it('clicking Remove opens confirmation dialog', async () => {
    const user = userEvent.setup()
    renderSection()

    const removeButtons = screen.getAllByRole('button', { name: /remove/i })
    await user.click(removeButtons[1])

    // Dialog title is unique in the portal
    expect(screen.getByText('Remove connection')).toBeInTheDocument()
    expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument()
  })

  it('confirming removal calls remove with connection id', async () => {
    const user = userEvent.setup()
    renderSection()

    const removeButtons = screen.getAllByRole('button', { name: /remove/i })
    await user.click(removeButtons[1])

    // Dialog portals to end of body — its confirm button is last in DOM order
    const allRemove = screen.getAllByRole('button', { name: /remove/i })
    await user.click(allRemove[allRemove.length - 1])

    await waitFor(() => {
      expect(mockRemove).toHaveBeenCalledWith('r1')
    })
  })

  it('cancelling remove dialog does not call remove', async () => {
    const user = userEvent.setup()
    renderSection()

    const removeButtons = screen.getAllByRole('button', { name: /remove/i })
    await user.click(removeButtons[1])

    // Cancel only appears in the dialog
    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockRemove).not.toHaveBeenCalled()
  })

  it('removing active remote calls setActive("local") before remove', async () => {
    mockActiveId = 'r1'
    const user = userEvent.setup()
    renderSection()

    const removeButtons = screen.getAllByRole('button', { name: /remove/i })
    await user.click(removeButtons[1])

    const allRemove = screen.getAllByRole('button', { name: /remove/i })
    await user.click(allRemove[allRemove.length - 1])

    await waitFor(() => {
      expect(mockSetActive).toHaveBeenCalledWith('local')
      expect(mockRemove).toHaveBeenCalledWith('r1')
    })
  })
})

describe('ConnectionsSection - Add dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL]
    mockActiveId = 'local'
  })

  it('Add connection button opens the dialog', async () => {
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /add connection/i }))

    expect(screen.getByText('Add Connection')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/my nrflo server/i)).toBeInTheDocument()
  })

  it('dialog is closed by default', () => {
    renderSection()
    expect(screen.queryByText('Add Connection')).not.toBeInTheDocument()
  })

  it('closing the dialog hides it', async () => {
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /add connection/i }))
    expect(screen.getByText('Add Connection')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByText('Add Connection')).not.toBeInTheDocument()
  })

  it('submitting a valid connection calls add', async () => {
    vi.mocked(clientApi.testConnection).mockResolvedValue({ ok: true, status: 200 })
    const user = userEvent.setup()
    renderSection()

    await user.click(screen.getByRole('button', { name: /add connection/i }))

    await user.type(screen.getByPlaceholderText(/my nrflo server/i), 'Staging')
    await user.type(screen.getByPlaceholderText(/https:\/\/nrflo/i), 'https://staging.example.com')
    await user.type(screen.getByPlaceholderText(/nrf_/i), 'nrf_abc123')

    const saveBtn = screen.getByRole('button', { name: /^save$/i })
    await user.click(saveBtn)

    expect(mockAdd).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Staging',
        baseURL: 'https://staging.example.com',
        token: 'nrf_abc123',
        isLocal: false,
      })
    )
  })
})
