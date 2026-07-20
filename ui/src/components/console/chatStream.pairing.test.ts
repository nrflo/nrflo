import { describe, it, expect } from 'vitest'
import { pairToolMessages } from './chatStream'
import type { MessageWithTime } from '@/types/workflow'

function message(overrides: Partial<MessageWithTime> = {}): MessageWithTime {
  return {
    content: 'hello',
    category: 'text',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('pairToolMessages', () => {
  function invokeMessage(
    toolUseId: string | undefined,
    endedAt: string | undefined,
    createdAt = '2026-01-01T00:00:00.000Z',
    input?: unknown
  ): MessageWithTime {
    return message({
      category: 'tool',
      content: '[Bash] ls -la',
      created_at: createdAt,
      payload: toolUseId
        ? JSON.stringify({ tool_use_id: toolUseId, ended_at: endedAt, ...(input !== undefined ? { input } : {}) })
        : undefined,
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

  it('surfaces the structured input from the invoke payload', () => {
    const invoke = invokeMessage('t5', '2026-01-01T00:00:01.000Z', undefined, { command: 'ls' })
    const pairs = pairToolMessages([invoke])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].input).toEqual({ command: 'ls' })
    expect(pairs[0].inputTruncated).toBeUndefined()
  })

  it('leaves input undefined for older rows whose payload has no input field', () => {
    const invoke = invokeMessage('t6', '2026-01-01T00:00:01.000Z')
    const pairs = pairToolMessages([invoke])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].input).toBeUndefined()
  })

  it('surfaces input_truncated as inputTruncated when the backend capped the input', () => {
    const invoke = message({
      category: 'tool',
      content: '[Bash] ls -la',
      payload: JSON.stringify({ tool_use_id: 't7', ended_at: '2026-01-01T00:00:01.000Z', input_truncated: true }),
    })
    const pairs = pairToolMessages([invoke])

    expect(pairs).toHaveLength(1)
    expect(pairs[0].input).toBeUndefined()
    expect(pairs[0].inputTruncated).toBe(true)
  })
})
