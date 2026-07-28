import { describe, it, expect } from 'vitest'
import {
  initialSessionStreamState,
  sessionEventReducer,
  mergeStream,
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

  it('session.cost_updated sets cost, ignoring a null payload', () => {
    let state = initialSessionStreamState()
    state = sessionEventReducer(state, {
      type: 'session.cost_updated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { cost_estimate: 1.23 },
    })
    expect(state.cost).toBe(1.23)

    const unchanged = sessionEventReducer(state, {
      type: 'session.cost_updated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: {},
    })
    expect(unchanged.cost).toBe(1.23)
  })

  it('an unrelated event leaves cost undefined', () => {
    const state = sessionEventReducer(initialSessionStreamState(), turnEvent('running'))
    expect(state.cost).toBeUndefined()
  })

  it('console_chat.sibling_opened populates siblingOpened for the origin session', () => {
    let state = initialSessionStreamState()
    expect(state.siblingOpened).toBeUndefined()

    state = sessionEventReducer(state, {
      type: 'console_chat.sibling_opened',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { origin_session_id: 'sid-1', sibling_session_id: 'sid-2', reason: 'model_switch' },
    })

    expect(state.siblingOpened).toEqual({
      origin_session_id: 'sid-1',
      sibling_session_id: 'sid-2',
      reason: 'model_switch',
    })
  })

  it('console_chat.queued seeds null then folds the full live queue', () => {
    let state = initialSessionStreamState()
    expect(state.queuedPrompts).toBeNull()

    state = sessionEventReducer(state, {
      type: 'console_chat.queued',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { count: 2, prompts: ['one', 'two'] },
    })
    expect(state.queuedPrompts).toEqual(['one', 'two'])

    state = sessionEventReducer(state, {
      type: 'console_chat.queued',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { count: 0, prompts: [] },
    })
    expect(state.queuedPrompts).toEqual([])
  })

  it('console_chat.yolo seeds null then folds the live effective state', () => {
    let state = initialSessionStreamState()
    expect(state.yolo).toBeNull()

    state = sessionEventReducer(state, {
      type: 'console_chat.yolo',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { yolo: true },
    })
    expect(state.yolo).toBe(true)

    state = sessionEventReducer(state, {
      type: 'console_chat.yolo',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { yolo: false },
    })
    expect(state.yolo).toBe(false)
  })

  it('console.context_rotated appends a rotation notice, in arrival order', () => {
    let state = initialSessionStreamState()
    expect(state.rotations).toEqual([])

    state = sessionEventReducer(state, {
      type: 'console.context_rotated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:00:00Z',
      data: { session_id: 'sid-1', tokens_before: 9000, tokens_after: 1200 },
    })
    state = sessionEventReducer(state, {
      type: 'console.context_rotated',
      project_id: 'p',
      ticket_id: '',
      session_id: 'sid-1',
      timestamp: '2026-01-01T00:05:00Z',
      data: { session_id: 'sid-1', tokens_before: 8500, tokens_after: 1100 },
    })

    expect(state.rotations).toEqual([
      { session_id: 'sid-1', tokens_before: 9000, tokens_after: 1200 },
      { session_id: 'sid-1', tokens_before: 8500, tokens_after: 1100 },
    ])
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
