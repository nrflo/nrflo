import type { MessageWithTime } from '@/types/workflow'

const TOOL_USE_START = /^\[tool_use:start\]\s+id=(\S+)\s+name=(\S+)/
const TOOL_USE_INPUT = /^\[tool_use:input\]\s+id=(\S+)\s+input=([\s\S]*)$/
const TOOL_RESULT = /^\[tool_result\]\s+name=(\S+)\s+output=([\s\S]*)$/
const TOOL_ERROR = /^\[tool_result:error\]\s+name=(\S+)\s+output=([\s\S]*)$/

export function normalizeApiMessages(messages: MessageWithTime[]): MessageWithTime[] {
  const out: MessageWithTime[] = []
  let i = 0
  while (i < messages.length) {
    const m = messages[i]
    const cat = m.category as string

    if (cat === 'tool_use_start') {
      const startMatch = m.content.match(TOOL_USE_START)
      if (!startMatch) {
        out.push(m)
        i++
        continue
      }
      const [, startId, name] = startMatch
      const next = messages[i + 1]
      if (next && (next.category as string) === 'tool_use_input') {
        const inputMatch = next.content.match(TOOL_USE_INPUT)
        if (inputMatch && inputMatch[1] === startId) {
          out.push({ content: `[${name}] ${inputMatch[2]}`, category: 'tool', created_at: m.created_at, payload: m.payload })
          i += 2
          continue
        }
      }
      // Orphan start — no following matching tool_use_input
      out.push({ content: `[${name}]`, category: 'tool', created_at: m.created_at, payload: m.payload })
      i++
    } else if (cat === 'tool_use_input') {
      // Standalone (page-boundary split, not consumed by a preceding start)
      const inputMatch = m.content.match(TOOL_USE_INPUT)
      if (!inputMatch) {
        out.push(m)
        i++
        continue
      }
      out.push({ content: inputMatch[2], category: 'tool', created_at: m.created_at, payload: m.payload })
      i++
    } else if (cat === 'tool_result') {
      const match = m.content.match(TOOL_RESULT)
      if (!match) {
        out.push(m)
        i++
        continue
      }
      out.push({ content: `[${match[1]}] → ${match[2]}`, category: 'tool', created_at: m.created_at, payload: m.payload })
      i++
    } else if (cat === 'tool_error') {
      const match = m.content.match(TOOL_ERROR)
      if (!match) {
        out.push(m)
        i++
        continue
      }
      // Plain `name: output` — no bracket — so the Error badge renders without a double-badge
      out.push({ content: `${match[1]}: ${match[2]}`, category: 'error', created_at: m.created_at, payload: m.payload })
      i++
    } else {
      out.push(m)
      i++
    }
  }
  return out
}
