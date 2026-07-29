import { describe, it, expect } from 'vitest'
import { indexSubLanesByParent, groupSubLanes, subLaneGroupsFor } from './subLanes'
import type { TraceLaneData } from './types'

function makeSubLane(overrides: Partial<TraceLaneData> = {}): TraceLaneData {
  return {
    lane_id: 'w1',
    phase: 'delegate:w1',
    layer: -1,
    agent_type: 'extractor',
    status: 'completed',
    ...overrides,
  }
}

describe('indexSubLanesByParent', () => {
  it('indexes sub-lanes by parent_lane_id and drops entries with no parent', () => {
    const lanes = [
      makeSubLane({ lane_id: 'w1', parent_lane_id: 'p1' }),
      makeSubLane({ lane_id: 'w2', parent_lane_id: 'p1' }),
      makeSubLane({ lane_id: 'w3', parent_lane_id: 'p2' }),
      makeSubLane({ lane_id: 'orphan' }), // no parent_lane_id
    ]
    const byParent = indexSubLanesByParent(lanes)
    expect(byParent.get('p1')).toHaveLength(2)
    expect(byParent.get('p2')).toHaveLength(1)
    expect([...byParent.values()].flat().find((l) => l.lane_id === 'orphan')).toBeUndefined()
  })
})

describe('groupSubLanes', () => {
  it('buckets a delegation fanout into one group', () => {
    const lanes = [
      makeSubLane({ lane_id: 'w1', kind: 'delegate', delegation_id: 'd1', parent_lane_id: 'p1' }),
      makeSubLane({ lane_id: 'w2', kind: 'delegate', delegation_id: 'd1', parent_lane_id: 'p1' }),
      makeSubLane({ lane_id: 'w3', kind: 'delegate', delegation_id: 'd1', parent_lane_id: 'p1' }),
    ]
    const groups = groupSubLanes(lanes)
    expect(groups).toHaveLength(1)
    expect(groups[0].kind).toBe('delegate')
    expect(groups[0].lanes).toHaveLength(3)
    expect(groups[0].label).toBe('⤷ delegate ×3')
  })

  it('buckets a consult child into its own group', () => {
    const lanes = [
      makeSubLane({ lane_id: 'c1', kind: 'consult', consult_id: 'cons1', agent_type: 'reviewer', parent_lane_id: 'p1' }),
    ]
    const groups = groupSubLanes(lanes)
    expect(groups).toHaveLength(1)
    expect(groups[0].kind).toBe('consult')
    expect(groups[0].label).toBe('⤷ consult · reviewer')
  })

  it('falls back to a per-lane_id bucket when delegation_id and consult_id are both absent', () => {
    const lanes = [
      makeSubLane({ lane_id: 'w1', kind: 'delegate' }),
      makeSubLane({ lane_id: 'w2', kind: 'delegate' }),
    ]
    const groups = groupSubLanes(lanes)
    expect(groups).toHaveLength(2)
  })
})

describe('subLaneGroupsFor', () => {
  it('drops an orphan sub-lane whose parent_lane_id does not resolve to a rendered lane', () => {
    const lanes = [makeSubLane({ lane_id: 'w1', parent_lane_id: 'ghost', delegation_id: 'd1' })]
    const byParent = indexSubLanesByParent(lanes)
    const renderedLaneIds = new Set(['p1'])
    expect(subLaneGroupsFor('ghost', byParent, renderedLaneIds)).toEqual([])
  })

  it('returns groups for a resolvable parent', () => {
    const lanes = [
      makeSubLane({ lane_id: 'w1', parent_lane_id: 'p1', delegation_id: 'd1' }),
      makeSubLane({ lane_id: 'w2', parent_lane_id: 'p1', delegation_id: 'd1' }),
    ]
    const byParent = indexSubLanesByParent(lanes)
    const renderedLaneIds = new Set(['p1'])
    const groups = subLaneGroupsFor('p1', byParent, renderedLaneIds)
    expect(groups).toHaveLength(1)
    expect(groups[0].lanes).toHaveLength(2)
  })

  it('returns empty for a parent with no sub-lanes', () => {
    const byParent = indexSubLanesByParent([])
    expect(subLaneGroupsFor('p1', byParent, new Set(['p1']))).toEqual([])
  })
})
