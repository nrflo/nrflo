import { describe, it, expect } from 'vitest'
import { fallbackLabel, formatCost, runAgentLabel, runStatusVariant, runTokens } from './systemAgentRuns'
import type { SystemAgentRun } from '@/types/systemAgentRuns'

function makeRun(overrides: Partial<SystemAgentRun> = {}): SystemAgentRun {
  return {
    kind: 'agent_session',
    session_id: 's1',
    agent_type: 'implementor',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('fallbackLabel', () => {
  it('returns null when chain_position is 0 (no fallback occurred)', () => {
    expect(fallbackLabel(makeRun({ chain_position: 0 }))).toBeNull()
  })

  it('returns null when chain_position is absent', () => {
    expect(fallbackLabel(makeRun())).toBeNull()
  })

  it('returns null when fallback_from is empty', () => {
    expect(fallbackLabel(makeRun({ chain_position: 1, fallback_from: [] }))).toBeNull()
  })

  it('renders "from → to" when the model changed', () => {
    const run = makeRun({
      chain_position: 1,
      fallback_from: [{ provider: 'anthropic', model_id: 'sonnet-5', execution_mode: 'api', reasoning_effort: '', tier: 1 }],
      model_id: 'qwen3-local',
      resolved_provider: 'local',
    })
    expect(fallbackLabel(run)).toBe('sonnet-5 → qwen3-local')
  })

  it('renders "mode → mode" when only the execution mode changed (same model)', () => {
    const run = makeRun({
      chain_position: 1,
      fallback_from: [{ provider: 'anthropic', model_id: 'sonnet-5', execution_mode: 'api', reasoning_effort: '', tier: 1 }],
      model_id: 'sonnet-5',
      resolved_execution_mode: 'cli_interactive',
    })
    expect(fallbackLabel(run)).toBe('api → cli_interactive')
  })

  it('renders "mode → mode" for a refinery_fold run when only execution mode changed (same registry slug)', () => {
    const run = makeRun({
      kind: 'refinery_fold',
      chain_position: 1,
      fallback_from: [{ provider: 'anthropic', model_id: 'haiku-4-5', execution_mode: 'api', reasoning_effort: '', tier: 1 }],
      model_id: 'haiku-4-5',
      resolved_execution_mode: 'cli_interactive',
    })
    expect(fallbackLabel(run)).toBe('api → cli_interactive')
  })

  it('renders "model → model" for a refinery_fold run when the model changed across the chain', () => {
    const run = makeRun({
      kind: 'refinery_fold',
      chain_position: 1,
      fallback_from: [{ provider: 'anthropic', model_id: 'haiku-4-5', execution_mode: 'api', reasoning_effort: '', tier: 1 }],
      model_id: 'gpt-5.6-luna',
      resolved_execution_mode: 'cli_interactive',
    })
    expect(fallbackLabel(run)).toBe('haiku-4-5 → gpt-5.6-luna')
  })
})

describe('runAgentLabel', () => {
  it('labels refinery folds distinctly from agent sessions', () => {
    expect(runAgentLabel(makeRun({ kind: 'refinery_fold' }))).toBe('Refinery fold')
    expect(runAgentLabel(makeRun({ agent_type: 'implementor' }))).toBe('implementor')
    expect(runAgentLabel(makeRun({ agent_type: undefined, session_id: 'sess-1' }))).toBe('sess-1')
  })

  it('labels step rotations, including the step_id when present', () => {
    expect(runAgentLabel(makeRun({ kind: 'step_rotation', step_id: 'step-3' }))).toBe(
      'Step rotation (step-3)'
    )
    expect(runAgentLabel(makeRun({ kind: 'step_rotation', step_id: undefined }))).toBe('Step rotation')
  })
})

describe('runTokens', () => {
  it('reads prompt/output tokens for refinery folds', () => {
    expect(runTokens(makeRun({ kind: 'refinery_fold', prompt_tokens: 10, output_tokens: 20 }))).toEqual({
      input: 10,
      output: 20,
    })
  })

  it('reads tokens_json for agent sessions, defaulting to 0', () => {
    expect(runTokens(makeRun({ tokens_json: { input_tokens: 5, output_tokens: 7 } }))).toEqual({
      input: 5,
      output: 7,
    })
    expect(runTokens(makeRun())).toEqual({ input: 0, output: 0 })
  })

  it('reports zero tokens for step rotations regardless of other fields', () => {
    expect(
      runTokens(makeRun({ kind: 'step_rotation', tokens_json: { input_tokens: 99, output_tokens: 99 } }))
    ).toEqual({ input: 0, output: 0 })
  })
})

describe('formatCost', () => {
  it('formats a numeric cost to 4 decimals and falls back to an em dash', () => {
    expect(formatCost(1.23456)).toBe('$1.2346')
    expect(formatCost(null)).toBe('—')
    expect(formatCost(undefined)).toBe('—')
  })
})

describe('runStatusVariant', () => {
  it('maps refinery fold ok/failed to success/destructive', () => {
    expect(runStatusVariant(makeRun({ kind: 'refinery_fold', status: 'ok' }))).toBe('success')
    expect(runStatusVariant(makeRun({ kind: 'refinery_fold', status: 'failed' }))).toBe('destructive')
  })

  it('maps agent_session result/status to success/destructive/secondary', () => {
    expect(runStatusVariant(makeRun({ result: 'completed' }))).toBe('success')
    expect(runStatusVariant(makeRun({ result: 'failed' }))).toBe('destructive')
    expect(runStatusVariant(makeRun({ status: 'running' }))).toBe('secondary')
  })

  it('always maps step rotations to secondary, ignoring status/result', () => {
    expect(runStatusVariant(makeRun({ kind: 'step_rotation', status: 'failed' }))).toBe('secondary')
    expect(runStatusVariant(makeRun({ kind: 'step_rotation', result: 'completed' }))).toBe('secondary')
  })
})
