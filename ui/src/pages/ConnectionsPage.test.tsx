import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ConnectionsPage } from './ConnectionsPage'
import type { Connection } from '@/stores/connectionsStore'

const mockTestConnection = vi.fn()
vi.mock('@/api/client', () => ({
  testConnection: (...args: unknown[]) => mockTestConnection(...args),
}))

const LOCAL: Connection = { id: 'local', name: 'Local', baseURL: '', isLocal: true }
const REMOTE: Connection = {
  id: 'r1',
  name: 'Production',
  baseURL: 'https://prod.example.com',
  isLocal: false,
  token: 'tok-abc',
}

let mockList: Connection[] = [LOCAL]
let mockActiveId = 'local'
const mockAdd = vi.fn()
const mockRemove = vi.fn()
const mockSetActive = vi.fn()

vi.mock('@/stores/connectionsStore', () => ({
  useConnectionsStore: vi.fn((selector?: (s: Record<string, unknown>) => unknown) => {
    const state = {
      list: mockList,
      activeId: mockActiveId,
      add: mockAdd,
      remove: mockRemove,
      setActive: mockSetActive,
    }
    return selector ? selector(state) : state
  }),
}))

function renderPage() {
  render(
    <MemoryRouter>
      <ConnectionsPage />
    </MemoryRouter>
  )
}

describe('ConnectionsPage - table', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTestConnection.mockResolvedValue({ ok: true, status: 200 })
    mockList = [LOCAL, REMOTE]
    mockActiveId = 'local'
  })

  it('renders Local and remote rows', () => {
    renderPage()
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('Production')).toBeInTheDocument()
  })

  it('Local row Remove button is disabled', () => {
    renderPage()
    const localCell = screen.getByText('Local').closest('tr')!
    const removeBtn = within(localCell).getByRole('button', { name: /remove/i })
    expect(removeBtn).toBeDisabled()
  })

  it('shows base URL for remote row', () => {
    renderPage()
    expect(screen.getByText('https://prod.example.com')).toBeInTheDocument()
  })
})

describe('ConnectionsPage - remote row Test button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL, REMOTE]
    mockActiveId = 'local'
  })

  it('calls testConnection with the connection object', async () => {
    mockTestConnection.mockResolvedValue({ ok: true, status: 200 })
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /^test$/i }))
    expect(mockTestConnection).toHaveBeenCalledWith(expect.objectContaining({ id: 'r1' }))
  })

  it('shows OK after successful test', async () => {
    mockTestConnection.mockResolvedValue({ ok: true, status: 200 })
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /^test$/i }))
    expect(await screen.findByText('OK')).toBeInTheDocument()
  })

  it('shows error message on failed test', async () => {
    mockTestConnection.mockResolvedValue({ ok: false, status: 401, message: 'Unauthorized' })
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /^test$/i }))
    expect(await screen.findByText('Unauthorized')).toBeInTheDocument()
  })
})

describe('ConnectionsPage - remove flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList = [LOCAL, REMOTE]
    mockActiveId = 'local'
  })

  it('clicking remote Remove opens confirm dialog', async () => {
    const user = userEvent.setup()
    renderPage()
    const remoteRow = screen.getByText('Production').closest('tr')!
    await user.click(within(remoteRow).getByRole('button', { name: /remove/i }))
    expect(await screen.findByText(/remove "production"/i)).toBeInTheDocument()
  })

  it('confirming remove calls store.remove with the connection id', async () => {
    const user = userEvent.setup()
    renderPage()
    const remoteRow = screen.getByText('Production').closest('tr')!
    await user.click(within(remoteRow).getByRole('button', { name: /remove/i }))
    await screen.findByText(/remove "production"/i)
    // Dialog footer has Cancel + Remove; Cancel is unique in the footer
    const cancelBtn = screen.getByRole('button', { name: /cancel/i })
    const dialogFooter = cancelBtn.parentElement!
    await user.click(within(dialogFooter).getByRole('button', { name: /^remove$/i }))
    expect(mockRemove).toHaveBeenCalledWith('r1')
  })

  it('cancelling remove does not call store.remove', async () => {
    const user = userEvent.setup()
    renderPage()
    const remoteRow = screen.getByText('Production').closest('tr')!
    await user.click(within(remoteRow).getByRole('button', { name: /remove/i }))
    await screen.findByText(/remove "production"/i)
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(mockRemove).not.toHaveBeenCalled()
  })
})

describe('ConnectionsPage - Add dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTestConnection.mockResolvedValue({ ok: true, status: 200 })
    mockList = [LOCAL]
    mockActiveId = 'local'
  })

  it('clicking Add connection shows the dialog', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /add connection/i }))
    expect(await screen.findByText('Add Connection')).toBeInTheDocument()
  })

  it('Save is disabled until all fields are valid', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /add connection/i }))
    await screen.findByText('Add Connection')
    expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled()
  })

  it('shows URL validation error for non-http URL', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /add connection/i }))
    await screen.findByText('Add Connection')
    await user.type(screen.getByPlaceholderText(/https:\/\/nrflo/i), 'not-a-url')
    expect(await screen.findByText(/absolute http/i)).toBeInTheDocument()
  })

  it('clicking Save calls store.add with correct fields', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /add connection/i }))
    await screen.findByText('Add Connection')
    await user.type(screen.getByPlaceholderText(/my nrflo server/i), 'Staging')
    await user.type(screen.getByPlaceholderText(/https:\/\/nrflo/i), 'https://staging.example.com')
    await user.type(screen.getByPlaceholderText(/nrf_/i), 'tok-staging')
    await user.click(screen.getByRole('button', { name: /^save$/i }))
    expect(mockAdd).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Staging',
      baseURL: 'https://staging.example.com',
      token: 'tok-staging',
      isLocal: false,
    }))
  })

  it('Test connection button calls testConnection', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /add connection/i }))
    await screen.findByText('Add Connection')
    await user.type(screen.getByPlaceholderText(/https:\/\/nrflo/i), 'https://staging.example.com')
    await user.type(screen.getByPlaceholderText(/nrf_/i), 'tok-staging')
    await user.click(screen.getByRole('button', { name: /test connection/i }))
    expect(mockTestConnection).toHaveBeenCalledWith(expect.objectContaining({
      baseURL: 'https://staging.example.com',
      token: 'tok-staging',
    }))
    expect(await screen.findByText(/connection successful/i)).toBeInTheDocument()
  })
})
