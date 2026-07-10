// Single source for trace status/category colors — segment pairs copied from
// PhaseGraph/AgentFlowNode.tsx so the two workflow views stay visually aligned.

/** Segment bar classes by session status/result. */
export function segmentClasses(status: string, result?: string): string {
  if (status === 'running' || status === 'user_interactive') {
    return 'border-amber-400 dark:border-amber-500 bg-amber-50 dark:bg-amber-950/30 animate-pulse'
  }
  if (result === 'pass') {
    return 'border-green-500 bg-green-50 dark:bg-green-950/30'
  }
  if (result === 'fail' || status === 'failed' || status === 'timeout') {
    return 'border-red-500 bg-red-50 dark:bg-red-950/30'
  }
  if (status === 'continued') {
    return 'border-blue-400 dark:border-blue-500 bg-blue-50 dark:bg-blue-950/30'
  }
  if (status === 'skipped') {
    return 'border-gray-300 bg-gray-50 dark:border-gray-600 dark:bg-gray-800/50 opacity-60'
  }
  return 'border-gray-300 bg-white dark:border-gray-600 dark:bg-gray-900'
}

/** Marker dot classes by marker type. */
export function markerClasses(type: string): string {
  switch (type) {
    case 'tool':
      return 'bg-sky-500 dark:bg-sky-400'
    case 'subagent':
      return 'bg-violet-500 dark:bg-violet-400'
    case 'skill':
      return 'bg-teal-500 dark:bg-teal-400'
    case 'user_input':
      return 'bg-amber-500 dark:bg-amber-400'
    case 'error':
      return 'bg-red-500 dark:bg-red-400'
    case 'finding':
      return 'bg-green-600 dark:bg-green-400'
    case 'lifecycle':
      return 'bg-orange-500 dark:bg-orange-400'
    case 'thinking':
      return 'bg-gray-400 dark:bg-gray-500'
    default:
      return 'bg-gray-500 dark:bg-gray-400'
  }
}

/** Legend chip label per marker type (also the filter chip set). */
export const MARKER_TYPES = ['tool', 'subagent', 'skill', 'user_input', 'error', 'finding', 'lifecycle'] as const
