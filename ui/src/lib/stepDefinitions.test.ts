import { describe, it, expect } from 'vitest'
import { validateStepDefinitions, STEP_FINDING_SCHEMAS } from './stepDefinitions'
import type { StepDefinition } from '@/types/workflow'

function makeStep(overrides: Partial<StepDefinition> = {}): StepDefinition {
  return {
    step_id: 'write-tests',
    title: 'Write tests',
    instruction: 'Write the tests for the feature.',
    ...overrides,
  }
}

describe('STEP_FINDING_SCHEMAS', () => {
  it('has exactly the 3 known schema names', () => {
    expect(STEP_FINDING_SCHEMAS).toEqual(['nonempty_text', 'ordered_lines', 'json_array_path_change'])
  })
})

describe('validateStepDefinitions', () => {
  it('accepts a minimal valid step', () => {
    expect(validateStepDefinitions([makeStep()])).toEqual([])
  })

  it('rejects empty steps', () => {
    expect(validateStepDefinitions([])).toEqual(['steps: at least one step is required'])
  })

  it('rejects more than 20 steps', () => {
    const steps = Array.from({ length: 21 }, (_, i) => makeStep({ step_id: `step-${i}` }))
    expect(validateStepDefinitions(steps)).toContain('steps: too many entries (max 20)')
  })

  it.each([
    ['uppercase', 'Write-Tests'],
    ['leading dash', '-write-tests'],
    ['too long', 'a'.repeat(65)],
  ])('rejects bad step_id: %s', (_label, stepId) => {
    const errors = validateStepDefinitions([makeStep({ step_id: stepId })])
    expect(errors.some((e) => e.includes('invalid step_id'))).toBe(true)
  })

  it('rejects duplicate step_id', () => {
    const errors = validateStepDefinitions([makeStep(), makeStep()])
    expect(errors.some((e) => e.includes('duplicate step_id'))).toBe(true)
  })

  it('rejects empty title', () => {
    const errors = validateStepDefinitions([makeStep({ title: '   ' })])
    expect(errors).toContain('steps[write-tests]: title is required')
  })

  it('rejects empty instruction', () => {
    const errors = validateStepDefinitions([makeStep({ instruction: '' })])
    expect(errors).toContain('steps[write-tests]: instruction is required')
  })

  it('rejects instruction over 16384 bytes', () => {
    const errors = validateStepDefinitions([makeStep({ instruction: 'a'.repeat(16385) })])
    expect(errors).toContain('steps[write-tests]: instruction exceeds 16384 bytes')
  })

  it('rejects more than 20 required_findings', () => {
    const findings = Array.from({ length: 21 }, (_, i) => ({ key: `k${i}`, schema: 'nonempty_text' }))
    const errors = validateStepDefinitions([makeStep({ required_findings: findings })])
    expect(errors.some((e) => e.includes('too many required_findings'))).toBe(true)
  })

  it('rejects unknown schema name', () => {
    const errors = validateStepDefinitions([
      makeStep({ required_findings: [{ key: 'k', schema: 'bogus_schema' }] }),
    ])
    expect(errors.some((e) => e.includes('invalid required_findings schema'))).toBe(true)
  })

  it.each([
    ['whitespace key', '  bad key  '],
    ['too long key', 'k'.repeat(129)],
  ])('rejects bad required_findings key: %s', (_label, key) => {
    const errors = validateStepDefinitions([
      makeStep({ required_findings: [{ key, schema: 'nonempty_text' }] }),
    ])
    expect(errors.some((e) => e.includes('required_findings key'))).toBe(true)
  })

  it('rejects empty check', () => {
    const errors = validateStepDefinitions([makeStep({ checks: ['   '] })])
    expect(errors.some((e) => e.includes('checks entry is empty'))).toBe(true)
  })

  it('rejects check over 1024 bytes', () => {
    const errors = validateStepDefinitions([makeStep({ checks: ['a'.repeat(1025)] })])
    expect(errors.some((e) => e.includes('checks entry exceeds 1024 bytes'))).toBe(true)
  })

  it('rejects path_overlap with an empty side', () => {
    const errors = validateStepDefinitions([
      makeStep({ path_overlap: { left: [], right: ['b'] } }),
    ])
    expect(errors.some((e) => e.includes('left and right must both be non-empty'))).toBe(true)
  })

  it('rejects path_overlap side with more than 10 keys', () => {
    const errors = validateStepDefinitions([
      makeStep({
        path_overlap: {
          left: Array.from({ length: 11 }, (_, i) => `l${i}`),
          right: ['r1'],
        },
      }),
    ])
    expect(errors.some((e) => e.includes('too many keys'))).toBe(true)
  })

  it('rejects path_overlap key present in both left and right', () => {
    const errors = validateStepDefinitions([
      makeStep({ path_overlap: { left: ['shared'], right: ['shared'] } }),
    ])
    expect(errors.some((e) => e.includes('appears in both left and right'))).toBe(true)
  })
})
