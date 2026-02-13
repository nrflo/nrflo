import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  listChains,
  getChain,
  createChain,
  updateChain,
  startChain,
  cancelChain,
} from './chains'
import * as client from './client'
import type { ChainExecution, ChainCreateRequest, ChainUpdateRequest } from '@/types/chain'

// Mock the client module
vi.mock('./client')

function createMockChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-123',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'pending',
    workflow_name: 'feature',
    category: 'full',
    created_by: 'test-user',
    total_items: 0,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [],
    ...overrides,
  }
}

describe('listChains', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiGet with correct endpoint without params', async () => {
    const chains = [createMockChain()]
    vi.mocked(client.apiGet).mockResolvedValue(chains)

    const result = await listChains()

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/chains')
    expect(result).toEqual(chains)
  })

  it('calls apiGet with status filter param', async () => {
    const chains = [createMockChain({ status: 'running' })]
    vi.mocked(client.apiGet).mockResolvedValue(chains)

    const result = await listChains({ status: 'running' })

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/chains?status=running')
    expect(result).toEqual(chains)
  })

  it('handles multiple chains in response', async () => {
    const chains = [
      createMockChain({ id: 'chain-1', name: 'Chain 1' }),
      createMockChain({ id: 'chain-2', name: 'Chain 2' }),
      createMockChain({ id: 'chain-3', name: 'Chain 3' }),
    ]
    vi.mocked(client.apiGet).mockResolvedValue(chains)

    const result = await listChains()

    expect(result).toHaveLength(3)
    expect(result).toEqual(chains)
  })

  it('handles empty chains list', async () => {
    vi.mocked(client.apiGet).mockResolvedValue([])

    const result = await listChains()

    expect(result).toEqual([])
  })

  it('propagates errors from apiGet', async () => {
    const error = new Error('Network error')
    vi.mocked(client.apiGet).mockRejectedValue(error)

    await expect(listChains()).rejects.toThrow('Network error')
  })
})

describe('getChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiGet with correct endpoint and chain ID', async () => {
    const chain = createMockChain({ id: 'chain-abc' })
    vi.mocked(client.apiGet).mockResolvedValue(chain)

    const result = await getChain('chain-abc')

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/chains/chain-abc')
    expect(result).toEqual(chain)
  })

  it('encodes special characters in chain ID', async () => {
    const chain = createMockChain({ id: 'chain/123' })
    vi.mocked(client.apiGet).mockResolvedValue(chain)

    await getChain('chain/123')

    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/chains/chain%2F123')
  })

  it('returns chain with items', async () => {
    const chain = createMockChain({
      items: [
        { id: 'item-1', chain_id: 'chain-123', ticket_id: 'TICKET-1', position: 0, status: 'pending' },
        { id: 'item-2', chain_id: 'chain-123', ticket_id: 'TICKET-2', position: 1, status: 'completed' },
      ],
    })
    vi.mocked(client.apiGet).mockResolvedValue(chain)

    const result = await getChain('chain-123')

    expect(result.items).toHaveLength(2)
    expect(result.items![0].ticket_id).toBe('TICKET-1')
  })

  it('propagates errors from apiGet', async () => {
    const error = new Error('Chain not found')
    vi.mocked(client.apiGet).mockRejectedValue(error)

    await expect(getChain('chain-nonexistent')).rejects.toThrow('Chain not found')
  })
})

describe('createChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiPost with correct endpoint and data', async () => {
    const createData: ChainCreateRequest = {
      name: 'New Chain',
      workflow_name: 'feature',
      category: 'full',
      ticket_ids: ['TICKET-1', 'TICKET-2'],
    }
    const createdChain = createMockChain(createData)
    vi.mocked(client.apiPost).mockResolvedValue(createdChain)

    const result = await createChain(createData)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains', createData)
    expect(result).toEqual(createdChain)
  })

  it('creates chain without category', async () => {
    const createData: ChainCreateRequest = {
      name: 'Simple Chain',
      workflow_name: 'hotfix',
      ticket_ids: ['TICKET-1'],
    }
    const createdChain = createMockChain({ ...createData, category: undefined })
    vi.mocked(client.apiPost).mockResolvedValue(createdChain)

    const result = await createChain(createData)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains', createData)
    expect(result.category).toBeUndefined()
  })

  it('creates chain with multiple tickets', async () => {
    const createData: ChainCreateRequest = {
      name: 'Multi-Ticket Chain',
      workflow_name: 'feature',
      ticket_ids: ['TICKET-1', 'TICKET-2', 'TICKET-3', 'TICKET-4'],
    }
    const createdChain = createMockChain()
    vi.mocked(client.apiPost).mockResolvedValue(createdChain)

    await createChain(createData)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains', createData)
  })

  it('propagates errors from apiPost', async () => {
    const error = new Error('Validation failed')
    vi.mocked(client.apiPost).mockRejectedValue(error)

    const createData: ChainCreateRequest = {
      name: 'Test',
      workflow_name: 'feature',
      ticket_ids: [],
    }

    await expect(createChain(createData)).rejects.toThrow('Validation failed')
  })
})

