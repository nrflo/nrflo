import { describe, it, expect } from 'vitest'
import { cliFromModelId } from './modelId'

describe('cliFromModelId', () => {
  it('extracts the cli from colon-form model_id', () => {
    expect(cliFromModelId('claude:sonnet-5')).toBe('claude')
  })

  it('returns undefined when there is no colon', () => {
    expect(cliFromModelId('sonnet-5')).toBeUndefined()
  })

  it('returns undefined for undefined input', () => {
    expect(cliFromModelId(undefined)).toBeUndefined()
  })
})
