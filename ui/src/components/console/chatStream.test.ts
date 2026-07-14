import { describe, it, expect } from 'vitest'
import {
  initialSessionStreamState,
  sessionEventReducer,
  mergeStream,
  pairToolMessages,
} from './chatStream'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { MessageWithTime } from '@/types/workflow'

function deltaEvent(item_id: string, text: string): WSEvent {
  return {
    type: 'console_chat.delta',
    project_id: 'p',
    ticket_id: '',
    session_id: 'sid-1',
    timestamp: '2026-01-01T00:00:00Z',
    data: { item_id, text },
  }
}

function turnEvent(state: 'idle' | 'running'): WSEvent {
  return {
    type: 'console_chat.turn',
    project_id: 'p',
    ticket_id: '',
    session_id: 'sid-1',
    timestamp: '2026-01-01T00:00:00Z',
    data: { state },
  }
}

function message(overrides: Partial<MessageWithTime> = {}): MessageWithTime {
  return {
    content: 'hello',
    category: 'text',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('sessionEventReducer', () => {
  it('accumulates deltas per item_id', () => {
    let state = initialSessionStreamState()
    state = sessionEventReducer(state, deltaEvent('item-1', 'Hello '))
    state = sessionEventReducer(state, deltaEvent('item-1', 'world'))
    state = sessionEventReducer(state, deltaEvent('item-2', 'other'))

    expect(state.deltas.get('item-1')).toBe('Hello world')
    expect(state.deltas.get('item-2')).toBe('other')
  })

  it('turn running -> idle clears the streaming indicator', () => {
    let state = initialSessionStreamState()
    state = sessionEventReducer(state, turnEvent('running'))
    expect(state.turn).toBe('running')
    expect(state.turnLive).toBe(true)

    state = sessionEventReducer(state, turnEvent('idle'))
    expect(state.turn).toBe('idle')
  })

  it('approval_resolved keeps the original request row and records the decision', () => {
    let state = initialSessionStreamState()
    state = sessionEventReducer(state, {
      type: 'console_chat.approval_request',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { approval_id: 'a1', kind: 'bash', command: 'rm -rf /tmp/x', cwd: '/tmp', reason: '' },
    })
    state = sessionEventReducer(state, {
      type: 'console_chat.approval_resolved',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { approval_id: 'a1', decision: 'deny', reason: 'nrflo: approval timed out' },
    })

    expect(state.approvals).toHaveLength(1)
    expect(state.approvals[0].approval_id).toBe('a1')
    expect(state.resolvedApprovals.get('a1')).toEqual({
      approval_id: 'a1',
      decision: 'deny',
      reason: 'nrflo: approval timed out',
    })
  })

  it('agent.context_updated sets contextLeft, ignoring a null payload', () => {
    let state = initialSessionStreamState()
    state = sessionEventReducer(state, {
      type: 'agent.context_updated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { context_left: 42 },
    })
    expect(state.contextLeft).toBe(42)

    const unchanged = sessionEventReducer(state, {
      type: 'agent.context_updated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: {},
    })
    expect(unchanged.contextLeft).toBe(42)
  })
})

describe('mergeStream', () => {
  it('drops a delta once the persisted text row covering it arrives', () => {
    const deltas = new Map([['item-1', 'Hello world']])
    const withoutHistory = mergeStream([], deltas)
    expect(withoutHistory).toEqual([{ kind: 'live', itemId: 'item-1', text: 'Hello world' }])

    const persisted = [message({ category: 'text', content: 'Hello world' })]
    const withHistory = mergeStream(persisted, deltas)
    expect(withHistory).toEqual([{ kind: 'message', message: persisted[0] }])
  })

  it('a delta for a still-streaming item survives an unrelated history refetch', () => {
    const deltas = new Map([['item-1', 'still typing']])
    const persistedBefore = [message({ category: 'text', content: 'unrelated completed message' })]
    const persistedAfter = [
      ...persistedBefore,
      message({ category: 'user_input', content: 'a new user message' }),
    ]

    const before = mergeStream(persistedBefore, deltas)
    const after = mergeStream(persistedAfter, deltas)

    expect(before.some((i) => i.kind === 'live' && i.itemId === 'item-1')).toBe(true)
    expect(after.some((i) => i.kind === 'live' && i.itemId === 'item-1')).toBe(true)
  })

  it('drops an empty delta buffer without needing a persisted match', () => {
    const deltas = new Map([['item-1', '']])
    expect(mergeStream([], deltas)).toEqual([])
  })

  // Regression: dedupe used to drop any delta *contained* in a persisted text
  // row, so the first few characters of a new reply were swallowed by an
  // earlier message that happened to start the same way — the streaming bubble
  // only appeared once the buffer grew unique.
  it('keeps a short in-progress delta that is a prefix of an earlier message', () => {
    const deltas = new Map([['item-2', 'Sure, ']])
    const persisted = [message({ category: 'text', content: 'Sure, here is the earlier answer.' })]

    const merged = mergeStream(persisted, deltas)

    expect(merged).toContainEqual({ kind: 'live', itemId: 'item-2', text: 'Sure, ' })
  })

  it('drops a delta whose persisted row differs only in surrounding whitespace', () => {
    const deltas = new Map([['item-3', 'Hello world']])
    const persisted = [message({ category: 'text', content: 'Hello world\n' })]

    expect(mergeStream(persisted, deltas)).toEqual([{ kind: 'message', message: persisted[0] }])
  })
})

describe('pairToolMessages', () => {
  function invokeMessage(toolUseId: string | undefined, endedAt: string | undefined, createdAt = '2026-01-01T00:00:00.000Z'): MessageWithTime {
    return message({
      category: 'tool',
      content: '[Bash] ls -la',
      created_at: createdAt,
      payload: toolUseId ? JSON.stringify({ tool_use_id: toolUseId, ended_at: endedAt }) : undefined,
    })
  }

  it('pairs an invoke with its immediately-following result and computes duration from ended_at', () => {
    const invoke = invokeMessage('t1', '2026-01-01T00:00:02.500Z')
    const result = message({ category: 'tool', content: '[Bash] file1\nfile2' })
    const pairs = pairToolMessages([invoke, result])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].toolUseId).toBe('t1')
    expect(pairs[0].result).toBe(result)
    expect(pairs[0].durationMs).toBe(2500)
    expect(pairs[0].running).toBe(false)
  })

  it('an invoke with no ended_at and no result renders as still-running', () => {
    const invoke = invokeMessage('t2', undefined)
    const pairs = pairToolMessages([invoke])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].result).toBeUndefined()
    expect(pairs[0].durationMs).toBeUndefined()
    expect(pairs[0].running).toBe(true)
  })

  it('pairs an error-category result row', () => {
    const invoke = invokeMessage('t3', '2026-01-01T00:00:01.000Z')
    const errorResult = message({ category: 'error', content: '[Bash] command not found' })
    const pairs = pairToolMessages([invoke, errorResult])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].result).toBe(errorResult)
    expect(pairs[0].result?.category).toBe('error')
  })

  it('does not pair rows without a tool_use_id in the payload', () => {
    const invoke = message({ category: 'tool', content: '[Codex] no id', payload: JSON.stringify({}) })
    const result = message({ category: 'tool', content: 'result text' })
    const pairs = pairToolMessages([invoke, result])
    expect(pairs).toHaveLength(0)
  })

  it('parses payload defensively when it is already an object, not a JSON string', () => {
    const invoke = message({
      category: 'skill',
      content: '[Skill] do-thing',
      payload: { tool_use_id: 't4', ended_at: undefined } as unknown as MessageWithTime['payload'],
    })
    const pairs = pairToolMessages([invoke])
    expect(pairs).toHaveLength(1)
    expect(pairs[0].toolUseId).toBe('t4')
    expect(pairs[0].running).toBe(true)
  })
})
