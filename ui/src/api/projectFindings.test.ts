import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProjectFindings } from './projectWorkflows'
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
