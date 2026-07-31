// Pure, DOM-free grouping of SystemAgentRun rows that share a delegation_id
// into a single collapsible entry, anchored at the newest worker so the
// newest-first ordering of the flat list is preserved.
import type { SystemAgentRun } from '@/types/systemAgentRuns'
import { runTokens } from './systemAgentRuns'

export interface SystemAgentRunDelegationGroup {
  kind: 'delegation_group'
  delegation_id: string
  caller_session_id?: string
  delegate_tier?: string
  branch?: string
  fanout: number
  workers: SystemAgentRun[]
  input_tokens: number
  output_tokens: number
  cost_estimate: number | null
  status?: string
  created_at: string
}

export type SystemAgentRunGroupEntry =
  | { kind: 'run'; run: SystemAgentRun }
  | SystemAgentRunDelegationGroup

export function groupSystemAgentRuns(items: SystemAgentRun[]): SystemAgentRunGroupEntry[] {
  const groups = new Map<string, SystemAgentRunDelegationGroup>()
  const entries: SystemAgentRunGroupEntry[] = []

  for (const run of items) {
    if (!run.delegation_id) {
      entries.push({ kind: 'run', run })
      continue
    }

    const existing = groups.get(run.delegation_id)
    if (existing) {
      existing.workers.push(run)
      const tokens = runTokens(run)
      existing.input_tokens += tokens.input
      existing.output_tokens += tokens.output
      if (run.cost_estimate != null) {
        existing.cost_estimate = (existing.cost_estimate ?? 0) + run.cost_estimate
      }
      continue
    }

    const tokens = runTokens(run)
    const group: SystemAgentRunDelegationGroup = {
      kind: 'delegation_group',
      delegation_id: run.delegation_id,
      caller_session_id: run.caller_session_id,
      delegate_tier: run.delegate_tier,
      branch: run.delegation_branch,
      fanout: run.fanout ?? 1,
      workers: [run],
      input_tokens: tokens.input,
      output_tokens: tokens.output,
      cost_estimate: run.cost_estimate ?? null,
      status: run.delegation_status,
      created_at: run.created_at,
    }
    groups.set(run.delegation_id, group)
    // Anchored at the newest worker's position — items arrive newest-first,
    // so the group's first-seen worker sets the group's slot in the list.
    entries.push(group)
  }

  return entries
}
