import { describe, it, expect } from 'vitest'
import { parseTaskNotification } from './taskNotification'

describe('parseTaskNotification', () => {
  it('parses a well-formed envelope with all tags', () => {
    const content = [
      '<task-notification>',
      '<task-id>bdt1d3p3q</task-id>',
      '<status>completed</status>',
      '<summary>Background command finished</summary>',
      '<result>exit code 0</result>',
      '</task-notification>',
    ].join('\n')

    expect(parseTaskNotification(content)).toEqual({
      taskId: 'bdt1d3p3q',
      status: 'completed',
      summary: 'Background command finished',
      result: 'exit code 0',
    })
  })

  it('returns nulls for tags missing from a partial envelope', () => {
    const content = '<task-notification><task-id>abc</task-id><status>failed</status></task-notification>'

    expect(parseTaskNotification(content)).toEqual({
      taskId: 'abc',
      status: 'failed',
      summary: null,
      result: null,
    })
  })

  it('returns null for non-envelope input', () => {
    expect(parseTaskNotification('just a plain message')).toBeNull()
  })

  it('returns null for empty content', () => {
    expect(parseTaskNotification('')).toBeNull()
  })

  it('captures a large multi-line result body', () => {
    const bigResult = Array.from({ length: 50 }, (_, i) => `line ${i}`).join('\n')
    const content = `<task-notification><task-id>t1</task-id><result>${bigResult}</result></task-notification>`

    expect(parseTaskNotification(content)?.result).toBe(bigResult)
  })
})
