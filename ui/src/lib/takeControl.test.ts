import { describe, it, expect } from 'vitest'
import { supportsTakeControl, pickTakeControlTarget } from './takeControl'
import type { ActiveAgentV4 } from '@/types/workflow'

function makeAgent(overrides: Partial<ActiveAgentV4> = {}): ActiveAgentV4 {
  return {
    agent_type: 'implementor',
    phase: 'implementation',
    model_id: 'claude:sonnet-5',
    pid: 1,
    session_id: 's',
    started_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('supportsTakeControl', () => {
  it('true for a running claude agent in cli_interactive mode', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'claude:sonnet-5', effective_mode: 'cli_interactive' }))).toBe(true)
  })

  it('true for a running codex agent in cli_interactive mode', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'codex:gpt-5', effective_mode: 'cli_interactive' }))).toBe(true)
  })

  it('false for a claude agent in api mode', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'claude:sonnet-5', effective_mode: 'api' }))).toBe(false)
  })

  it('false for a claude agent in script mode', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'claude:sonnet-5', effective_mode: 'script' }))).toBe(false)
  })

  it('true for a claude agent with no effective_mode (back-compat default)', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'claude:sonnet-5', effective_mode: undefined }))).toBe(true)
  })

  it('false for an unknown cli', () => {
    expect(supportsTakeControl(makeAgent({ model_id: 'openai:gpt-5' }))).toBe(false)
  })

  it('false when model_id is missing', () => {
    expect(supportsTakeControl(makeAgent({ model_id: undefined }))).toBe(false)
  })

  it('false when session_id is missing', () => {
    expect(supportsTakeControl(makeAgent({ session_id: undefined }))).toBe(false)
  })

  it('false for a completed agent (has result)', () => {
    expect(supportsTakeControl(makeAgent({ result: 'pass' }))).toBe(false)
  })
})

describe('pickTakeControlTarget', () => {
  it('returns the panel agent when it is supported', () => {
    const panelAgent = makeAgent({ session_id: 'panel-session' })
    const activeAgents = { fallback: makeAgent({ session_id: 'fallback-session' }) }

    expect(pickTakeControlTarget(activeAgents, panelAgent)).toBe(panelAgent)
  })

  it('falls back to the first supported activeAgents entry when the panel agent is unsupported', () => {
    const panelAgent = makeAgent({ effective_mode: 'api' })
    const fallback = makeAgent({ session_id: 'fallback-session' })
    const activeAgents = { fallback }

    expect(pickTakeControlTarget(activeAgents, panelAgent)).toBe(fallback)
  })

  it('falls back to the first supported activeAgents entry when the panel agent is completed', () => {
    const panelAgent = makeAgent({ result: 'pass' })
    const fallback = makeAgent({ session_id: 'fallback-session' })
    const activeAgents = { fallback }

    expect(pickTakeControlTarget(activeAgents, panelAgent)).toBe(fallback)
  })

  it('returns undefined when nothing qualifies', () => {
    const panelAgent = makeAgent({ result: 'pass' })
    const activeAgents = { unsupported: makeAgent({ model_id: 'openai:gpt-5' }) }

    expect(pickTakeControlTarget(activeAgents, panelAgent)).toBeUndefined()
  })

  it('returns undefined when panelAgent is null and activeAgents is empty', () => {
    expect(pickTakeControlTarget({}, null)).toBeUndefined()
  })
})