describe('updateChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiPatch with correct endpoint, ID, and data', async () => {
    const updateData: ChainUpdateRequest = {
      name: 'Updated Chain',
      ticket_ids: ['TICKET-1', 'TICKET-2', 'TICKET-3'],
    }
    const updatedChain = createMockChain({ name: 'Updated Chain' })
    vi.mocked(client.apiPatch).mockResolvedValue(updatedChain)

    const result = await updateChain('chain-123', updateData)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/chains/chain-123', updateData)
    expect(result).toEqual(updatedChain)
  })

  it('updates only chain name', async () => {
    const updateData: ChainUpdateRequest = { name: 'New Name' }
    const updatedChain = createMockChain({ name: 'New Name' })
    vi.mocked(client.apiPatch).mockResolvedValue(updatedChain)

    const result = await updateChain('chain-abc', updateData)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/chains/chain-abc', updateData)
    expect(result.name).toBe('New Name')
  })

  it('updates only ticket list', async () => {
    const updateData: ChainUpdateRequest = {
      ticket_ids: ['TICKET-A', 'TICKET-B'],
    }
    const updatedChain = createMockChain()
    vi.mocked(client.apiPatch).mockResolvedValue(updatedChain)

    await updateChain('chain-xyz', updateData)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/chains/chain-xyz', updateData)
  })

  it('encodes special characters in chain ID', async () => {
    const updateData: ChainUpdateRequest = { name: 'Updated' }
    const updatedChain = createMockChain()
    vi.mocked(client.apiPatch).mockResolvedValue(updatedChain)

    await updateChain('chain/special', updateData)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/chains/chain%2Fspecial', updateData)
  })

  it('propagates errors from apiPatch', async () => {
    const error = new Error('Update failed')
    vi.mocked(client.apiPatch).mockRejectedValue(error)

    const updateData: ChainUpdateRequest = { name: 'Test' }

    await expect(updateChain('chain-123', updateData)).rejects.toThrow('Update failed')
  })
})

describe('startChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiPost with correct endpoint and chain ID', async () => {
    const response = { status: 'running', chain_id: 'chain-123' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    const result = await startChain('chain-123')

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains/chain-123/start', {})
    expect(result).toEqual(response)
  })

  it('encodes special characters in chain ID', async () => {
    const response = { status: 'running', chain_id: 'chain/abc' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    await startChain('chain/abc')

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains/chain%2Fabc/start', {})
  })

  it('returns status response', async () => {
    const response = { status: 'running', chain_id: 'chain-xyz' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    const result = await startChain('chain-xyz')

    expect(result.status).toBe('running')
    expect(result.chain_id).toBe('chain-xyz')
  })

  it('propagates errors from apiPost', async () => {
    const error = new Error('Cannot start chain')
    vi.mocked(client.apiPost).mockRejectedValue(error)

    await expect(startChain('chain-123')).rejects.toThrow('Cannot start chain')
  })
})

describe('cancelChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls apiPost with correct endpoint and chain ID', async () => {
    const response = { status: 'canceled', chain_id: 'chain-123' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    const result = await cancelChain('chain-123')

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains/chain-123/cancel', {})
    expect(result).toEqual(response)
  })

  it('encodes special characters in chain ID', async () => {
    const response = { status: 'canceled', chain_id: 'chain/def' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    await cancelChain('chain/def')

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains/chain%2Fdef/cancel', {})
  })

  it('returns status response', async () => {
    const response = { status: 'canceled', chain_id: 'chain-abc' }
    vi.mocked(client.apiPost).mockResolvedValue(response)

    const result = await cancelChain('chain-abc')

    expect(result.status).toBe('canceled')
    expect(result.chain_id).toBe('chain-abc')
  })

  it('propagates errors from apiPost', async () => {
    const error = new Error('Cannot cancel chain')
    vi.mocked(client.apiPost).mockRejectedValue(error)

    await expect(cancelChain('chain-123')).rejects.toThrow('Cannot cancel chain')
  })
})

describe('chains API - Type Safety', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('listChains returns correctly typed ChainExecution array', async () => {
    const chains: ChainExecution[] = [createMockChain()]
    vi.mocked(client.apiGet).mockResolvedValue(chains)

    const result = await listChains()

    expect(result[0].id).toBeDefined()
    expect(result[0].status).toBeDefined()
    expect(result[0].workflow_name).toBeDefined()
  })

  it('getChain returns correctly typed ChainExecution', async () => {
    const chain: ChainExecution = createMockChain()
    vi.mocked(client.apiGet).mockResolvedValue(chain)

    const result = await getChain('chain-123')

    expect(result.id).toBeDefined()
    expect(result.project_id).toBeDefined()
    expect(result.name).toBeDefined()
  })

  it('createChain accepts typed ChainCreateRequest', async () => {
    const createData: ChainCreateRequest = {
      name: 'Typed Chain',
      workflow_name: 'feature',
      ticket_ids: ['TICKET-1'],
    }
    const chain: ChainExecution = createMockChain()
    vi.mocked(client.apiPost).mockResolvedValue(chain)

    await createChain(createData)

    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/chains', createData)
  })

  it('updateChain accepts typed ChainUpdateRequest', async () => {
    const updateData: ChainUpdateRequest = {
      name: 'Updated',
      ticket_ids: ['TICKET-1', 'TICKET-2'],
    }
    const chain: ChainExecution = createMockChain()
    vi.mocked(client.apiPatch).mockResolvedValue(chain)

    await updateChain('chain-123', updateData)

    expect(client.apiPatch).toHaveBeenCalledWith('/api/v1/chains/chain-123', updateData)
  })
})
