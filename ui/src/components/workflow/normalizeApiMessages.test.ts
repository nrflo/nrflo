import { describe, it, expect } from 'vitest'
import { normalizeApiMessages } from './normalizeApiMessages'
import type { MessageWithTime } from '@/types/workflow'

function msg(content: string, category: string, created_at = '2026-01-01T00:00:00Z'): MessageWithTime {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return { content, category: category as any, created_at }
}

describe('normalizeApiMessages', () => {
  it('returns empty array for empty input', () => {
    expect(normalizeApiMessages([])).toEqual([])
  })

  describe('tool_use_start + tool_use_input pair', () => {
    it('folds matched pair into single [name] input tool row', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=call-1 name=Bash', 'tool_use_start'),
        msg('[tool_use:input] id=call-1 input=git status', 'tool_use_input'),
      ])
      expect(result).toHaveLength(1)
      expect(result[0]).toMatchObject({ category: 'tool', content: '[Bash] git status' })
    })

    it('preserves created_at from start row in folded pair', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Read', 'tool_use_start', '2026-01-01T00:00:01Z'),
        msg('[tool_use:input] id=c1 input=file.ts', 'tool_use_input', '2026-01-01T00:00:02Z'),
      ])
      expect(result[0].created_at).toBe('2026-01-01T00:00:01Z')
    })

    it('treats start as orphan when next message id does not match', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        msg('[tool_use:input] id=c2 input=ls', 'tool_use_input'),
      ])
      expect(result).toHaveLength(2)
      expect(result[0]).toMatchObject({ category: 'tool', content: '[Bash]' })
    })

    it('handles multiline input content', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Write', 'tool_use_start'),
        msg('[tool_use:input] id=c1 input=line1\nline2', 'tool_use_input'),
      ])
      expect(result).toHaveLength(1)
      expect(result[0].content).toBe('[Write] line1\nline2')
    })
  })

  describe('orphan tool_use_start', () => {
    it('emits [name] tool row when no next message follows', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Write', 'tool_use_start'),
      ])
      expect(result).toHaveLength(1)
      expect(result[0]).toMatchObject({ category: 'tool', content: '[Write]' })
    })

    it('emits [name] tool row when next message is not tool_use_input', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Read', 'tool_use_start'),
        msg('some text', 'text'),
      ])
      expect(result).toHaveLength(2)
      expect(result[0]).toMatchObject({ category: 'tool', content: '[Read]' })
      expect(result[1]).toMatchObject({ category: 'text', content: 'some text' })
    })
  })

  describe('tool_result', () => {
    it('folds into [name] → output tool row', () => {
      const result = normalizeApiMessages([
        msg('[tool_result] name=Bash output=on branch main', 'tool_result'),
      ])
      expect(result).toHaveLength(1)
      expect(result[0]).toMatchObject({ category: 'tool', content: '[Bash] → on branch main' })
    })

    it('handles underscored tool names', () => {
      const result = normalizeApiMessages([
        msg('[tool_result] name=findings_add output=ok', 'tool_result'),
      ])
      expect(result[0]).toMatchObject({ category: 'tool', content: '[findings_add] → ok' })
    })
  })

  describe('tool_error (tool_result:error)', () => {
    it('folds into name: output error row without brackets', () => {
      const result = normalizeApiMessages([
        msg('[tool_result:error] name=Write output=permission denied', 'tool_error'),
      ])
      expect(result).toHaveLength(1)
      expect(result[0]).toMatchObject({ category: 'error', content: 'Write: permission denied' })
    })
  })

  describe('clean rows pass-through', () => {
    it('passes clean tool row through unchanged (same reference)', () => {
      const m = msg('[Bash] git status', 'tool')
      expect(normalizeApiMessages([m])[0]).toBe(m)
    })

    it('passes clean text row through unchanged', () => {
      const m = msg('some text', 'text')
      expect(normalizeApiMessages([m])[0]).toBe(m)
    })

    it('passes clean error row through unchanged', () => {
      const m = msg('error: something failed', 'error')
      expect(normalizeApiMessages([m])[0]).toBe(m)
    })

    it('passes clean result row through unchanged', () => {
      const m = msg('workflow done', 'result')
      expect(normalizeApiMessages([m])[0]).toBe(m)
    })
  })

  describe('regex failure fallback', () => {
    it('keeps malformed tool_use_start as-is without throwing', () => {
      const m = msg('not a valid start row', 'tool_use_start')
      const result = normalizeApiMessages([m])
      expect(result).toHaveLength(1)
      expect(result[0]).toBe(m)
    })

    it('keeps malformed tool_result as-is without throwing', () => {
      const m = msg('bad tool_result', 'tool_result')
      const result = normalizeApiMessages([m])
      expect(result).toHaveLength(1)
      expect(result[0]).toBe(m)
    })

    it('keeps malformed tool_error as-is without throwing', () => {
      const m = msg('bad tool_error', 'tool_error')
      const result = normalizeApiMessages([m])
      expect(result).toHaveLength(1)
      expect(result[0]).toBe(m)
    })
  })

  describe('output categories stay within MessageCategory set', () => {
    const VALID = new Set(['text', 'tool', 'subagent', 'skill', 'user_input', 'error', 'result', 'validation'])

    it('all output rows have valid MessageCategory values after full legacy mix', () => {
      const input = [
        msg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        msg('[tool_use:input] id=c1 input=ls', 'tool_use_input'),
        msg('[tool_result] name=Read output=ok', 'tool_result'),
        msg('[tool_result:error] name=Write output=err', 'tool_error'),
        msg('clean text', 'text'),
        msg('[Bash] already-clean', 'tool'),
      ]
      for (const m of normalizeApiMessages(input)) {
        expect(VALID.has(m.category)).toBe(true)
      }
    })

    it('reduces 4 legacy rows to 3 normalized rows (pair counts as one)', () => {
      const result = normalizeApiMessages([
        msg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        msg('[tool_use:input] id=c1 input=ls', 'tool_use_input'),
        msg('[tool_result] name=Read output=ok', 'tool_result'),
        msg('[tool_result:error] name=Write output=err', 'tool_error'),
      ])
      expect(result).toHaveLength(3)
    })
  })
})
