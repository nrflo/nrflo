// Pure time-scale math for the trace timeline. No timers anywhere: "now" is
// the data-arrival timestamp, so the running edge advances on WS-driven
// refetches only.

import type { TraceLaneData, TraceMarker, WorkflowTraceResponse } from './types'

export interface TimeDomain {
  min: number
  max: number
}

export function parseTs(ts: string | null | undefined): number | null {
  if (!ts) return null
  const ms = Date.parse(ts)
  return Number.isNaN(ms) ? null : ms
}

/** Clamped percentage position of a timestamp within the domain. */
export function toPct(tMs: number | null, domain: TimeDomain): number | null {
  if (tMs == null || domain.max <= domain.min) return null
  const pct = ((tMs - domain.min) / (domain.max - domain.min)) * 100
  return Math.min(100, Math.max(0, pct))
}

/**
 * Build the visible time domain. Min is the instance start (fallback:
 * earliest observed timestamp); max is the instance end, or for a live run
 * the later of data arrival time and the latest observed timestamp.
 */
export function buildDomain(trace: WorkflowTraceResponse, receivedAtMs: number): TimeDomain | null {
  let min = parseTs(trace.started_at)
  let latest = min ?? 0
  for (const lane of trace.lanes ?? []) {
    for (const seg of lane.segments ?? []) {
      const s = parseTs(seg.started_at)
      const e = parseTs(seg.ended_at)
      if (s != null && (min == null || s < min)) min = s
      if (s != null && s > latest) latest = s
      if (e != null && e > latest) latest = e
    }
    for (const m of lane.markers ?? []) {
      const t = parseTs(m.at)
      if (t != null && t > latest) latest = t
    }
  }
  if (min == null) return null
  const ended = parseTs(trace.ended_at)
  let max = ended ?? Math.max(receivedAtMs, latest)
  if (max <= min) max = min + 1000
  return { min, max }
}

const TICK_STEPS_MS = [
  1_000, 5_000, 15_000, 30_000, // seconds
  60_000, 300_000, 900_000, 1_800_000, // minutes
  3_600_000, 10_800_000, 21_600_000, 43_200_000, // hours
  86_400_000, 604_800_000, // days
]

/** Tick timestamps (ms) at a "nice" step targeting ~targetCount ticks. */
export function niceTicks(domain: TimeDomain, targetCount = 7): number[] {
  const span = domain.max - domain.min
  if (span <= 0) return []
  let step = TICK_STEPS_MS[TICK_STEPS_MS.length - 1]
  for (const s of TICK_STEPS_MS) {
    if (span / s <= targetCount) {
      step = s
      break
    }
  }
  const ticks: number[] = []
  for (let t = Math.ceil(domain.min / step) * step; t <= domain.max; t += step) {
    ticks.push(t)
  }
  return ticks
}

export interface MarkerBucket {
  pct: number
  markers: TraceMarker[]
  dominantType: string
  hasError: boolean
}

/**
 * Bucket markers by horizontal pixel so lanes stay cheap regardless of
 * marker count: at most widthPx/bucketPx DOM nodes per lane.
 */
export function bucketMarkers(
  markers: TraceMarker[],
  domain: TimeDomain,
  widthPx: number,
  bucketPx = 8
): MarkerBucket[] {
  const bucketCount = Math.max(1, Math.floor(widthPx / bucketPx))
  const byBucket = new Map<number, TraceMarker[]>()
  for (const m of markers) {
    const pct = toPct(parseTs(m.at), domain)
    if (pct == null) continue
    const idx = Math.min(bucketCount - 1, Math.floor((pct / 100) * bucketCount))
    const list = byBucket.get(idx)
    if (list) list.push(m)
    else byBucket.set(idx, [m])
  }
  return [...byBucket.entries()]
    .sort(([a], [b]) => a - b)
    .map(([idx, list]) => {
      const counts = new Map<string, number>()
      for (const m of list) counts.set(m.type, (counts.get(m.type) ?? 0) + 1)
      let dominantType = list[0].type
      let best = 0
      for (const [type, n] of counts) {
        if (n > best) {
          best = n
          dominantType = type
        }
      }
      return {
        pct: ((idx + 0.5) / bucketCount) * 100,
        markers: list,
        dominantType,
        hasError: list.some((m) => m.type === 'error'),
      }
    })
}

export interface LaneGroup {
  layer: number
  lanes: TraceLaneData[]
}

/** Group backend-ordered lanes into consecutive layer groups. */
export function laneRows(lanes: TraceLaneData[]): LaneGroup[] {
  const groups: LaneGroup[] = []
  for (const lane of lanes) {
    const last = groups[groups.length - 1]
    if (last && last.layer === lane.layer) last.lanes.push(lane)
    else groups.push({ layer: lane.layer, lanes: [lane] })
  }
  return groups
}
