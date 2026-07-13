import { describe, it, expect } from 'vitest'
import { buildCLIEffortOptions, buildAPIEffortOptions, REASONING_EFFORT_OPTIONS } from './effortOptions'

function find(options: ReturnType<typeof buildCLIEffortOptions>, value: string) {
  return options.find((o) => o.value === value)
}

describe('effortOptions', () => {
  it('exposes the full option set including xhigh and ultra', () => {
    expect(REASONING_EFFORT_OPTIONS.map((o) => o.value)).toEqual([
      '', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra',
    ])
  })

  describe('buildCLIEffortOptions', () => {
    it('claude + opus_4_8-mapped model: xhigh enabled, ultra absent', () => {
      const options = buildCLIEffortOptions('claude', 'claude-opus-4-8-20260101')
      expect(find(options, 'ultra')).toBeUndefined()
      expect(find(options, 'xhigh')?.disabled).toBeFalsy()
    })

    it('claude + sonnet-5-mapped model: xhigh enabled', () => {
      const options = buildCLIEffortOptions('claude', 'claude-sonnet-5')
      expect(find(options, 'xhigh')?.disabled).toBeFalsy()
    })

    it('claude + haiku: xhigh present but disabled with a tooltip', () => {
      const options = buildCLIEffortOptions('claude', 'claude-haiku-4-5')
      const xhigh = find(options, 'xhigh')
      expect(xhigh).toBeDefined()
      expect(xhigh?.disabled).toBe(true)
      expect(xhigh?.tooltip).toMatch(/xhigh/i)
    })

    it('codex + gpt-5.6-sol: ultra enabled, xhigh absent', () => {
      const options = buildCLIEffortOptions('codex', 'gpt-5.6-sol')
      expect(find(options, 'xhigh')).toBeUndefined()
      expect(find(options, 'ultra')?.disabled).toBeFalsy()
    })

    it('codex + gpt-5.3: ultra disabled with a tooltip', () => {
      const options = buildCLIEffortOptions('codex', 'gpt-5.3')
      const ultra = find(options, 'ultra')
      expect(ultra).toBeDefined()
      expect(ultra?.disabled).toBe(true)
      expect(ultra?.tooltip).toMatch(/ultra/i)
    })
  })

  describe('buildAPIEffortOptions', () => {
    it('anthropic + opus-4-8-mapped model: xhigh enabled, no ultra at all', () => {
      const options = buildAPIEffortOptions('anthropic', 'claude-opus-4-8-20260101')
      expect(find(options, 'ultra')).toBeUndefined()
      expect(find(options, 'xhigh')?.disabled).toBeFalsy()
    })

    it('anthropic + haiku-mapped model: xhigh disabled with a tooltip', () => {
      const options = buildAPIEffortOptions('anthropic', 'claude-haiku-4-5')
      const xhigh = find(options, 'xhigh')
      expect(xhigh?.disabled).toBe(true)
      expect(xhigh?.tooltip).toMatch(/xhigh/i)
    })

    it('openai: neither xhigh nor ultra are offered', () => {
      const options = buildAPIEffortOptions('openai', 'gpt-5.6-sol')
      expect(find(options, 'xhigh')).toBeUndefined()
      expect(find(options, 'ultra')).toBeUndefined()
    })
  })
})
