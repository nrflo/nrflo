import { describe, it, expect } from 'vitest'
import { groupSystemAgentRuns } from './systemAgentRunGroups'
import type { SystemAgentRun } from '@/types/systemAgentRuns'

function makeRun(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'agent_session',
    session_id: 's1',
    agent_type: 'executor',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('groupSystemAgentRuns', () => {
  it('collapses two workers sharing a delegation_id into one group', () => {
    const entries = groupSystemAgentRuns([
      makeRun({ session_id: 'w1', delegation_id: 'd1', fanout: 2 }),
      makeRun({ session_id: 'w2', delegation_id: 'd1', fanout: 2 }),
    ])

    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'delegation_group', delegation_id: 'd1', fanout: 2 })
    if (entries[0].kind === 'delegation_group') {
      expect(entries[0].workers.map((w) => w.session_id)).toEqual(['w1', 'w2'])
    }
  })

  it('passes a run without delegation_id through unchanged', () => {
    const run = makeRun({ session_id: 'solo' })
    const entries = groupSystemAgentRuns([run])
    expect(entries).toEqual([{ kind: 'run', run }])
  })

  it('sums input/output tokens and cost across workers, treating null cost_estimate as zero', () => {
    const entries = groupSystemAgentRuns([
      makeRun({
        session_id: 'w1',
        delegation_id: 'd1',
        tokens_json: { input_tokens: 10, output_tokens: 20 },
        cost_estimate: 0.5,
      }),
      makeRun({
        session_id: 'w2',
        delegation_id: 'd1',
        tokens_json: { input_tokens: 5, output_tokens: 7 },
        cost_estimate: null,
      }),
    ])

    expect(entries[0]).toMatchObject({
      kind: 'delegation_group',
      input_tokens: 15,
      output_tokens: 27,
      cost_estimate: 0.5,
    })
  })

  it('reports cost_estimate as null when every worker has a null/undefined cost', () => {
    const entries = groupSystemAgentRuns([
      makeRun({ session_id: 'w1', delegation_id: 'd1', cost_estimate: null }),
      makeRun({ session_id: 'w2', delegation_id: 'd1' }),
    ])
    expect(entries[0]).toMatchObject({ kind: 'delegation_group', cost_estimate: null })
  })

  it('preserves newest-first ordering, anchoring the group at its newest worker relative to ungrouped rows', () => {
    const ungroupedNewest = makeRun({ session_id: 'u1', created_at: '2026-01-03T00:00:00Z' })
    const worker1 = makeRun({ session_id: 'w1', delegation_id: 'd1', created_at: '2026-01-02T00:00:00Z' })
    const ungroupedOldest = makeRun({ session_id: 'u2', created_at: '2026-01-01T30:00:00Z' })
    const worker2 = makeRun({ session_id: 'w2', delegation_id: 'd1', created_at: '2026-01-01T00:00:00Z' })

    const entries = groupSystemAgentRuns([ungroupedNewest, worker1, ungroupedOldest, worker2])

    expect(entries).toHaveLength(3)
    expect(entries[0]).toEqual({ kind: 'run', run: ungroupedNewest })
    expect(entries[1]).toMatchObject({ kind: 'delegation_group', delegation_id: 'd1' })
    expect(entries[2]).toEqual({ kind: 'run', run: ungroupedOldest })
  })

  it('reports workers-shown count below fanout as a partial group, without inventing missing workers', () => {
    const entries = groupSystemAgentRuns([makeRun({ session_id: 'w1', delegation_id: 'd1', fanout: 5 })])
    expect(entries[0]).toMatchObject({ kind: 'delegation_group', fanout: 5 })
    if (entries[0].kind === 'delegation_group') {
      expect(entries[0].workers).toHaveLength(1)
    }
  })

  it('groups a single worker even at fanout 1 (no singleton special-case)', () => {
    const entries = groupSystemAgentRuns([makeRun({ session_id: 'w1', delegation_id: 'd1', fanout: 1 })])
    expect(entries[0].kind).toBe('delegation_group')
  })
})
