import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getPlan, listPlanRevisions, revisePlan, approvePlan, cancelPlan } from './plan'
import * as client from './client'

vi.mock('./client')

describe('plan api', () => {
  beforeEach(() => vi.clearAllMocks())

  it('getPlan hits GET /api/v1/workflow-instances/{iid}/plan', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ head: null, templates: [] })
    await getPlan('inst-1')
    expect(client.apiGet).toHaveBeenCalledWith('/api/v1/workflow-instances/inst-1/plan')
  })

  it('getPlan encodeURIComponent-encodes the instance id', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ head: null, templates: [] })
    await getPlan('inst/with space')
    expect(client.apiGet).toHaveBeenCalledWith(
      '/api/v1/workflow-instances/inst%2Fwith%20space/plan'
    )
  })

  it('listPlanRevisions hits GET .../plan/revisions', async () => {
    vi.mocked(client.apiGet).mockResolvedValue({ revisions: [] })
    await listPlanRevisions('inst-1')
    expect(client.apiGet).toHaveBeenCalledWith(
      '/api/v1/workflow-instances/inst-1/plan/revisions'
    )
  })

  it('revisePlan posts to .../plan/revise with the request body', async () => {
    const revision = { instance_id: 'inst-1', revision: 2, manifest: '{}', hash: 'h', author: 'caller' as const, created_at: '2026-01-01T00:00:00Z' }
    vi.mocked(client.apiPost).mockResolvedValue(revision)
    const req = { revision: 1, feedback: 'add a step' }
    await revisePlan('inst-1', req)
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/workflow-instances/inst-1/plan/revise',
      req
    )
  })

  it('approvePlan posts to .../plan/approve with the pinned revision', async () => {
    const revision = { instance_id: 'inst-1', revision: 3, manifest: '{}', hash: 'h', author: 'caller' as const, created_at: '2026-01-01T00:00:00Z' }
    vi.mocked(client.apiPost).mockResolvedValue(revision)
    await approvePlan('inst-1', { revision: 3 })
    expect(client.apiPost).toHaveBeenCalledWith(
      '/api/v1/workflow-instances/inst-1/plan/approve',
      { revision: 3 }
    )
  })

  it('cancelPlan posts to .../plan/cancel with no body', async () => {
    vi.mocked(client.apiPost).mockResolvedValue({ status: 'cancelled' })
    await cancelPlan('inst-1')
    expect(client.apiPost).toHaveBeenCalledWith('/api/v1/workflow-instances/inst-1/plan/cancel')
  })

  it('returns the API response for getPlan', async () => {
    const draft = { head: null, templates: [] }
    vi.mocked(client.apiGet).mockResolvedValue(draft)
    const result = await getPlan('inst-1')
    expect(result).toEqual(draft)
  })
})
