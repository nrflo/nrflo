import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProjectFindings, upsertProjectFinding, deleteProjectFinding } from './projectWorkflows'
import * as client from './client'

vi.mock('./client', () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiDelete: vi.fn(),
}))

describe('getProjectFindings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls GET /api/v1/projects/:id/findings', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({})
    await getProjectFindings('my-project')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/projects/my-project/findings')
  })

  it('URL-encodes project ID', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({})
    await getProjectFindings('project/with/slashes')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/projects/project%2Fwith%2Fslashes/findings')
  })

  it('returns findings map from server', async () => {
    const findings = { deploy_url: 'https://example.com', version: '1.2.3' }
    vi.mocked(client.apiGet).mockResolvedValue(findings)
    const result = await getProjectFindings('my-project')
    expect(result).toEqual(findings)
  })

  it('returns empty object when no findings', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({})
    const result = await getProjectFindings('my-project')
    expect(result).toEqual({})
  })
})

describe('upsertProjectFinding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls POST /api/v1/projects/:id/findings with key and value', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ foo: 'bar' })
    await upsertProjectFinding('my-project', 'foo', 'bar')
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/my-project/findings',
      { key: 'foo', value: 'bar' }
    )
  })

  it('URL-encodes project ID with slashes', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({})
    await upsertProjectFinding('project/with/slashes', 'key', 'value')
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/project%2Fwith%2Fslashes/findings',
      { key: 'key', value: 'value' }
    )
  })

  it('passes object values through unchanged', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({})
    const value = { nested: true, count: 3 }
    await upsertProjectFinding('my-project', 'config', value)
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/projects/my-project/findings',
      { key: 'config', value }
    )
  })
})

describe('deleteProjectFinding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls DELETE /api/v1/projects/:id/findings/:key', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ message: 'deleted' })
    await deleteProjectFinding('my-project', 'foo')
    expect(client.apiDelete).toHaveBeenCalledWith(
      '/api/v1/projects/my-project/findings/foo'
    )
  })

  it('URL-encodes project ID with slashes', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ message: 'deleted' })
    await deleteProjectFinding('project/with/slashes', 'foo')
    expect(client.apiDelete).toHaveBeenCalledWith(
      '/api/v1/projects/project%2Fwith%2Fslashes/findings/foo'
    )
  })

  it('URL-encodes key with special characters', async () => {
    vi.mocked(client.apiDelete).mockResolvedValue({ message: 'deleted' })
    await deleteProjectFinding('my-project', 'key/with spaces&special')
    expect(client.apiDelete).toHaveBeenCalledWith(
      '/api/v1/projects/my-project/findings/key%2Fwith%20spaces%26special'
    )
  })
})
