import { describe, it, expect } from 'vitest'
import { buildDomain, toPct, niceTicks, bucketMarkers, laneRows, parseTs, splitSpans } from './timeScale'
import type { TraceMarker, WorkflowTraceResponse, TraceLaneData } from './types'

const T0 = '2025-01-01T00:00:00Z'
const T1 = '2025-01-01T00:01:00Z'
const T2 = '2025-01-01T00:02:00Z'

function makeTrace(overrides: Partial<WorkflowTraceResponse> = {}): WorkflowTraceResponse {
  return {
    instance_id: 'wfi-1',
    project_id: 'p',
    workflow: 'wf',
    status: 'active',
    started_at: T0,
    lanes: [
      {
        lane_id: 's1',
        phase: 'a',
        layer: 0,
        agent_type: 'a',
        status: 'running',
        segments: [{ session_id: 's1', status: 'running', started_at: T1, ended_at: null }],
        markers: [],
      },
    ],
    ...overrides,
  }
}

describe('buildDomain', () => {
  it('uses instance start and clamps a live run to receivedAt', () => {
    const receivedAt = parseTs(T2)!
    const d = buildDomain(makeTrace(), receivedAt)!
    expect(d.min).toBe(parseTs(T0))
    expect(d.max).toBe(receivedAt)
  })

  it('live run never shrinks below the latest observed timestamp', () => {
    const staleReceivedAt = parseTs(T0)!
    const d = buildDomain(makeTrace(), staleReceivedAt)!
    expect(d.max).toBe(parseTs(T1)) // latest segment start wins over stale now
  })

  it('terminal run ends at ended_at', () => {
    const d = buildDomain(makeTrace({ ended_at: T2 }), Date.parse('2030-01-01'))!
    expect(d.max).toBe(parseTs(T2))
  })

  it('degenerate domain is padded, unparsable start yields null', () => {
    const d = buildDomain(makeTrace({ lanes: [] }), parseTs(T0)!)!
    expect(d.max - d.min).toBe(1000)
    expect(buildDomain(makeTrace({ started_at: 'bogus', lanes: [] }), 0)).toBeNull()
  })
})

describe('toPct', () => {
  const domain = { min: 0, max: 1000 }
  it('maps linearly and clamps', () => {
    expect(toPct(500, domain)).toBe(50)
    expect(toPct(-100, domain)).toBe(0)
    expect(toPct(2000, domain)).toBe(100)
    expect(toPct(null, domain)).toBeNull()
    expect(toPct(5, { min: 10, max: 10 })).toBeNull()
  })
})

describe('niceTicks', () => {
  it('picks a step yielding roughly the target count', () => {
    const ticks = niceTicks({ min: 0, max: 60_000 }, 7)
    expect(ticks.length).toBeGreaterThan(2)
    expect(ticks.length).toBeLessThanOrEqual(8)
    expect(ticks[0] % 1000).toBe(0)
  })
  it('empty for degenerate domain', () => {
    expect(niceTicks({ min: 5, max: 5 })).toEqual([])
  })
})

describe('bucketMarkers', () => {
  const domain = { min: 0, max: 100_000 }
  const marker = (at: number, type = 'tool'): TraceMarker => ({
    type,
    at: new Date(at).toISOString(),
    label: type,
  })

  it('groups nearby markers into one bucket with dominant type and error flag', () => {
    const buckets = bucketMarkers(
      [marker(1000), marker(1100), marker(1200, 'error'), marker(90_000, 'finding')],
      domain,
      1000,
      8
    )
    expect(buckets).toHaveLength(2)
    expect(buckets[0].markers).toHaveLength(3)
    expect(buckets[0].dominantType).toBe('tool')
    expect(buckets[0].hasError).toBe(true)
    expect(buckets[1].dominantType).toBe('finding')
  })

  it('caps DOM nodes at widthPx/bucketPx regardless of marker count', () => {
    const many = Array.from({ length: 5000 }, (_, i) => marker(i * 20))
    const buckets = bucketMarkers(many, domain, 800, 8)
    expect(buckets.length).toBeLessThanOrEqual(100)
  })

  it('skips unparsable timestamps', () => {
    expect(bucketMarkers([{ type: 'tool', at: 'bogus', label: 'x' }], domain, 1000)).toHaveLength(0)
  })
})

describe('splitSpans', () => {
  const domain = { min: 0, max: 100_000 }
  const span = (at: number, endedAt: number | null, type = 'tool'): TraceMarker => ({
    type,
    at: new Date(at).toISOString(),
    ended_at: endedAt == null ? null : new Date(endedAt).toISOString(),
    label: type,
  })

  it('wide closed spans become bars, open/narrow markers stay points', () => {
    const { spans, points } = splitSpans(
      [span(0, 50_000), span(60_000, 60_100), span(70_000, null)],
      domain,
      1000
    )
    expect(spans).toHaveLength(1)
    expect(spans[0].startPct).toBe(0)
    expect(spans[0].endPct).toBe(50)
    expect(points).toHaveLength(2) // 0.1% wide (1px) + open span
  })

  it('overflow beyond the cap degrades shortest spans to points', () => {
    const many = Array.from({ length: 250 }, (_, i) => span(i * 400, i * 400 + 5000 + i))
    const { spans, points } = splitSpans(many, domain, 1000)
    expect(spans).toHaveLength(200)
    expect(points).toHaveLength(50)
    // Spans stay sorted by start after decimation
    for (let i = 1; i < spans.length; i++) {
      expect(spans[i].startPct).toBeGreaterThanOrEqual(spans[i - 1].startPct)
    }
  })

  it('domain max accounts for marker ended_at (buildDomain)', () => {
    const trace = {
      instance_id: 'i',
      project_id: 'p',
      workflow: 'w',
      status: 'active',
      started_at: T0,
      lanes: [
        {
          lane_id: 's1',
          phase: 'a',
          layer: 0,
          agent_type: 'a',
          status: 'running',
          markers: [{ type: 'tool', at: T1, ended_at: T2, label: 'x' }],
        },
      ],
    }
    const d = buildDomain(trace, 0)!
    expect(d.max).toBe(parseTs(T2))
  })
})

describe('laneRows', () => {
  it('groups consecutive lanes by layer', () => {
    const lane = (id: string, layer: number): TraceLaneData => ({
      lane_id: id,
      phase: id,
      layer,
      agent_type: id,
      status: 'completed',
    })
    const groups = laneRows([lane('a', 0), lane('b', 1), lane('c', 1), lane('d', -1)])
    expect(groups.map((g) => g.layer)).toEqual([0, 1, -1])
    expect(groups[1].lanes).toHaveLength(2)
  })
})
