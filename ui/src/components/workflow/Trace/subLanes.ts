// Pure grouping helpers for delegate/consult sub-lanes. TraceView stays free
// of this logic, mirroring how timeScale.ts holds laneRows/bucketMarkers.

import type { TraceLaneData } from './types'

export interface SubLaneGroup {
  key: string
  kind: 'delegate' | 'consult' | string
  label: string
  lanes: TraceLaneData[]
}

/** Index sub-lanes by their parent_lane_id, dropping entries with no key. */
export function indexSubLanesByParent(subLanes: TraceLaneData[]): Map<string, TraceLaneData[]> {
  const byParent = new Map<string, TraceLaneData[]>()
  for (const lane of subLanes) {
    if (!lane.parent_lane_id) continue
    const list = byParent.get(lane.parent_lane_id)
    if (list) list.push(lane)
    else byParent.set(lane.parent_lane_id, [lane])
  }
  return byParent
}

function groupLabel(kind: string, lanes: TraceLaneData[]): string {
  if (kind === 'delegate') return `⤷ delegate ×${lanes.length}`
  if (kind === 'consult') return `⤷ consult · ${lanes[0]?.agent_type ?? ''}`.trim()
  return `⤷ ${kind}`
}

/**
 * Bucket one parent's sub-lanes by delegation_id/consult_id into labelled
 * groups. Lanes missing both ids fall back to a per-lane_id bucket so nothing
 * is silently dropped.
 */
export function groupSubLanes(lanes: TraceLaneData[]): SubLaneGroup[] {
  const byKey = new Map<string, TraceLaneData[]>()
  for (const lane of lanes) {
    const key = lane.delegation_id ?? lane.consult_id ?? lane.lane_id
    const list = byKey.get(key)
    if (list) list.push(lane)
    else byKey.set(key, [lane])
  }
  return [...byKey.entries()].map(([key, groupLanes]) => {
    const kind = groupLanes[0]?.kind ?? 'delegate'
    return { key, kind, label: groupLabel(kind, groupLanes), lanes: groupLanes }
  })
}

/**
 * Resolve sub-lane groups for a parent lane_id, given the full sub_lanes
 * index and the set of rendered parent lane ids. Sub-lanes whose parent
 * isn't a rendered lane are dropped rather than promoted to top level.
 */
export function subLaneGroupsFor(
  parentLaneId: string,
  byParent: Map<string, TraceLaneData[]>,
  renderedLaneIds: Set<string>
): SubLaneGroup[] {
  if (!renderedLaneIds.has(parentLaneId)) return []
  const lanes = byParent.get(parentLaneId)
  if (!lanes || lanes.length === 0) return []
  return groupSubLanes(lanes)
}
