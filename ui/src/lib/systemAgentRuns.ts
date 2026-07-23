// Pure presentation helpers for SystemAgentRun rows (kept out of the
// component tree so they load in the fast node vitest project).
import type { SystemAgentRun } from '@/types/systemAgentRuns'

export function fallbackLabel(run: SystemAgentRun): string | null {
  if (!run.chain_position) return null
  const from = run.fallback_from?.[0]
  if (!from) return null

  const toModel = run.model_id || run.resolved_provider || ''
  const fromModel = from.model_id ?? from.provider
  if (fromModel !== toModel) {
    return `${fromModel} → ${toModel}`
  }
  if (from.execution_mode && from.execution_mode !== run.resolved_execution_mode) {
    return `${from.execution_mode} → ${run.resolved_execution_mode}`
  }
  return `${fromModel} → ${toModel}`
}

export function runAgentLabel(run: SystemAgentRun): string {
  if (run.kind === 'refinery_fold') return 'Refinery fold'
  if (run.kind === 'step_rotation') return run.step_id ? `Step rotation (${run.step_id})` : 'Step rotation'
  return run.agent_type || run.session_id
}

export function runTokens(run: SystemAgentRun): { input: number; output: number } {
  if (run.kind === 'refinery_fold') {
    return { input: run.prompt_tokens ?? 0, output: run.output_tokens ?? 0 }
  }
  if (run.kind === 'step_rotation') {
    return { input: 0, output: 0 }
  }
  return {
    input: run.tokens_json?.input_tokens ?? 0,
    output: run.tokens_json?.output_tokens ?? 0,
  }
}

export function formatCost(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return `$${n.toFixed(4)}`
}

export function runStatusVariant(run: SystemAgentRun): 'success' | 'destructive' | 'secondary' {
  if (run.kind === 'refinery_fold') {
    return run.status === 'ok' ? 'success' : 'destructive'
  }
  if (run.kind === 'step_rotation') return 'secondary'
  if (run.result === 'failed' || run.status === 'failed') return 'destructive'
  if (run.result === 'completed' || run.status === 'completed') return 'success'
  return 'secondary'
}
